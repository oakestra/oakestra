import unittest
import os.path
from sla.versioned_sla_parser import parse_sla_json


class SLATestCase(unittest.TestCase):
    def test_correct_1(self):
        f = open(os.path.join(os.getcwd(), "tests/service_level_agreements/sla_correct_1.json"), "r")
        self.assertTrue(parse_sla_json(f.read()))

    def test_correct_2(self):
        f = open(os.path.join(os.getcwd(), "tests/service_level_agreements/sla_correct_2.json"), "r")
        self.assertTrue(parse_sla_json(f.read()))

    def test_flawed_1(self):
        f = open(os.path.join(os.getcwd(), "tests/service_level_agreements/sla_flawed_1.json"), "r")
        try:
            parse_sla_json(f.read())
        except:
            return
        self.fail("Exception expected")

    def test_flawed_2(self):
        f = open(os.path.join(os.getcwd(), "tests/service_level_agreements/sla_flawed_2.json"), "r")
        try:
            parse_sla_json(f.read())
        except:
            return
        self.fail("Exception expected")

    def test_flawed_3(self):
        f = open(os.path.join(os.getcwd(), "tests/service_level_agreements/sla_flawed_3.json"), "r")
        try:
            parse_sla_json(f.read())
        except:
            return
        self.fail("Exception expected")

    def test_flawed_4(self):
        f = open(os.path.join(os.getcwd(), "tests/service_level_agreements/sla_flawed_4.json"), "r")
        try:
            parse_sla_json(f.read())
        except:
            return
        self.fail("Exception expected")
