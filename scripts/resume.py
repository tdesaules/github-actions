#!/usr/bin/env python3
"""Authorize reuse of a successful run's tested archives at the same main commit."""
import json
import os
import subprocess

from ci import emit


def validate(run, revision):
    if (run['head_sha'] != revision or run['head_branch'] != 'main'
            or run['path'] != '.github/workflows/main.yml'
            or run['event'] not in ('push', 'workflow_dispatch')
            or run['conclusion'] != 'success'):
        raise ValueError('artifact source must be a successful catalogue run at this exact main commit')


if __name__ == '__main__':
    identifier = os.environ['ARTIFACT_RUN_ID']
    if not identifier.isdecimal():
        raise ValueError('artifact run ID must be numeric')
    run = json.loads(subprocess.check_output(['gh', 'api', f"repos/{os.environ['GITHUB_REPOSITORY']}/actions/runs/{identifier}"]))
    validate(run, os.environ['GITHUB_SHA'])
    emit('artifact-pattern', f"oci-*-{identifier}-{run['run_attempt']}")
