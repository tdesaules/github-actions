#!/usr/bin/env python3
"""Check the rebuilt interpreter and the missing-hostname TLS regression."""
import argparse
import subprocess

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--reference', required=True)
parser.add_argument('--platform', default='linux/amd64')
args = parser.parse_args()

program = '''
import bz2, ctypes, hashlib, lzma, pyexpat, sqlite3, ssl, sys
from html.parser import HTMLParser
from xml.etree import ElementTree
assert sys.version_info[:3] == (3, 13, 18), sys.version
assert pyexpat.version_info >= (2, 8, 0), pyexpat.EXPAT_VERSION
context = ssl.create_default_context()
assert context.check_hostname
try:
    context.wrap_bio(ssl.MemoryBIO(), ssl.MemoryBIO())
except ValueError as error:
    assert 'server_hostname' in str(error), str(error)
else:
    raise AssertionError('CVE-2026-19553: missing hostname silently accepted')
connection = context.wrap_bio(ssl.MemoryBIO(), ssl.MemoryBIO(), server_hostname='localhost')
assert connection.server_hostname == 'localhost'
assert ElementTree.fromstring('<root>verified</root>').text == 'verified'
assert sqlite3.connect(':memory:').execute('select 42').fetchone() == (42,)
HTMLParser().feed('<html><body>verified</body></html>')
print('PASS Python 3.13.18, extension ABI, Expat baseline and TLS missing-hostname rejection')
'''
subprocess.run(['podman', 'run', '--rm', '--platform', args.platform,
                '--entrypoint', '/usr/bin/python3', args.reference, '-c', program], check=True)
