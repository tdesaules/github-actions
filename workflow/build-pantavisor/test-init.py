#!/usr/bin/env python3
"""Exercise the actual image PID-1 lifecycle with synthetic, non-firmware inputs."""
import argparse
import json
from pathlib import Path
import subprocess
import tempfile

PROVENANCE = r'''
import json
from pathlib import Path
import subprocess
import time

def run(root, deploy, names, command):
    # Deliberately do not invoke KAS: this fixture tests process supervision only.
    record = {'synthetic': True, 'pid1': Path('/proc/1/comm').read_text().strip(),
              'pidsMax': Path('/sys/fs/cgroup/pids.max').read_text().strip(),
              'spawned': 0, 'peakPids': 0, 'zombies': 0}
    children = []
    try:
        for _ in range(160):
            child = subprocess.run(['python3', '-c',
                                    'import os,time; child=os.fork(); '
                                    'print(child,flush=True) if child else None; '
                                    'time.sleep(.02 if child==0 else 0); os._exit(0)'],
                                   check=True, timeout=15, capture_output=True, text=True)
            children.append(int(child.stdout.strip()))
            record['spawned'] += 1
            record['peakPids'] = max(record['peakPids'], int(Path('/sys/fs/cgroup/pids.current').read_text()))
        deadline = time.monotonic() + 5
        while any(Path('/proc', str(pid)).exists() for pid in children) and time.monotonic() < deadline:
            time.sleep(.02)
    finally:
        for file in Path('/proc').glob('[0-9]*/stat'):
            try:
                record['zombies'] += file.read_text().split(') ', 1)[1].split()[0] == 'Z'
            except (FileNotFoundError, ProcessLookupError):
                pass
        record['pidsEvents'] = Path('/sys/fs/cgroup/pids.events').read_text()
        record['remainingChildren'] = [pid for pid in children if Path('/proc', str(pid)).exists()]
        (root / 'artifacts/ci/init-probe.json').write_text(json.dumps(record, indent=2) + '\n')
    if record['zombies'] or record['remainingChildren'] or record['spawned'] != 160:
        raise RuntimeError('PID 1 failed to reap orphaned build processes')
    deploy.mkdir(parents=True, exist_ok=True)
    for name in names:
        (deploy / name).write_text('SYNTHETIC INIT FIXTURE, NOT FIRMWARE\n')
    (deploy / 'build-provenance.json').write_text('{"synthetic":true}\n')
'''


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--reference', required=True)
    parser.add_argument('--platform', choices=('linux/amd64', 'linux/arm64'), default='linux/amd64')
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    args.output.parent.mkdir(parents=True, exist_ok=True)
    temporary_root = '/tmp/opencode' if Path('/tmp/opencode').is_dir() else None
    with tempfile.TemporaryDirectory(prefix='kas-init-', dir=temporary_root) as temporary:
        root = Path(temporary)
        (root / 'build').mkdir()
        (root / 'kas').mkdir()
        (root / 'build/provenance.py').write_text(PROVENANCE)
        (root / 'build/component.json').write_text(json.dumps({
            'name': 'core', 'image': 'synthetic', 'machine': 'userspace-arm64'}))
        (root / 'kas/ci.yml').write_text('header:\n  version: 16\n')
        (root / '.gitignore').write_text('artifacts/\n')
        subprocess.run(['git', 'init', '-q', str(root)], check=True)
        # Override engine defaults: the image itself must provide the reaper.
        cidfile = root / 'container.cid'
        base = ['podman', 'run', '--rm', '--platform', args.platform,
                '--pid=private', '--init=false',
                '--pids-limit=128', '--userns=keep-id', '--user=0:0',
                '--cidfile', str(cidfile),
                '-v', str(root) + ':/github/workspace:Z', args.reference]

        def execute(arguments):
            try:
                return subprocess.run(base + arguments, text=True,
                                      capture_output=True, timeout=180)
            finally:
                if cidfile.exists():
                    subprocess.run(['podman', 'rm', '-f', cidfile.read_text().strip()],
                                   capture_output=True, timeout=30)
                    cidfile.unlink()

        result = execute(['--project=core', '--cold=true'])
        record_path = root / 'artifacts/ci/init-probe.json'
        record = json.loads(record_path.read_text()) if record_path.exists() else {'synthetic': True}
        record.update(reference=args.reference, platform=args.platform, exit=result.returncode, passed=False)
        args.output.write_text(json.dumps(record, indent=2) + '\n')
        if result.returncode or record.get('zombies') != 0 or record.get('remainingChildren') or record.get('spawned') != 160:
            raise RuntimeError(result.stdout + result.stderr + '\n' + json.dumps(record))
        rejected = execute(['--project=invalid'])
        record['invalidProjectExit'] = rejected.returncode
        record['passed'] = rejected.returncode == 1 and 'project must be' in rejected.stderr
        args.output.write_text(json.dumps(record, indent=2) + '\n')
        if not record['passed']:
            raise RuntimeError('Init did not preserve the action error: ' + rejected.stderr)
        print(f'PASS: {args.platform}, 160 orphaned children reaped, no zombies, exit status preserved')


if __name__ == '__main__':
    main()
