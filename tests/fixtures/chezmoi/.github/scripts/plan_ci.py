"""Qualify the consumer helper interface inside the catalog candidate."""
import subprocess
import os

assert os.environ["INPUT_MODE"] == "development"
for tool in ['git', 'git-cliff']:
    subprocess.run([tool, "--version"], check=True)
