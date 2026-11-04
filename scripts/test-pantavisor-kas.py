#!/usr/bin/env python3
"""Run a cold action build in a fresh copy of explicit producer source files."""
import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--source', type=Path, required=True)
parser.add_argument('--project', choices=('core', 'k0s', 'system'), required=True)
parser.add_argument('--output', type=Path, required=True)
parser.add_argument('--reference', default='localhost/github-actions/workflow-build-pantavisor:pantavisor-candidate')
parser.add_argument('--resume', action='store_true', help='Recover this fixture build and record separate non-cold evidence')
args = parser.parse_args()
output = args.output.resolve()
output.mkdir(parents=True, exist_ok=args.resume)
source = args.source.resolve()
checkout = output / 'source'
checkout.mkdir(mode=0o755, exist_ok=args.resume)
paths = subprocess.check_output(['git', 'ls-files', '-z', '--cached', '--others', '--exclude-standard'], cwd=source).decode().split('\0')
for name in ([] if args.resume else sorted(set(paths))):
    if not name or not (source / name).exists():
        continue
    destination = checkout / name
    destination.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(source / name, destination, follow_symlinks=False)
env = os.environ | {'GIT_AUTHOR_NAME': 'KAS Qualification', 'GIT_AUTHOR_EMAIL': 'fixture@example.invalid',
                   'GIT_COMMITTER_NAME': 'KAS Qualification', 'GIT_COMMITTER_EMAIL': 'fixture@example.invalid',
                   'GIT_CONFIG_GLOBAL': '/dev/null', 'GIT_CONFIG_NOSYSTEM': '1'}
for command in (() if args.resume else (['git', 'init', '-q', '-b', 'main'], ['git', 'add', '.'], ['git', 'commit', '-q', '-m', 'test: qualify KAS action runtime'])):
    subprocess.run(command, cwd=checkout, env=env, check=True)
identity = subprocess.check_output(['podman', 'image', 'inspect', args.reference, '--format', '{{.Id}}'], text=True).strip()
prefix = 'resume-' if args.resume else ''
if args.resume:
    previous = json.loads((output / 'environment.json').read_text())
    if previous['source'] != str(source) or previous['project'] != args.project:
        raise SystemExit('Resume must use the original fixture project')
    if subprocess.check_output(['git', 'status', '--porcelain'], cwd=checkout, env=env).strip():
        raise SystemExit('Resume fixture source is dirty')
(output / (prefix + 'environment.json')).write_text(json.dumps({'imageID': identity, 'source': str(source), 'project': args.project, 'cold': not args.resume}) + '\n')
with (output / (prefix + 'build.log')).open('w') as log:
    result = subprocess.run(['podman', 'run', '--rm', '--security-opt', 'label=disable',
                             '-v', f'{checkout}:/github/workspace', '-w', '/github/workspace',
                             args.reference, '--project=' + args.project, '--cold=' + str(not args.resume).lower()], stdout=log, stderr=subprocess.STDOUT)
print(f'KAS action exit={result.returncode}; evidence={output}', flush=True)
raise SystemExit(result.returncode)
