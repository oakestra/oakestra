import pathlib
import unittest

from sla.versioned_sla_parser import SLAFormatError, parse_sla_json

TEST_SLAS_PATH = pathlib.Path.cwd() / "tests" / "service_level_agreements"


# Note: helper functions for unit tests need to start with a different prefix than "test_"
def aux_test_correct_sla(self: unittest.TestCase, correct_test_sla_id: int) -> None:
    with open(TEST_SLAS_PATH / f"sla_correct_{correct_test_sla_id}.json", "r") as f:
        self.assertTrue(parse_sla_json(f.read()))


def aux_test_flawed_sla(
    self: unittest.TestCase, flawed_test_sla_id: int, exception_type: BaseException
) -> None:
    with open(TEST_SLAS_PATH / f"sla_flawed_{flawed_test_sla_id}.json", "r") as f:
        try:
            parse_sla_json(f.read())
        except exception_type:
            return
        self.fail("Exception expected")


class SLATestCase(unittest.TestCase):
    def test_correct_1(self):
        aux_test_correct_sla(self, 1)

    def test_correct_2(self):
        aux_test_correct_sla(self, 2)

    def test_correct_3(self):
        aux_test_correct_sla(self, 3)

    def test_correct_4(self):
        aux_test_correct_sla(self, 4)

    def test_flawed_1(self):
        aux_test_flawed_sla(self, 1, KeyError)

    def test_flawed_2(self):
        aux_test_flawed_sla(self, 2, SLAFormatError)

    def test_flawed_3(self):
        aux_test_flawed_sla(self, 3, KeyError)

    def test_flawed_4(self):
        aux_test_flawed_sla(self, 4, SLAFormatError)

    def test_flawed_5(self):
        aux_test_flawed_sla(self, 5, SLAFormatError)
