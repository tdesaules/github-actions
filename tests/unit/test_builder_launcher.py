import importlib.util
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch


SPEC = importlib.util.spec_from_file_location('builder_launcher', Path(__file__).resolve().parents[2] / 'shared/run-builder.py')
launcher = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(launcher)


class BuilderLauncherTests(unittest.TestCase):
    def test_spaces_secrets_output_and_failure_are_preserved(self):
        with tempfile.TemporaryDirectory(prefix='builder spaces ') as directory:
            output = Path(directory) / 'step output'
            output.touch()
            inputs = {'context': 'source with spaces', 'registry-password': 'secret $value;'}
            environment = {
                'BUILDER_OPERATION': 'build-image',
                'BUILDER_IMAGE': 'docker://example.invalid/buildah@sha256:'+'a'*64,
                'BUILDER_INPUTS': json.dumps(inputs),
                'GITHUB_WORKSPACE': directory,
                'GITHUB_OUTPUT': str(output),
            }
            with patch.dict(os.environ, environment), patch.object(launcher.subprocess, 'run') as run:
                run.return_value.returncode = 42
                with self.assertRaises(SystemExit) as error:
                    launcher.main()
                self.assertEqual(error.exception.code, 42)
                args = run.call_args.args[0]
                env = run.call_args.kwargs['env']
                self.assertIn(f'{Path(directory).resolve()}:/github/workspace', args)
                self.assertIn(f'{output.resolve()}:/github/output', args)
                self.assertIn('--security-opt=seccomp=unconfined', args)
                self.assertIn('--security-opt=apparmor=unconfined', args)
                self.assertNotIn('--privileged', args)
                self.assertNotIn(inputs['registry-password'], args)
                self.assertEqual(env['INPUT_REGISTRY-PASSWORD'], inputs['registry-password'])
                self.assertEqual(env['INPUT_CONTEXT'], inputs['context'])
                self.assertEqual(args[-1], 'build-image')

    def test_other_tools_cannot_use_exception(self):
        with patch.dict(os.environ, {'BUILDER_OPERATION': 'new-sbom'}), patch.object(launcher.subprocess, 'run') as run:
            with self.assertRaises(ValueError):
                launcher.main()
            run.assert_not_called()
