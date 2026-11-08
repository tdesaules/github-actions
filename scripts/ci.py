#!/usr/bin/env python3
"""Discover components from actual files; select work without a central catalogue."""
import argparse
import hashlib
import fnmatch
import json
import os
from pathlib import Path
import re
import shlex
import subprocess
import sys

from oci_archive import inspect_archive

ROOT = Path(__file__).resolve().parents[1]
SEMVER = re.compile(r"^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*))*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$")
IMAGE = re.compile(r"^(?:  image|        BUILDER_IMAGE): docker://([^\s]+)$", re.M)
BASES = {'scratch', 'quay.io/fedora/fedora-minimal', 'quay.io/fedora/fedora',
         'docker.io/library/alpine', 'docker.io/chainguard/wolfi-base',
         'docker.io/redhat/ubi10-micro', 'docker.io/redhat/ubi10-minimal'}


def validate_base(reference, stages, name):
    if reference in stages or reference == 'scratch':
        return
    if not re.search(r'@sha256:[a-f0-9]{64}$', reference):
        raise ValueError(f'{name}: unpinned external base/donor {reference}')
    repository = reference.split('@')[0].split(':')[0]
    if repository not in BASES:
        print(f'WARNING: {name}: base/donor outside allowlist: {reference}; migration required', file=sys.stderr)


def git(*args):
    return subprocess.check_output(["git", *args], cwd=ROOT).decode().strip()


def discover(root=ROOT):
    actions = {}
    owners = {}
    for path in sorted([*root.glob("oci/*/action.yml"), *root.glob("check/*/action.yml"), *root.glob("lint/*/action.yml"), *root.glob("workflow/*/action.yml"), *root.glob("maintenance/*/action.yml")]):
        if path.parent.parent.name == 'workflow' and not (path.parent / 'Containerfile').exists():
            continue
        image, reference = image_reference(path)
        expected = path.parent.parent.name+'-'+path.parent.name
        # Linter moves preserve their existing independently published image IDs.
        if path.parent.parent.name != 'lint' and image != expected:
            raise ValueError(f'{path}: image must be owned by this action ({expected})')
        directory = str(path.parent.relative_to(root))
        actions[directory] = image
        if (path.parent / "Containerfile").is_file():
            if image in owners:
                raise ValueError(f"duplicate Containerfile owner for {image}")
            owners[image] = directory
    if set(actions.values()) != set(owners):
        raise ValueError("every action image must have exactly one Containerfile owner")
    return actions, owners


def image_reference(path):
    text = path.read_text()
    match = IMAGE.search(text)
    if match:
        reference = match[1]
        return reference.rsplit('/', 1)[-1].split(':')[0].split('@')[0], reference
    if not re.search(r'^  image: Containerfile$', text, re.M) or not re.search(r'^  using: docker$', text, re.M):
        raise ValueError(f'{path}: missing Docker image reference')
    image = path.parent.parent.name + '-' + path.parent.name
    if (path.parent / 'Containerfile').is_symlink():
        raise ValueError(f'{path}: source recipe must be an owned regular file')
    recipe = (path.parent / 'Containerfile').read_text().rsplit('FROM ', 1)[-1]
    if not re.match(r'[^\n]+ AS final\n', recipe) or not re.search(r'^LABEL [^\n]*\borg\.opencontainers\.image\.title="' + re.escape(image) + r'"(?:\s|$)', recipe, re.M):
        raise ValueError(f'{path}: source image requires matching final-stage title')
    return image, ''


def pinned_metadata(path, reference):
    if not re.search(r'@sha256:[a-f0-9]{64}$', reference):
        raise ValueError('publication must provide a real digest reference')
    _, previous = image_reference(path)
    text = path.read_text()
    if not previous:
        return text.replace('  image: Containerfile\n', '  image: docker://' + reference + '\n', 1)
    return IMAGE.sub(lambda match: match[0].replace(match[1], reference), text)


def source_path(path):
    return Path(path).suffix.lower() not in ('.md', '.markdown') and Path(path).name != 'action.yml'


def release_baseline(head, root=ROOT):
    """Find the latest reachable stable release; never fetch."""
    def local_git(*args):
        return subprocess.check_output(['git', *args], cwd=root, text=True).strip()
    if local_git('rev-parse', '--is-shallow-repository') != 'false':
        raise ValueError('release-baseline selection requires complete Git history (fetch-depth: 0)')
    for tag in local_git('tag', '--merged', head, '--sort=-version:refname').splitlines():
        if not SEMVER.fullmatch(tag) or '-' in tag or '+' in tag:
            continue
        base = local_git('rev-parse', '--verify', '--end-of-options', 'refs/tags/' + tag + '^{commit}')
        return base
    return ''


