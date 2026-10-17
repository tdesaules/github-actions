#!/usr/bin/env python3
"""Plan immutable releases and atomically push their commit and tag."""
import argparse
import json
import os
from pathlib import Path
import subprocess

from ci import ROOT, SEMVER, IMAGE, discover, git
from images import ENGINE, mount

CLIFF = 'docker.io/orhunp/git-cliff:2.14.2@sha256:1e696a381a1b0366bb6ccbb6bd4e50e73473fda88eab3a757b95e0d0d66c9531'


def cliff(*args):
    return subprocess.check_output([ENGINE, 'run', '--rm', '-v', mount(ROOT, '/repo', True),
                                    '-w', '/repo', CLIFF, '--config', 'cliff.toml', *args], text=True).strip()


def plan():
    base = git('rev-parse', 'HEAD')
    tags = [t for t in git('tag', '--merged', 'HEAD').splitlines() if SEMVER.fullmatch(t) and '-' not in t and '+' not in t]
    latest = max(tags, key=lambda t: tuple(map(int, t.split('.')))) if tags else None
    subjects = git('log', '--format=%s', (latest+'..HEAD') if latest else 'HEAD').splitlines()
    changed = any(not s.startswith('chore(release):') for s in subjects)
    version = cliff('--bumped-version') if changed else latest
    if not version or not SEMVER.fullmatch(version) or '-' in version or '+' in version:
        raise ValueError('git-cliff did not produce a stable SemVer')
    return {'base': base, 'previous': latest, 'version': version, 'changed': changed and version != latest}


def finish(directory):
    state = json.loads((directory / 'publication.json').read_text())
    plan_data = json.loads((directory / 'release-plan.json').read_text())
    version = plan_data['version']
    if version != state['version'] or not SEMVER.fullmatch(version):
        raise ValueError('publication and release plan differ')
    if not all(i.get('signed') for i in state['images'].values()):
        raise ValueError('release requires verified signatures')
    base = plan_data['base']
    if git('rev-parse', 'HEAD') != base:
        raise ValueError('checkout differs from tested release base')
    remote = git('ls-remote', 'origin', 'refs/heads/main').split()[0]
    if remote != base:
        raise ValueError('main advanced: retain publication artifacts and prepare again; no force push')
    if git('ls-remote', 'origin', 'refs/tags/'+version):
        raise ValueError('release tag already exists; validate it instead of moving it')
    if git('status', '--porcelain'):
        raise ValueError('release checkout must be clean')
    patch = directory / 'action-digests.patch'
    if patch.stat().st_size:
        subprocess.run(['git', 'apply', str(patch)], check=True)
    actions, _ = discover()
    for directory_name in actions:
        reference = IMAGE.search((ROOT / directory_name / 'action.yml').read_text())[1]
        if '@sha256:' not in reference:
            raise ValueError('all release references must be immutable')
    notes = cliff('--tag', version)
    (ROOT / 'CHANGELOG.md').write_text(notes+'\n')
    subprocess.run(['git', 'add', 'CHANGELOG.md', *[a+'/action.yml' for a in actions]], check=True)
    subprocess.run(['git', '-c', 'user.name=github-actions[bot]', '-c', 'user.email=41898282+github-actions[bot]@users.noreply.github.com',
                    'commit', '-m', 'chore(release): '+version], check=True)
    subprocess.run(['git', 'tag', version], check=True)
    # Atomic refs prevent a partial main/tag update. Non-fast-forward rejects races.
    subprocess.run(['git', 'push', '--atomic', 'origin', 'HEAD:refs/heads/main', 'refs/tags/'+version], check=True)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('command', choices=('plan', 'finish'))
    parser.add_argument('--directory', type=Path, default=ROOT / '.build/publication')
    args = parser.parse_args()
    if args.command == 'plan':
        result = plan()
        args.directory.mkdir(parents=True, exist_ok=True)
        (args.directory / 'release-plan.json').write_text(json.dumps(result, indent=2)+'\n')
        print(json.dumps(result))
        if os.environ.get('GITHUB_OUTPUT'):
            with open(os.environ['GITHUB_OUTPUT'], 'a') as stream:
                stream.write('release-version='+result['version']+'\n')
                stream.write('release-changed='+str(result['changed']).lower()+'\n')
    else:
        finish(args.directory.resolve())


if __name__ == '__main__':
    main()
