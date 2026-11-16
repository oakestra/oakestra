"""Make the cluster-manager root importable and stub the resource abstractor client.

resource_aggregation imports resource_abstractor_client at module level, which is not
needed to test the pure aggregation functions.
"""

import os
import sys
from unittest.mock import MagicMock

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
sys.modules.setdefault("resource_abstractor_client", MagicMock())
