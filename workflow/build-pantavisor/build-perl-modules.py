"""Repackage the installed core modules with two checksum-pinned CPAN fixes."""
import hashlib
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import urllib.request

root = Path('/perl-backport')
root.mkdir()
package = root / 'package'
package.mkdir()
name = 'perl-modules-5.40'
status = subprocess.check_output(['dpkg-query', '-s', name], text=True)
records = {}
key = None
for line in status.splitlines():
    if line.startswith(' '):
        records[key] += '\n' + line
    else:
        key, value = line.split(': ', 1)
        records[key] = value
original = records['Version']
if original != '5.40.1-6+deb13u1':
    raise ValueError('Unexpected base Perl module version: ' + original)
for filename in subprocess.check_output(['dpkg-query', '-L', name], text=True).splitlines():
    source = Path(filename)
    if filename == '/.':
        continue
    target = package / filename.lstrip('/')
    if source.is_symlink():
        target.parent.mkdir(parents=True, exist_ok=True)
        target.symlink_to(source.readlink())
    elif source.is_dir():
        target.mkdir(parents=True, exist_ok=True)
    elif source.is_file():
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(source, target)

modules = [
    ('Archive-Tar-3.14', 'https://cpan.metacpan.org/authors/id/B/BI/BINGOS/Archive-Tar-3.14.tar.gz',
     '177ce0506124afbe3ef95836761fb0799c941b87b79bac489bddc6cda860d904'),
    ('podlators-v6.1.1', 'https://cpan.metacpan.org/authors/id/R/RR/RRA/podlators-v6.1.1.tar.gz',
     '709a7dc4a2259683eb2f89e085ec70cf743710d950510e489ec6ddca3eb80c68'),
]
for distribution, url, checksum in modules:
    archive = root / (distribution + '.tar.gz')
    with urllib.request.urlopen(url, timeout=120) as response:
        archive.write_bytes(response.read())
    if hashlib.sha256(archive.read_bytes()).hexdigest() != checksum:
        raise ValueError('Source checksum mismatch: ' + distribution)
    with tarfile.open(archive) as stream:
        stream.extractall(root, filter='data')
    source = root / distribution
    subprocess.run(['chown', '-R', '30000:30000', str(source)], check=True)
    prefix = ['setpriv', '--reuid=30000', '--regid=30000', '--clear-groups']
    env = os.environ | {'HOME': '/tmp'}
    for command in (['perl', 'Makefile.PL'], ['make'], ['make', 'test']):
        subprocess.run(prefix + command, cwd=source, env=env, check=True)
    # Replace the core copies, not a second vendor copy shadowing vulnerable code.
    shutil.copytree(source / 'blib/lib', package / 'usr/share/perl/5.40.1', dirs_exist_ok=True,
                    ignore=shutil.ignore_patterns('.exists'))
    license_file = next((source / p for p in ('LICENSE', 'LICENSE.md', 'README', 'README.md')
                         if (source / p).is_file()), None)
    if license_file is None:
        raise ValueError('Missing module license/readme: ' + distribution)
    destination = package / 'usr/share/doc' / name / distribution / 'copyright'
    destination.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(license_file, destination)

version = original + '+pantavisor1'
records['Version'] = version
records['Source'] = 'perl (' + version + ')'
records['Maintainer'] = 'Pantavisor build tools <noreply@github.com>'
for field in ('Status', 'Installed-Size', 'Config-Version'):
    records.pop(field, None)
control = package / 'DEBIAN'
control.mkdir()
(control / 'control').write_text(''.join(k + ': ' + v + '\n' for k, v in records.items()))
files = sorted(p for p in package.rglob('*') if p.is_file() and not p.is_symlink()
               and control not in p.parents)
(control / 'md5sums').write_text(''.join(hashlib.md5(p.read_bytes()).hexdigest() + '  ' +
                                      str(p.relative_to(package)) + '\n' for p in files))
Path('/packages').mkdir(exist_ok=True)
subprocess.run(['dpkg-deb', '--root-owner-group', '--build', str(package),
                '/packages/perl-modules.deb'], check=True)
