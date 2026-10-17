#!/usr/bin/env python3
"""Launch only the Buildah operations with the qualified hosted Docker profile."""
import json
import os
from pathlib import Path
import subprocess
import sys


def main():
    operation = os.environ['BUILDER_OPERATION']
    if operation not in ('build-image', 'merge-image-index'):
        raise ValueError('unsupported builder operation')
    reference = os.environ['BUILDER_IMAGE'].removeprefix('docker://')
    workspace = Path(os.environ['GITHUB_WORKSPACE']).resolve()
    output = Path(os.environ['GITHUB_OUTPUT']).resolve()
    env = os.environ.copy()
    command = ['docker', 'run', '--rm', '--security-opt=seccomp=unconfined',
               '--security-opt=apparmor=unconfined', '-v', f'{workspace}:/github/workspace',
               '-v', f'{output}:/github/output', '-w', '/github/workspace',
               '-e', 'GITHUB_OUTPUT=/github/output']
    for name, value in json.loads(os.environ['BUILDER_INPUTS']).items():
        variable = 'INPUT_'+name.upper()
        env[variable] = str(value)
        command += ['-e', variable]
    command += [reference, operation]
    # Do not print input values: registry passwords and build secrets may be present.
    sys.exit(subprocess.run(command, env=env).returncode)


if __name__ == '__main__':
    main()
