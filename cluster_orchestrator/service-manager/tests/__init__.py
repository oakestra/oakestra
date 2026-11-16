import sys
import unittest.mock

sys.modules["interfaces.mongodb_requests"] = unittest.mock.Mock()
mongodb_client = sys.modules["interfaces.mongodb_requests"]
