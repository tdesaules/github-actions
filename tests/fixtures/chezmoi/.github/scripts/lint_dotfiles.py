"""Qualify the consumer helper interface inside the catalog candidate."""
import subprocess

for tool in ['nu', 'niri', 'chezmoi', 'shellcheck']:
    subprocess.run([tool, "--version"], check=True)
