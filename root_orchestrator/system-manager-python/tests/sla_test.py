import unittest
import os.path
from sla_parser import validate_json


class SLATestCase(unittest.TestCase):
    def test_correct_1(self):
        f = open(os.path.join(os.getcwd(), "service_level_agreements/sla_correct_1.json"), "r")
        self.assertTrue(validate_json(f.read()))

    def test_correct_2(self):
        f = open(os.path.join(os.getcwd(), "service_level_agreements/sla_correct_2.json"), "r")
        self.assertTrue(validate_json(f.read()))

    def test_flawed_1(self):
        f = open(os.path.join(os.getcwd(), "service_level_agreements/sla_flawed_1.json"), "r")
        self.assertFalse(validate_json(f.read()))

    def test_flawed_2(self):
        f = open(os.path.join(os.getcwd(), "service_level_agreements/sla_flawed_2.json"), "r")
        self.assertFalse(validate_json(f.read()))

    def test_flawed_3(self):
        f = open(os.path.join(os.getcwd(), "service_level_agreements/sla_flawed_3.json"), "r")
        self.assertFalse(validate_json(f.read()))

    def test_flawed_4(self):
        f = open(os.path.join(os.getcwd(), "service_level_agreements/sla_flawed_4.json"), "r")
        self.assertFalse(validate_json(f.read()))


if __name__ == '__main__':
    unittest.main()
