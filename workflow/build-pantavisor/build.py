"""Build the checkout's fixed KAS CI target and retain only bound deploy outputs."""
import importlib.util
import json
import os
from pathlib import Path
import shutil
import sys
import tempfile
import time

project = sys.argv[1]
cold = sys.argv[2] == 'true'
root = Path.cwd()
build_name = 'base' if project == 'k0s' else project
build = root / 'artifacts/build' / build_name
deploy = build / 'tmp/deploy/images' / ('ventuno-q' if project == 'system' else 'userspace-arm64')
output = root / 'artifacts/ci/deploy'
if (cold and build.exists()) or output.exists():
    raise SystemExit('Cold CI build requires new build and deploy directories')
if shutil.disk_usage(root).free < 8 * 1024**3:
    raise SystemExit('Less than 8 GiB free before the build')
os.environ.update(KAS_WORK_DIR=str(root), KAS_BUILD_DIR=str(build))
os.environ.pop('DOWNLOAD_CACHE', None)
os.environ.pop('SSTATE_CACHE', None)
command = ['kas', 'build', str(root / 'kas/ci.yml')]
git_home = tempfile.TemporaryDirectory(prefix='pantavisor-kas-git-')
git_config = Path(git_home.name) / 'gitconfig'
import subprocess
subprocess.run(['git', 'config', '--file', str(git_config), '--add', 'safe.directory', str(root)], check=True)
subprocess.run(['git', 'config', '--file', str(git_config), '--add', 'safe.directory', '/work'], check=True)
# KAS deliberately filters GIT_CONFIG_* variables. Its supported config input
# carries narrowly scoped checkout trust through its temporary HOME instead.
os.environ['GITCONFIG_FILE'] = str(git_config)
started = time.monotonic()
report = {'project': project, 'cold': cold, 'initialFreeBytes': shutil.disk_usage(root).free, 'success': False}
try:
    sys.path.insert(0, str(root))
    if project == 'system':
        from tools.pvbuild.provenance import run
        names = ['pantavisor-initramfs-ventuno-q.' + suffix for suffix in ('cpio.gz', 'spdx.tar.zst')]
        run(command, deploy, names)
    else:
        spec = importlib.util.spec_from_file_location('producer_provenance', root / 'build/provenance.py')
        provenance = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(provenance)
        config = json.loads((root / 'build/component.json').read_text())
        if config['name'] != project:
            raise ValueError('Project input does not match producer metadata')
        prefix = config['image'] + '-' + config['machine']
        names = [prefix + '.' + suffix for suffix in ('squashfs', 'tar.gz', 'manifest', 'spdx.tar.zst')]
        if project == 'core':
            names.append(prefix + '.core-release.json')
        provenance.run(root, deploy, names, command)
    output.mkdir(parents=True, exist_ok=False)
    for name in [*names, 'build-provenance.json']:
        shutil.copyfile(deploy / name, output / name)
    # Only this invocation's build directory is removed, after successful staging.
    shutil.rmtree(build)
    report['success'] = True
finally:
    report.update(durationSeconds=round(time.monotonic() - started, 3), finalFreeBytes=shutil.disk_usage(root).free)
    (root / 'artifacts/ci/build-report.json').write_text(json.dumps(report, sort_keys=True) + '\n')
    git_home.cleanup()
