#!/usr/bin/env python3
"""Discover components from actual files; select work without a central catalogue."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess

from oci_archive import inspect_archive

ROOT = Path(__file__).resolve().parents[1]
SEMVER = re.compile(r"^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*))*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$")
IMAGE = re.compile(r"^(?:  image|        BUILDER_IMAGE): docker://([^\s]+)$", re.M)


def git(*args):
    return subprocess.check_output(["git", *args], cwd=ROOT).decode().strip()


def discover(root=ROOT):
    actions = {}
    owners = {}
    for path in sorted([*root.glob("oci/*/action.yml"), *root.glob("check/*/action.yml"), *root.glob("lint/*/action.yml"), *root.glob("workflow/*/action.yml"), *root.glob("maintenance/*/action.yml")]):
        if path.parent.parent.name == 'workflow' and not (path.parent / 'Containerfile').exists():
            continue
        match = IMAGE.search(path.read_text())
        if not match:
            raise ValueError(f"{path}: missing Docker image reference")
        reference = match[1]
        image = reference.rsplit("/", 1)[1].split(":")[0].split("@")[0]
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


def select(paths, full=False, root=ROOT):
    actions, owners = discover(root)
    images, selected_actions = set(), set()
    if full:
        images.update(owners)
        selected_actions.update(actions)
    for path in paths:
        if Path(path).suffix.lower() in ('.md', '.markdown'):
            continue
        if path.startswith(".github/workflows/") or path == 'grype-review.json':
            selected_actions.update(actions)
        if path.startswith(("shared/", "scripts/", "tasks/")) or path in ("go.mod", "go.sum", ".containerignore", ".dockerignore", "taskfile.yml"):
            images.update(owners)
            selected_actions.update(actions)
        for directory, image in actions.items():
            if path.startswith(directory + "/"):
                selected_actions.add(directory)
                filename = path[len(directory) + 1:]
                if filename == "Containerfile" or filename == ".containerignore" or filename.endswith(('.go', '.mjs', '.cjs')) or filename.startswith(("src/", "dependencies.")):
                    images.add(image)
    for directory, image in actions.items():
        if image in images:
            selected_actions.add(directory)
    # Tests of an action-only change use its pinned image, not a rebuilt image.
    matrix = []
    for image in sorted({actions[a] for a in selected_actions} | images):
        reference = IMAGE.search((root / owners[image] / 'action.yml').read_text())[1]
        rebuild = image in images or reference.endswith(':unpublished')
        for arch in ("amd64", "arm64"):
            matrix.append({"image": image, "directory": owners[image], "arch": arch,
                           "runner": "ubuntu-24.04" if arch == "amd64" else "ubuntu-24.04-arm",
                           "rebuild": rebuild})
    if len(matrix) > 256:
        raise ValueError("GitHub supports at most 256 matrix jobs")
    return {"include": matrix}


def changed_paths(before, after):
    # --no-renames intentionally exposes BOTH names as delete/add; deleted owners
    # are absent from discovery and are therefore never scheduled for rebuilding.
    data = subprocess.check_output(["git", "diff", "--name-only", "--no-renames", "-z", before, after], cwd=ROOT)
    return [p.decode() for p in data.split(b"\0") if p]


def mode(event, ref):
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


def lint_files(paths, tracked, full=False, root=ROOT):
    """Classify the Git diff for explicit workflow conditions, without a work graph."""
    changed = set(paths)
    files = {name: [] for name in ('actionlint', 'hadolint', 'shellcheck',
                                 'yaml', 'toml', 'markdownlint', 'lychee', 'action-metadata')}
    config = {
        'markdownlint': {'.markdownlint.json'}, 'lychee': {'lychee.toml'},
        'hadolint': {'.hadolint.yaml', '.hadolint.yml'}, 'shellcheck': {'.shellcheckrc'},
    }
    for path in sorted(set(tracked)):
        source = root / path
        if not source.is_file():
            continue
        kinds = []
        suffix = source.suffix.lower()
        if suffix in ('.yml', '.yaml'):
            kinds.append('yaml')
            if path.startswith('.github/workflows/'):
                kinds.append('actionlint')
        if source.name in ('Containerfile', 'Dockerfile') or source.name.startswith(('Containerfile.', 'Dockerfile.')):
            kinds.append('hadolint')
        if suffix == '.toml':
            kinds.append('toml')
        if suffix in ('.md', '.markdown'):
            kinds.extend(('markdownlint', 'lychee'))
        if suffix in ('.sh', '.bash', '.ksh'):
            kinds.append('shellcheck')
        elif not suffix or suffix == '.script':
            with source.open('rb') as stream:
                if re.match(rb'^#!.*\b(sh|bash|dash|ksh)\b', stream.readline(200)):
                    kinds.append('shellcheck')
        if source.name == 'action.yml' and len(Path(path).parts) == 3 and Path(path).parts[0] in ('oci', 'check', 'lint', 'workflow', 'maintenance'):
            kinds.append('action-metadata')
        for kind in kinds:
            if full or path in changed or changed & config.get(kind, set()):
                files[kind].append(path)
    return files


def check_changes(paths, full=False):
    """Conditions for independent checks; deletions are deliberately included."""
    selected = dict.fromkeys(('catalog', 'go', 'python'), full)
    for path in paths:
        p = Path(path)
        if p.suffix.lower() in ('.md', '.markdown'):
            continue
        if path.startswith('.github/workflows/') or path in ('taskfile.yml', '.containerignore', '.dockerignore'):
            selected = dict.fromkeys(selected, True)
        if p.name in ('action.yml', 'Containerfile') or path.startswith('check/test-catalog/'):
            selected['catalog'] = True
        if p.suffix == '.go' or path in ('go.mod', 'go.sum') or path.startswith('check/test-go/'):
            selected['go'] = True
        if path.startswith(('scripts/', 'tests/unit/', 'check/test-python/')) or p.name in ('action.yml', 'Containerfile'):
            selected['python'] = True
        if path.startswith('shared/testsuite/'):
            selected['python'] = True
    return selected


def source_changes(after):
    full = os.environ.get('GITHUB_EVENT_NAME', 'push') != 'push' or os.environ.get('GITHUB_REF', '').startswith('refs/tags/')
    payload = json.loads(Path(os.environ['GITHUB_EVENT_PATH']).read_text()) if os.environ.get('GITHUB_EVENT_PATH') else {}
    before = payload.get('before', '')
    if not before or set(before) == {'0'}:
        full = True
    return ([], True) if full else (changed_paths(before, after), False)


def emit_source_changes(paths, full):
    tracked = subprocess.check_output(['git', 'ls-files', '-z'], cwd=ROOT).decode().split('\0')
    for name, files in lint_files(paths, [p for p in tracked if p], full=full).items():
        emit(name + '-files', files)
        emit(name + '-changed', str(bool(files)).lower())
    for name, changed in check_changes(paths, full).items():
        emit(name + '-changed', str(changed).lower())


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
    parser.add_argument("command", choices=("changes", "select", "validate", "aggregate"))
    parser.add_argument("paths", nargs="*")
    args = parser.parse_args()
    if args.command == 'changes':
        after = os.environ.get('GITHUB_SHA') or git('rev-parse', 'HEAD')
        paths, full = source_changes(after)
        emit_source_changes(paths, full)
        return
    if args.command == "aggregate":
        print(json.dumps(aggregate(args.paths), indent=2))
        return
    actions, owners = discover()
    if args.command == "validate":
        for name, directory in owners.items():
            text = (ROOT / directory / "Containerfile").read_text()
            if "FROM scratch AS final" not in text:
                raise ValueError(f"{name}: final stage must use scratch")
            stages = set()
            for line in text.splitlines():
                if line.startswith('FROM '):
                    words = line.split()
                    if words[1] != 'scratch' and words[1] not in stages and '@sha256:' not in words[1]:
                        raise ValueError(f"{name}: unpinned base")
                    if len(words) == 4 and words[2].upper() == 'AS':
                        stages.add(words[3])
        print(f"Validated {len(actions)} Docker actions and {len(owners)} scratch image definitions")
        return
    event = os.environ.get("GITHUB_EVENT_NAME", "push")
    ref = os.environ.get("GITHUB_REF", "refs/heads/main")
    pipeline_mode = mode(event, ref)
    after = os.environ.get("GITHUB_SHA") or git("rev-parse", "HEAD")
    paths = args.paths
    full = pipeline_mode == "prepare"
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
    matrix = select(paths, full) if pipeline_mode in ("development", "prepare") else {"include": []}
    if pipeline_mode == 'prepare' and os.environ.get('ARTIFACT_RUN_ID'):
        # The workflow validates the source run before selecting this empty matrix.
        matrix = {'include': []}
    emit("mode", pipeline_mode)
    emit("matrix", matrix)
    emit("has-work", str(bool(matrix["include"])).lower())
    emit("version", "0.0.0-sha-"+after[:12] if pipeline_mode != "release" else ref.removeprefix("refs/tags/"))


if __name__ == "__main__":
    main()
