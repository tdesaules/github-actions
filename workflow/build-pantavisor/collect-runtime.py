"""Collect installed KAS build-tool packages and dependencies, not the distro."""
import json
from pathlib import Path
import re
import shutil
import subprocess
import sys

runtime = Path('/runtime')
runtime.mkdir()
records = subprocess.check_output(['dpkg-query', '-W', '-f=${Package}\t${Depends}\t${Pre-Depends}\t${Provides}\n'], text=True)
rows = [line.split('\t') for line in records.splitlines()]
packages = {row[0]: row[1:3] for row in rows}
providers = {}
for row in rows:
    for item in row[3].split(','):
        name = re.split(r'[ :(<]', item.strip())[0]
        if name:
            providers.setdefault(name, row[0])
selected = set()
def visit(name):
    # Debian may replace a concrete package with a virtual provider (pkgconf).
    if name not in packages and name in providers:
        name = providers[name]
    if name in selected:
        return
    if name not in packages:
        raise ValueError('Missing installed build dependency: ' + name)
    selected.add(name)
    for group in ','.join(packages[name]).split(','):
        if not group.strip():
            continue
        choices = [re.split(r'[ :(<]', part.strip())[0] for part in group.split('|')]
        present = next((candidate if candidate in packages else providers[candidate]
                        for candidate in choices if candidate in packages or candidate in providers), None)
        if present is None:
            raise ValueError('Unresolved installed dependency: ' + group)
        visit(present)

for name in ('bash dash hostname coreutils findutils diffutils grep sed tar gzip bzip2 xz-utils zstd cpio pigz lz4 '
             'build-essential chrpath diffstat gawk file patch perl pkg-config rsync git '
             'wget curl ca-certificates locales python3 python3-yaml python3-jsonschema '
             'python3-kconfiglib python3-distro python3-colorlog python3-git python3-jinja2 '
             'python3-pexpect python3-ruamel.yaml python3-gnupg socat unzip util-linux procps texinfo').split():
    visit(name)
for name in ('gcc-multilib', 'g++-multilib'):
    if name in packages:
        visit(name)

copied = set()
def copy(source):
    source = Path(source)
    if not source.exists() and not source.is_symlink():
        return
    source = source.parent.resolve() / source.name
    if source in copied or (source.is_dir() and not source.is_symlink()):
        return
    copied.add(source)
    destination = runtime / source.relative_to('/')
    destination.parent.mkdir(parents=True, exist_ok=True)
    if source.is_symlink():
        target = source.readlink()
        if source.is_dir():
            # Perl's versioned include directories use symlink aliases. The
            # selected packages still control which target files are copied.
            (runtime / source.resolve().relative_to('/')).mkdir(parents=True, exist_ok=True)
        else:
            copy(target if target.is_absolute() else source.parent / target)
        destination.symlink_to(target, target_is_directory=source.is_dir())
    else:
        shutil.copy2(source, destination)

for name in sorted(selected):
    for filename in subprocess.check_output(['dpkg-query', '-L', name], text=True).splitlines():
        if filename.startswith('/usr/share/doc/') and not filename.endswith('/copyright'):
            continue
        if filename.startswith(('/usr/share/man/', '/usr/share/info/')):
            continue
        if filename.startswith('/'):
            copy(filename)
for source in ('/usr/local/bin/kas', '/etc/os-release', '/etc/nsswitch.conf', '/etc/passwd', '/etc/group', '/usr/lib/locale/locale-archive'):
    copy(source)
# update-alternatives creates aliases that do not occur in dpkg file lists.
# Retain only aliases whose resolved executable is already in the closure.
for directory in ('/usr/bin', '/usr/sbin'):
    for source in Path(directory).iterdir():
        if source.is_symlink() and source.resolve() in copied:
            copy(source)
python_library = f'usr/local/lib/python{sys.version_info.major}.{sys.version_info.minor}'
shutil.copytree('/' + python_library, runtime / python_library, dirs_exist_ok=True)
shutil.copytree('/etc/ssl/certs', runtime / 'etc/ssl/certs', dirs_exist_ok=True, symlinks=True)
for name in ('bin', 'sbin', 'lib', 'lib64'):
    source = Path('/') / name
    if source.is_symlink():
        destination = runtime / name
        if destination.is_symlink():
            if destination.readlink() != source.readlink():
                raise ValueError('Conflicting runtime alias: ' + name)
        else:
            destination.symlink_to(source.readlink())
for name in ('tmp', 'var/tmp'):
    (runtime / name).mkdir(parents=True, exist_ok=True)
    (runtime / name).chmod(0o1777)
(runtime / 'etc/pantavisor-action-packages.json').write_text(json.dumps(sorted(selected)) + '\n')
# Retain accurate package identities for Syft/Grype; omitting the package database
# would hide the build-tool dependencies from OS vulnerability matching.
status = []
for stanza in Path('/var/lib/dpkg/status').read_text().split('\n\n'):
    match = re.search(r'^Package: (.+)$', stanza, re.M)
    if match and match[1] in selected:
        status.append(stanza)
(runtime / 'var/lib/dpkg').mkdir(parents=True, exist_ok=True)
(runtime / 'var/lib/dpkg/status').write_text('\n\n'.join(status) + '\n')
# Yocto's uninative loader is not Debian's multiarch loader. Its native tools
# also need the minimal root's cache to find host libraries such as libgomp.
subprocess.run(['ldconfig', '-r', str(runtime)], check=True)
