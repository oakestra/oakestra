import json
import unittest
from unittest.mock import patch

from api.v1.apps_blueprint import applicationsblp
from bson import ObjectId
from flask import Flask


class BlueprintTestCase(unittest.TestCase):
    def setUp(self):
        self.app = Flask(__name__)
        self.app.register_blueprint(applicationsblp)
        self.client = self.app.test_client()

    @patch("api.v1.apps_blueprint.apps_db.find_apps")
    def test_get_all_apps(self, mock_find_apps):
        expected = [
            {"_id": "65d200f3812caeb85e21ee19", "application_name": "app1"},
            {"_id": "65d200f3812caeb85e21ee12", "application_name": "app2"},
        ]
        mock_find_apps.return_value = [
            {"_id": ObjectId("65d200f3812caeb85e21ee19"), "application_name": "app1"},
            {"_id": ObjectId("65d200f3812caeb85e21ee12"), "application_name": "app2"},
        ]

        response = self.client.get("/api/v1/applications/")
        response_data = json.loads(response.data)

        self.assertEqual(response.status_code, 200)
        self.assertEqual(response_data, expected)
        # Add more assertions to check the response data


if __name__ == "__main__":
    unittest.main()
