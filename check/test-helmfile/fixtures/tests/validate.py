"""Exercise the actual native toolchain inside the qualification image."""
import subprocess
for tool, args in [('helm', ['version', '--short']), ('helmfile', ['--version']), ('kyverno', ['version'])]:
    subprocess.run([tool, *args], check=True)
