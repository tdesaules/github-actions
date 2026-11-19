"""Qualify the consumer helper interface inside the catalog candidate."""
import subprocess
import os

assert os.environ["GITHUB_TOKEN"] == "fixture-only"
for tool in ['chezmoi']:
    subprocess.run([tool, "--version"], check=True)
