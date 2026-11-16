from unittest.mock import patch

import pytest

from ..clients import resource_aggregation


@pytest.fixture(autouse=True)
def candidates_mock():
    with patch.object(resource_aggregation, "candidate_operations") as mock:
        mock.get_candidates.return_value = []
        yield mock
