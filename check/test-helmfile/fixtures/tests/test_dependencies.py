import unittest
import bcrypt
import yaml
from jsonschema import validate

class Contracts(unittest.TestCase):
    def test_environment(self):
        validate(yaml.safe_load("enabled: true"), {"type": "object", "required": ["enabled"]})
        self.assertTrue(bcrypt.checkpw(b"fixture", bcrypt.hashpw(b"fixture", bcrypt.gensalt(4))))