def unreleased_changes(head, root=ROOT):
    """Return source-baseline paths and conservative rebuild flag; never fetch."""
    base = release_baseline(head, root)
    if not base:
        return [], True
    data = subprocess.check_output(['git', 'diff', '--name-only', '--no-renames', '-z', base, head, '--'], cwd=root)
    return [p.decode() for p in data.split(b'\0') if p], False


def select(paths, full=False, root=ROOT, unreleased=(), rebuild_selected=False, baseline=''):
    actions, owners = discover(root)
    ignore_paths = {'.containerignore', '.dockerignore'}
    ignore_impacts = {image: True for image in owners}
    if baseline and ignore_paths.intersection([*paths, *unreleased]):
        change = ignore_change(root, baseline, unreleased)
        ignore_impacts = {image: ignore_affects(root, directory, change) for image, directory in owners.items()}
    images, selected_actions = set(), set()
    if full:
        images.update(owners)
        selected_actions.update(actions)
    for path in paths:
        if Path(path).suffix.lower() in ('.md', '.markdown'):
            continue
        if path == 'grype-review.json' or path.startswith('shared/review/'):
            selected_actions.update(actions)
        if path in (".containerignore", ".dockerignore"):
            images.update(image for image in owners if ignore_impacts[image])
            selected_actions.update(actions)
            continue
        for directory, image in actions.items():
            if source_path(path) and (path == directory + '/Containerfile' or build_dependency(root, directory, path)):
                images.add(image)
                selected_actions.add(directory)
                continue
            if path.startswith(('.github/workflows/', 'tests/fixtures/', 'scripts/', 'tasks/')) or path == 'taskfile.yml':
                selected_actions.add(directory)
            if path.startswith(directory + "/"):
                selected_actions.add(directory)
    for directory, image in actions.items():
        if image in images:
            selected_actions.add(directory)
    # Reuse metadata-only selections only when their source is still released.
    matrix = []
    for image in sorted({actions[a] for a in selected_actions} | images):
        _, reference = image_reference(root / owners[image] / 'action.yml')
        stale = any(source_path(path) and (ignore_impacts[image] if path in ignore_paths else
                    path == owners[image] + '/Containerfile' or build_dependency(root, owners[image], path)) for path in unreleased)
        rebuild = image in images or rebuild_selected or stale or not reference or reference.endswith(':unpublished')
        scan = rebuild or full or any(path == 'grype-review.json' or path.startswith('shared/review/') for path in paths)
        for arch in ("amd64", "arm64"):
            matrix.append({"image": image, "directory": owners[image], "arch": arch,
                           "runner": "ubuntu-24.04" if arch == "amd64" else "ubuntu-24.04-arm",
                            "rebuild": rebuild, "scan": scan})
    if len(matrix) > 256:
        raise ValueError("GitHub supports at most 256 matrix jobs")
    return {"include": matrix}


def build_matrix(tests):
    builds = [dict(job) for job in tests['include'] if job['rebuild'] and not job.get('artifact-id')]
    reviewers = {job['arch'] for job in builds if job['image'] == 'workflow-new-vulnerability-review'}
    for job in tests['include']:
        if (job.get('scan', False) or job['rebuild']) and job['arch'] not in reviewers:
            builds.append({'image': 'workflow-new-vulnerability-review',
                           'directory': 'workflow/new-vulnerability-review',
                           'arch': job['arch'], 'runner': job['runner'],
                           'rebuild': True, 'reference': '', 'scan': False, 'dependency': True})
            reviewers.add(job['arch'])
    return {'include': builds}


def ignore_change(root, baseline, changed):
    paths = [p for p in changed if p in ('.containerignore', '.dockerignore')]
    if not paths:
        return [], [], []
    def tree(revision):
        data = subprocess.check_output(['git', 'ls-tree', '-r', '--name-only', '-z', revision], cwd=root)
        return {p.decode() for p in data.split(b'\0') if p}
    before = tree(baseline)
    patterns = []
    for name in ('.containerignore', '.dockerignore'):
        if name in before:
            patterns = subprocess.check_output(['git', 'show', baseline+':'+name], cwd=root, text=True).splitlines()
            break
    return paths, before | tree('HEAD'), patterns


