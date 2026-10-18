#!/usr/bin/env python3
"""Exercise each specialized check against real valid/invalid read-only files."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess

from images import ENGINE, ROOT, local_ref, mount


def prepare(directory):
    directory.mkdir(parents=True, exist_ok=True)
    fixtures = {
        'workflow valid.yml': 'name: fixture\non: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo hello\n',
        'workflow invalid.yml': 'on: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo "${{ unknown.value }}"\n',
        'Containerfile valid': 'FROM scratch\nCOPY hello /hello\nUSER 65532:65532\n',
        'Containerfile invalid': 'FROM ubuntu:latest\nRUN apt-get update\n',
        'script without extension': '#!/bin/sh\nvalue="hello world"\nprintf "%s\\n" "$value"\n',
        'script invalid.script': '#!/bin/sh\necho $undefined_variable\n',
        'valid stream.yaml': 'name: first\n---\nname: second\n',
        'invalid.yaml': 'name: first\nname: duplicate\n',
        'valid config.toml': '[package]\nname = "hello world"\n',
        'invalid.toml': 'name = "first"\nname = "duplicate"\n',
    }
    for name, content in fixtures.items():
        (directory / name).write_text(content)
    return {
        'check-test-github-workflow': ('workflow valid.yml', 'workflow invalid.yml'),
        'check-test-containerfile': ('Containerfile valid', 'Containerfile invalid'),
        'check-test-shell': ('script without extension', 'script invalid.script'),
        'check-test-yaml': ('valid stream.yaml', 'invalid.yaml'),
        'check-test-toml': ('valid config.toml', 'invalid.toml'),
    }


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--image', required=True)
    parser.add_argument('--version', default='0.0.0-bootstrap')
    args = parser.parse_args()
    directory = ROOT / '.build/check-fixtures' / args.image
    valid, invalid = prepare(directory)[args.image]
    before = {p.name: hashlib.sha256(p.read_bytes()).hexdigest() for p in directory.iterdir()}
    command = [ENGINE, 'run', '--rm', '--read-only', '--user=65532:65532',
               '-v', mount(directory, '/fixtures', True), '-w', '/fixtures',
               local_ref(args.image, args.version)]
    for names, success in [([valid], True), ([invalid], False), ([valid, invalid], False),
                           ([], False), (['missing file'], False), ([valid, valid], False)]:
        result = subprocess.run([*command, '--files='+json.dumps(names)], capture_output=True, text=True)
        print(result.stdout, result.stderr)
        if (result.returncode == 0) != success:
            raise AssertionError(f'{args.image}: unexpected status for {names}: {result.returncode}')
    # Neither the image's own rootfs nor source permissions may hide read failures.
    unreadable = directory / valid
    unreadable.chmod(0)
    try:
        result = subprocess.run([*command, '--files='+json.dumps([valid])], capture_output=True)
        if result.returncode == 0:
            raise AssertionError('check ignored a source permission failure')
    finally:
        unreadable.chmod(0o644)
    after = {p.name: hashlib.sha256(p.read_bytes()).hexdigest() for p in directory.iterdir()}
    if before != after:
        raise AssertionError('checks modified their source files')


if __name__ == '__main__':
    main()
