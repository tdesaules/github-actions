"""Qualify the consumer helper interface inside the catalog candidate."""
import subprocess
import os

assert os.environ["CI_TOKEN"] == "fixture-only"
for tool in ['git', 'git-cliff']:
    subprocess.run([tool, "--version"], check=True)