def ignore_affects(root, directory, change):
    paths, files, patterns = change
    if not paths:
        return False
    if '$' in (root / directory / 'Containerfile').read_text():
        return True  # Unknown context expansion remains conservative.
    for path in paths:
        if build_dependency(root, directory, path, patterns) or build_dependency(root, directory, path):
            return True
    return any(build_dependency(root, directory, path, patterns) != build_dependency(root, directory, path) for path in files)


def build_dependency(root, directory, changed, ignore_patterns=None):
    """Mirror context COPY/ADD and bind dependencies, including deleted files."""
    for ignore in ('.containerignore', '.dockerignore'):
        file = root / ignore
        if ignore_patterns is not None or file.exists():
            ignored = False
            for pattern in (ignore_patterns if ignore_patterns is not None else file.read_text().splitlines()):
                pattern = pattern.strip()
                if not pattern or pattern.startswith('#'):
                    continue
                negate = pattern.startswith('!')
                pattern = pattern.lstrip('!').strip('/')
                if any(fnmatch.fnmatchcase(p, pattern) for p in [changed, *[str(p) for p in Path(changed).parents]]):
                    ignored = not negate
            if ignored:
                return False
            break
    recipe = root / directory / 'Containerfile'
    if not recipe.exists():
        return False
    sources = []
    for line in recipe.read_text().replace('\\\n', ' ').splitlines():
        command, _, rest = line.strip().partition(' ')
        if command.upper() in ('COPY', 'ADD'):
            if re.search(r'^--from(?:=|\s)', rest):
                continue
            rest = re.sub(r'^(?:--[^\s]+\s+)+', '', rest)
            words = json.loads(rest) if rest.startswith('[') else shlex.split(rest)
            sources.extend(s for s in words[:-1] if '://' not in s and not s.startswith(('git@', '<<')))
        elif command.upper() == 'RUN':
            for mount in re.findall(r'--mount=([^\s]+)', rest):
                values = dict(part.split('=', 1) for part in mount.split(',') if '=' in part)
                if not values.get('from') and values.get('type', 'bind') == 'bind':
                    sources.append(values.get('source', values.get('src', '.')))
    for source in sources:
        source = source.removeprefix('./').strip('/')
        if '$' in source or source in ('', '.') or changed == source or changed.startswith(source + '/'):
            return True
        if any(fnmatch.fnmatchcase(p, source) for p in [changed, *[str(p) for p in Path(changed).parents]]):
            return True
    return False


def changed_paths(before, after):
    # --no-renames intentionally exposes BOTH names as delete/add; deleted owners
    # are absent from discovery and are therefore never scheduled for rebuilding.
    data = subprocess.check_output(["git", "diff", "--name-only", "--no-renames", "-z", before, after], cwd=ROOT)
    return [p.decode() for p in data.split(b"\0") if p]


def mode(event, ref, qualification=False):
    if qualification:
        if event != 'workflow_dispatch' or ref != 'refs/heads/main':
            raise ValueError('full qualification must be dispatched on main')
        return 'qualification'
    if event == "schedule":
        return "maintenance"
    if event == "workflow_dispatch":
        if ref != "refs/heads/main":
            raise ValueError("release preparation must target main")
        return "prepare"
    if ref.startswith("refs/tags/"):
        tag = ref.removeprefix("refs/tags/")
        if not SEMVER.fullmatch(tag):
            raise ValueError(f"tag is not SemVer without v: {tag}")
        return "release"
    return "development"


def aggregate(paths):
    result = {}
    for path in sorted(paths):
        item = json.loads(Path(path).read_text())
        key = (item["image"], item["arch"])
        if key in result:
            raise ValueError(f"duplicate matrix result: {key}")
        archive = Path(path).parent / "image.oci.tar"
        with archive.open("rb") as stream:
            actual = hashlib.file_digest(stream, "sha256").hexdigest()
        if actual != item["archive_sha256"]:
            raise ValueError(f"archive checksum mismatch: {archive}")
        if inspect_archive(archive)["arch"] != item["arch"]:
            raise ValueError(f"archive architecture does not match result: {archive}")
        if not item.get("tested"):
            raise ValueError(f"untested result: {path}")
        result[key] = item
    for image in {key[0] for key in result}:
        if {key[1] for key in result if key[0] == image} != {"amd64", "arm64"}:
            raise ValueError(f"both tested architectures required for {image}")
    return [result[key] for key in sorted(result)]


