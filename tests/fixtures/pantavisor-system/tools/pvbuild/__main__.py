"""Exercise the real tools required by System's checks without building Yocto."""
from pathlib import Path
import subprocess
import sys
import tempfile

if sys.argv[1:] != ['check']:
    raise SystemExit('expected the fixed check operation')

with tempfile.TemporaryDirectory() as temporary:
    root = Path(temporary)
    script = root / 'grub.cfg'
    script.write_text('set timeout=1\nmenuentry "fixture" { echo ready; }\n')
    subprocess.run(['grub-script-check', str(script)], check=True)
    subprocess.run(['grub-editenv', str(root / 'grubenv'), 'create'], check=True)
    subprocess.run(['grub-editenv', str(root / 'grubenv'), 'set', 'fixture=ready'], check=True)
    if 'fixture=ready' not in subprocess.check_output(['grub-editenv', str(root / 'grubenv'), 'list'], text=True):
        raise SystemExit('GRUB environment roundtrip failed')
    if 'GNU cpio' not in subprocess.check_output(['cpio', '--version'], text=True):
        raise SystemExit('GNU cpio required')
    subprocess.run(['git', 'init', '-q', str(root / 'repo')], check=True)
    subprocess.run(['bash', '-n'], input=b'echo "fixture"\n', check=True)
print('PASS: System action runtime dependencies')
