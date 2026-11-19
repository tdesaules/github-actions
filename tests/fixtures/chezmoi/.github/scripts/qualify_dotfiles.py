"""Qualify the consumer helper interface inside the catalog candidate."""
import subprocess

for tool in ['git-cliff', 'chezmoi', 'jq']:
    subprocess.run([tool, "--version"], check=True)