def emit(name, value):
    if not isinstance(value, str):
        value = json.dumps(value, separators=(",", ":"))
    print(f"{name}={value}")
    if os.environ.get("GITHUB_OUTPUT"):
        with open(os.environ["GITHUB_OUTPUT"], "a") as stream:
            stream.write(f"{name}={value}\n")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("command", choices=("select", "validate", "aggregate"))
    parser.add_argument("paths", nargs="*")
    parser.add_argument('--allow-source', action='store_true', help='allow unpublished source during bootstrap qualification only')
    args = parser.parse_args()
    if args.command == "aggregate":
        print(json.dumps(aggregate(args.paths), indent=2))
        return
    actions, owners = discover()
    if args.command == 'validate' or os.environ.get('GITHUB_REF', '').startswith('refs/tags/'):
        for directory in owners.values():
            _, reference = image_reference(ROOT / directory / 'action.yml')
            if not re.search(r'@sha256:[a-f0-9]{64}$', reference):
                if not args.allow_source or os.environ.get('GITHUB_REF', '').startswith('refs/tags/'):
                    raise ValueError(f'{directory}: released catalogue requires digest pins; qualification may use --allow-source')
    if args.command == "validate":
        for name, directory in owners.items():
            text = (ROOT / directory / "Containerfile").read_text()
            final = False
            stages = set()
            for line in text.splitlines():
                if line.startswith('FROM '):
                    words = line.split()
                    validate_base(words[1], stages, name)
                    if len(words) == 4 and words[2].upper() == 'AS':
                        stages.add(words[3])
                        final = words[3] == 'final'
                    else:
                        raise ValueError(f"{name}: require named stages")
                if line.startswith('COPY '):
                    for field in line.split()[1:]:
                        if field.startswith('--from='):
                            validate_base(field.removeprefix('--from='), stages, name)
                if line.startswith('RUN '):
                    for field in line.split()[1:]:
                        if field.startswith('--mount='):
                            for option in field.removeprefix('--mount=').split(','):
                                if option.startswith('from='):
                                    validate_base(option.removeprefix('from='), stages, name)
            if not final:
                raise ValueError(f"{name}: last stage must be named final")
        print(f"Validated {len(actions)} action/image contracts and {len(owners)} image definitions")
        return
    event = os.environ.get("GITHUB_EVENT_NAME", "push")
    ref = os.environ.get("GITHUB_REF", "refs/heads/main")
    pipeline_mode = mode(event, ref, os.environ.get('QUALIFY_CATALOG') == 'true')
    after = os.environ.get("GITHUB_SHA") or git("rev-parse", "HEAD")
    paths = args.paths
    full = pipeline_mode in ("prepare", "qualification")
    plan_path = ROOT / '.build/publication/release-plan.json'
    if pipeline_mode == 'prepare' and plan_path.exists():
        plan = json.loads(plan_path.read_text())
        if plan['base'] != after:
            raise ValueError('release plan does not match checkout')
        if not plan['changed']:
            full, paths = False, []
        elif plan['previous']:
            full = False
            paths = changed_paths(plan['previous'], after)
    if pipeline_mode == "development" and not paths:
        payload = json.loads(Path(os.environ["GITHUB_EVENT_PATH"]).read_text()) if os.environ.get("GITHUB_EVENT_PATH") else {}
        before = payload.get("before", "")
        if not before or set(before) == {"0"}:
            try:
                before = git("merge-base", after, "origin/main")
                # First repository push has no prior release or usable base.
                full = before == after
            except subprocess.CalledProcessError:
                full = True
        if not full:
            paths = changed_paths(before, after)
    matrix = {"include": []}
    if pipeline_mode in ('development', 'prepare', 'qualification'):
        unreleased, rebuild_selected = ([], False) if full else unreleased_changes(after)
        baseline = '' if full else release_baseline(after)
        matrix = select(paths, full, unreleased=unreleased, rebuild_selected=rebuild_selected, baseline=baseline)
    if pipeline_mode == 'prepare' and os.environ.get('ARTIFACT_RUN_ID'):
        # The workflow validates the source run before selecting this empty matrix.
        matrix = {'include': []}
    emit("mode", pipeline_mode)
    emit("matrix", matrix)
    builds = build_matrix(matrix)
    emit("build-matrix", builds)
    emit("has-build", str(bool(builds['include'])).lower())
    emit("has-work", str(bool(matrix["include"])).lower())
    emit("version", "0.0.0-sha-"+after[:12] if pipeline_mode != "release" else ref.removeprefix("refs/tags/"))


if __name__ == "__main__":
    main()
