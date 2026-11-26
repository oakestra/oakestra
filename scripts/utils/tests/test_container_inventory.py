import importlib.util
import tempfile
import unittest
from pathlib import Path

MODULE_PATH = Path(__file__).parents[1] / "generateContainerInventory.py"
SPEC = importlib.util.spec_from_file_location("container_inventory", MODULE_PATH)
inventory = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(inventory)


def service(cluster="root", **fields):
    return {
        "labels": {"oakestra.cluster.id": cluster, "oakestra.logging.collector": "one-doc"},
        **fields,
    }


class InventoryTests(unittest.TestCase):
    def test_root_cluster_and_replicas(self):
        text = inventory.render_inventory(
            {
                "name": "oakestra",
                "services": {
                    "system_manager": service(),
                    "cluster_manager": service("edge-one", deploy={"replicas": 2}),
                },
            }
        )
        self.assertIn('cluster_id="root",compose_service="system_manager"', text)
        self.assertIn('cluster_id="edge-one",compose_service="cluster_manager"', text)
        self.assertIn('compose_project="oakestra"} 2', text)

    def test_disabled_and_unmanaged_services_are_not_expected(self):
        ignored = service()
        ignored["labels"]["oakestra.monitoring.ignore"] = "true"
        text = inventory.render_inventory(
            {
                "name": "oakestra",
                "services": {
                    "disabled": service(deploy={"replicas": 0}),
                    "scaled_down": service(scale=0),
                    "ignored": ignored,
                    "unmanaged": {"image": "anything"},
                },
            }
        )
        self.assertNotIn("oakestra_expected_container_replicas{", text)
        self.assertIn('oakestra_container_inventory_services{compose_project="oakestra"} 0', text)

    def test_profiles_are_explicit(self):
        config = {"name": "oakestra", "services": {"optional": service(profiles=["extra"])}}
        self.assertNotIn('compose_service="optional"', inventory.render_inventory(config))
        for profiles in [["extra"], ["*"]]:
            self.assertIn(
                'compose_service="optional"', inventory.render_inventory(config, profiles)
            )

    def test_label_escaping_and_no_secrets(self):
        config = {
            "name": "oakestra",
            "services": {
                "manager": service('edge"\\\n', environment={"PASSWORD": "do-not-export"})
            },
        }
        text = inventory.render_inventory(config)
        self.assertIn('cluster_id="edge\\"\\\\\\n"', text)
        self.assertNotIn("PASSWORD", text)
        self.assertNotIn("do-not-export", text)

    def test_default_resource_alert_thresholds(self):
        text = inventory.render_inventory({"name": "oakestra", "services": {}})
        self.assertIn('resource="cpu",severity="warning"} 80', text)
        self.assertIn('resource="cpu",severity="critical"} 90', text)
        self.assertIn('resource="memory_available",severity="warning"} 15', text)
        self.assertIn('resource="memory_available",severity="critical"} 10', text)
        self.assertIn('resource="disk_free",severity="warning"} 15', text)
        self.assertIn('resource="disk_free",severity="critical"} 10', text)

    def test_resource_alert_threshold_overrides(self):
        thresholds = inventory.parse_resource_thresholds(
            {
                "OAKESTRA_RESOURCE_CPU_WARNING_PERCENT": "72.5",
                "OAKESTRA_RESOURCE_CPU_CRITICAL_PERCENT": "91",
                "OAKESTRA_RESOURCE_MEMORY_WARNING_AVAILABLE_PERCENT": "20",
                "OAKESTRA_RESOURCE_MEMORY_CRITICAL_AVAILABLE_PERCENT": "8",
                "OAKESTRA_RESOURCE_DISK_WARNING_FREE_PERCENT": "18",
                "OAKESTRA_RESOURCE_DISK_CRITICAL_FREE_PERCENT": "7.5",
            }
        )
        text = inventory.render_inventory(
            {"name": "oakestra", "services": {}}, resource_thresholds=thresholds
        )
        self.assertIn('resource="cpu",severity="warning"} 72.5', text)
        self.assertIn('resource="disk_free",severity="critical"} 7.5', text)

    def test_invalid_resource_alert_thresholds(self):
        invalid_environments = [
            {"OAKESTRA_RESOURCE_CPU_WARNING_PERCENT": "not-a-number"},
            {"OAKESTRA_RESOURCE_CPU_WARNING_PERCENT": "101"},
            {
                "OAKESTRA_RESOURCE_CPU_WARNING_PERCENT": "90",
                "OAKESTRA_RESOURCE_CPU_CRITICAL_PERCENT": "80",
            },
            {
                "OAKESTRA_RESOURCE_MEMORY_WARNING_AVAILABLE_PERCENT": "5",
                "OAKESTRA_RESOURCE_MEMORY_CRITICAL_AVAILABLE_PERCENT": "10",
            },
            {
                "OAKESTRA_RESOURCE_DISK_WARNING_FREE_PERCENT": "10",
                "OAKESTRA_RESOURCE_DISK_CRITICAL_FREE_PERCENT": "10",
            },
        ]
        for environ in invalid_environments:
            with self.subTest(environ=environ), self.assertRaises(ValueError):
                inventory.parse_resource_thresholds(environ)

    def test_invalid_types_fail_instead_of_emitting_partial_inventory(self):
        for config in [
            None,
            [],
            {"name": "oakestra"},
            {"name": "oakestra", "services": {"bad": None}},
            {"name": "oakestra", "services": {"bad": service(labels=[])}},
            {"name": "oakestra", "services": {"bad": service(scale=True)}},
            {"name": "oakestra", "services": {"bad": service(scale="2")}},
        ]:
            with self.assertRaises(TypeError):
                inventory.render_inventory(config)

    def test_invalid_values_fail_instead_of_emitting_partial_inventory(self):
        for config in [
            {},
            {"name": "oakestra", "services": {"bad": service(deploy={"replicas": -1})}},
            {"name": "oakestra", "services": {"bad": service("")}},
        ]:
            with self.assertRaises(ValueError):
                inventory.render_inventory(config)

    def test_atomic_file_replacement(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "inventory.prom"
            inventory.write_inventory(path, "previous\n")
            inventory.write_inventory(path, "replacement\n")
            self.assertEqual(path.read_text(), "replacement\n")
            self.assertEqual(list(Path(directory).iterdir()), [path])
            self.assertEqual(path.stat().st_mode & 0o777, 0o644)


if __name__ == "__main__":
    unittest.main()
