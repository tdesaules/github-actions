"""Run the System harness and export only explicitly public result files."""
from pathlib import Path
import json
import shutil
import sys

sys.path.insert(0, str(Path.cwd()))
from tools.pvbuild.build import boot_tools
from tools.pvbuild.integration import integration

bundle, output, reports = map(Path, sys.argv[1:])
reports.mkdir(parents=True, exist_ok=False)
try:
    modules = boot_tools(output.parent / (output.name + '-boot-tools'))
    integration(bundle, modules, output)
except Exception as error:
    (reports / 'action-failure.json').write_text(json.dumps({
        'phase': 'preparation-or-qualification', 'error': str(error),
    }, sort_keys=True) + '\n')
    (reports / 'action-failure.json').chmod(0o644)
    raise
finally:
    # Never copy keys, disks, rendered state or known_hosts. These files alone
    # are the public diagnostics contract of the current System harness.
    results = output / 'results'
    if results.is_dir():
        for source in results.iterdir():
            if source.is_file() and not source.is_symlink() and source.suffix in ('.json', '.log'):
                destination = reports / source.name
                shutil.copyfile(source, destination)
                destination.chmod(0o644)
    reports.chmod(0o755)
