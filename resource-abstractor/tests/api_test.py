import json
import unittest
from unittest.mock import patch

from api.v1.apps_blueprint import applicationsblp
from bson import ObjectId
from flask import Flask
from utils.json_encoder import MongoJSONEncoder


class BlueprintTestCase(unittest.TestCase):
    def setUp(self):
        self.app = Flask(__name__)
        self.app.json_encoder = MongoJSONEncoder
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

    @patch("services.hook_service.hooks_db.find_hooks", return_value=[])
    @patch("api.v1.apps_blueprint.apps_db.update_app")
    def test_patch_app_filters_by_user_id(self, mock_update_app, _):
        mock_update_app.return_value = None

        self.client.patch(
            "/api/v1/applications/65d200f3812caeb85e21ee19?userId=bob",
            json={"application_name": "app1"},
        )

        app_id, _, extra_filter = mock_update_app.call_args.args
        self.assertEqual(app_id, "65d200f3812caeb85e21ee19")
        self.assertEqual(extra_filter, {"userId": "bob"})


if __name__ == "__main__":
    unittest.main()
