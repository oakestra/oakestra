import sys
import unittest.mock

sys.modules["interfaces.mongodb_requests"] = unittest.mock.Mock()
mongodb_client = sys.modules["interfaces.mongodb_requests"]
mongodb_client.mongo_get_cluster_id = unittest.mock.MagicMock(return_value="1")
