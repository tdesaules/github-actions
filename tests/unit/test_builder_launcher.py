"""Exercise the actual composite's narrow Docker launcher with a fake engine."""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import textwrap
import unittest


class BuilderLauncherTests(unittest.TestCase):
    def test_literal_arguments_scoped_flags_and_exit_status(self):
        metadata = Path(__file__).resolve().parents[2] / 'oci/build-image/action.yml'
        script = textwrap.dedent(metadata.read_text().split('      run: |\n', 1)[1])
        with tempfile.TemporaryDirectory(prefix='builder launcher ') as temporary:
            root = Path(temporary)
            engine = root/'docker'
            engine.write_text('#!'+sys.executable+'\nimport json,os,sys\n'
                              'open(os.environ["CAPTURE"],"w").write(json.dumps(sys.argv[1:]))\n'
                              'sys.exit(int(os.environ["ENGINE_EXIT"]))\n')
            engine.chmod(0o755)
            variables = {
                'PATH': str(root)+os.pathsep+os.environ['PATH'],
                'CAPTURE': str(root/'arguments.json'), 'ENGINE_EXIT': '23',
                'GITHUB_WORKSPACE': str(root/'workspace with spaces'),
                'GITHUB_OUTPUT': str(root/'output with spaces'),
                'BUILDER_IMAGE': 'docker://example.invalid/builder@sha256:'+'a'*64,
                'INPUT_CONTEXT': 'source with spaces', 'INPUT_CONTAINERFILE': 'Containerfile',
                'INPUT_IMAGE': 'example.invalid/test', 'INPUT_VERSION': '1.0.0',
                'INPUT_PLATFORM': 'linux/amd64', 'INPUT_TARGET': '',
                'INPUT_LABELS': '{}', 'INPUT_BUILD_ARGS': '{"value":"$(touch injected)"}',
                'INPUT_SECRETS': '["id=secret,src=private file"]', 'INPUT_OUTPUT': 'image archive.tar',
            }
            result = subprocess.run(['bash','-e','-u','-o','pipefail','-c',script], env=os.environ|variables, cwd=root)
            self.assertEqual(result.returncode,23)
            args = json.loads((root/'arguments.json').read_text())
            self.assertIn('--context=source with spaces',args)
            self.assertIn('--build-args={"value":"$(touch injected)"}',args)
            self.assertFalse((root/'injected').exists())
            self.assertIn('--security-opt=seccomp=unconfined',args)
            self.assertIn('--security-opt=apparmor=unconfined',args)
            self.assertNotIn('--privileged',args)
            self.assertNotIn('build-image',args)
            self.assertNotIn('/var/run/docker.sock', ' '.join(args))
            self.assertIn(str(root/'output with spaces')+':/github/output',args)
