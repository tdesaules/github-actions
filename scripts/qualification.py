"""Conservative, dependency-free reader for the catalogue's indented workflows.

The Go action parses YAML fully. Local tooling recognizes the repository's fixed
job/step layout; unsupported layouts select all rather than omit qualification.
"""
import json
import re
import subprocess


def parse(text):
    lines = text.splitlines()
    if 'jobs:' not in lines or any(re.match(r'^\s*-\s*\{', line) for line in lines):
        return None
    jobs = {}
    name = None
    jobs_index = lines.index('jobs:')
    for index, line in enumerate(lines):
        match = re.match(r'^  ([a-z][a-z0-9_-]*):\s*$', line)
        if match and index > jobs_index:
            name = match[1]
            jobs[name] = []
        elif name:
            jobs[name].append(line)
    if 'test' not in jobs:
        return None
    context, steps, current = [], [], None
    for line in jobs['test']:
        if line.startswith('      - '):
            current = [line]
            steps.append(current)
        elif current is not None:
            current.append(line)
        else:
            context.append(line)
    normalize = lambda rows: '\n'.join(line.rstrip() for line in rows if line.strip() and not line.lstrip().startswith('#'))
    globals_ = []
    keep = False
    for line in lines[:lines.index('jobs:')]:
        if re.match(r'^[a-zA-Z]', line):
            keep = any(line.startswith(key+':') for key in ('env', 'defaults', 'permissions'))
        if keep:
            globals_.append(line)
    return normalize(globals_), normalize(jobs.get('build', [])), normalize(context), [normalize(step) for step in steps]


def applies(step, image):
    condition = re.search(r'^        if:\s*(.*)$', step, re.M)
    if not condition:
        return True
    value = condition[1].strip().strip('"')
    value = value.removeprefix('${{').removesuffix('}}').strip()
    match = re.fullmatch(r"matrix\.image == '([^']+)'", value)
    if match:
        return match[1] == image
    match = re.fullmatch(r"contains\(fromJSON\('([^']+)'\), matrix\.image\)", value)
    if match:
        try:
            names = json.loads(match[1])
            if isinstance(names, list) and all(isinstance(name, str) for name in names):
                return image in names
        except ValueError:
            pass
    return True


def select(root, actions, paths, before, build_dependency):
    selected = set()
    source = root / '.github/workflows/main.yml'
    current = parse(source.read_text()) if source.exists() else None
    if current is None:
        if any((source.exists() or p.startswith(('.github/workflows/', 'tests/fixtures/'))) and not p.endswith(('.md', '.markdown')) for p in paths):
            selected.update(actions)
        return selected
    changed = '.github/workflows/main.yml' in paths
    previous = None
    if changed and re.fullmatch('[a-f0-9]{40}', before):
        result = subprocess.run(['git', 'show', before+':.github/workflows/main.yml'], cwd=root, capture_output=True, text=True)
        if result.returncode == 0:
            previous = parse(result.stdout)
    for directory, image in actions.items():
        steps = [step for step in current[3] if applies(step, image)]
        old = [step for step in previous[3] if applies(step, image)] if previous else []
        if changed and (previous is None or current[:3] != previous[:3] or steps != old):
            selected.add(directory)
        for step in steps + old:
            if re.search(r'^        id:\s*security-', step, re.M) and re.search(r'^        continue-on-error:\s*true\s*$', step, re.M):
                continue
            match = re.search(r'^\s*(?:- )?uses:\s*([^\s]+)', step, re.M)
            helper = match[1].removeprefix('./') if match and match[1].startswith('./') else ''
            if helper.startswith('.candidate-actions/'):
                helper = ''
            for path in paths:
                if path.startswith('tests/fixtures/'):
                    references = re.findall(r'tests/fixtures/[A-Za-z0-9_./-]+', step)
                    if any(path == p.rstrip('/') or path.startswith(p.rstrip('/')+'/') for p in references):
                        selected.add(directory)
                if path.endswith(('.md', '.markdown')) or path in ('.containerignore', '.dockerignore'):
                    continue
                if helper and (path.startswith(helper+'/') or build_dependency(root, helper, path)):
                    selected.add(directory)
    return selected
