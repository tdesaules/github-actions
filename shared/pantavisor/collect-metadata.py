"""Retain Debian package identities/licenses for files in a minimal tool closure."""
from pathlib import Path
import re
import shutil
import subprocess

runtime = Path('/runtime')
paths = {str((Path('/') / p.relative_to(runtime)).resolve()) for p in runtime.rglob('*') if p.is_file()}
packages = set()
for package in subprocess.check_output(['dpkg-query', '-W', '-f=${binary:Package}\n'], text=True).splitlines():
    files = subprocess.check_output(['dpkg-query', '-L', package], text=True).splitlines()
    if any(str(Path(name).resolve()) in paths for name in files if name.startswith('/') and Path(name).is_file()):
        packages.add(package.split(':')[0])
stanzas = []
for stanza in Path('/var/lib/dpkg/status').read_text().split('\n\n'):
    match = re.search(r'^Package: (.+)$', stanza, re.M)
    if match and match[1] in packages:
        stanzas.append(stanza)
        license_file = Path('/usr/share/doc') / match[1] / 'copyright'
        if license_file.is_file():
            destination = runtime / license_file.relative_to('/')
            destination.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(license_file, destination)
(runtime / 'var/lib/dpkg').mkdir(parents=True, exist_ok=True)
(runtime / 'var/lib/dpkg/status').write_text('\n\n'.join(stanzas) + '\n')
# Preserve ownership so binary catalogers recognize distribution backports.
info = runtime / 'var/lib/dpkg/info'
info.mkdir(parents=True, exist_ok=True)
for source in Path('/var/lib/dpkg/info').iterdir():
    package = source.name.rsplit('.', 1)[0].split(':')[0]
    if package in packages and source.suffix in ('.list', '.md5sums'):
        shutil.copyfile(source, info / source.name)
shutil.copyfile('/etc/os-release', runtime / 'etc/os-release')
