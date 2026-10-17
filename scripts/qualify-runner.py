#!/usr/bin/env python3
"""Compare outer Docker profiles; keep failures as evidence, not success."""
import json
from pathlib import Path
import subprocess

IMAGE = 'quay.io/buildah/stable@sha256:07e637ad7a1df37f44afea8287935b350224ac193d63e5cc96debb50a366853c'
PROFILES = {
    'default': [],
    'seccomp': ['--security-opt=seccomp=unconfined'],
    'apparmor': ['--security-opt=apparmor=unconfined'],
    'sys-admin': ['--cap-add=SYS_ADMIN'],
    'seccomp-apparmor': ['--security-opt=seccomp=unconfined', '--security-opt=apparmor=unconfined'],
    'sys-admin-seccomp': ['--cap-add=SYS_ADMIN', '--security-opt=seccomp=unconfined'],
    'sys-admin-apparmor': ['--cap-add=SYS_ADMIN', '--security-opt=apparmor=unconfined'],
    'sys-admin-seccomp-apparmor': ['--cap-add=SYS_ADMIN', '--security-opt=seccomp=unconfined', '--security-opt=apparmor=unconfined'],
    'privileged': ['--privileged'],
}


def main():
    root = Path.cwd()
    out = root / '.build/qualification'
    out.mkdir(parents=True, exist_ok=True)
    subprocess.run(['docker', 'pull', IMAGE], check=True)
    results = []
    for name, flags in PROFILES.items():
        archive = out / 'proof.tar'
        archive.unlink(missing_ok=True)
        cmd = ['docker', 'run', '--rm', *flags, '-v', f'{root}:/github/workspace',
               '-w', '/github/workspace', '--entrypoint=/bin/sh', IMAGE, 'tests/runner/build.sh']
        try:
            result = subprocess.run(cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=300)
            code, log = result.returncode, result.stdout
        except subprocess.TimeoutExpired as exc:
            code, log = 124, exc.stdout or b''
        (out / (name+'.log')).write_bytes(log)
        passed = code == 0 and archive.is_file()
        results.append({'profile': name, 'flags': flags, 'exit': code, 'build_and_export': passed})
        print(name, 'PASS' if passed else f'FAIL ({code})', flush=True)
    (out / 'profiles.json').write_text(json.dumps(results, indent=2)+'\n')


if __name__ == '__main__':
    main()
