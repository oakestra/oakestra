import os
import subprocess
import tempfile
import unittest
from pathlib import Path

SCRIPTS = Path(__file__).parents[2]


class StartupTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)
        self.bin = self.root / "bin"
        self.bin.mkdir()
        self.commands = self.root / "docker-commands"
        self.environ = {
            **os.environ,
            "HOME": str(self.root),
            "PATH": str(self.bin) + os.pathsep + os.environ["PATH"],
            "OAKESTRA_VERSION": "main",
            "OVERRIDE_FILES": "",
            "SYSTEM_MANAGER_URL": "192.0.2.1",
            "CLUSTER_ADDRESS": "192.0.2.2",
            "CLUSTER_NAME": "test-cluster",
            "CLUSTER_LOCATION": "0,0,1000",
            "MOCK_DOCKER_LOG": str(self.commands),
            "MOCK_DOWNLOAD_EXIT": "0",
            "MOCK_COMPOSE_SERVICES": "system_manager",
            "MOCK_CONFIG_EXIT": "0",
            "MOCK_COMPOSE_JSON": '{"name": "test", "services": {}}',
            "MOCK_GENERATOR": "",
        }
        self.executable("sudo", '#!/bin/bash\n[ "$1" != "-E" ] || shift\nexec "$@"\n')
        self.executable(
            "docker",
            '#!/bin/bash\nprintf "%s\\n" "$*" >> "$MOCK_DOCKER_LOG"\n'
            'if [[ "$*" == *"config --services"* ]]; then\n'
            '    printf "%s\\n" "$MOCK_COMPOSE_SERVICES"\n'
            '    exit "$MOCK_CONFIG_EXIT"\n'
            'elif [[ "$*" == *"config --format json"* ]]; then\n'
            '    printf "%s\\n" "$MOCK_COMPOSE_JSON"\n'
            '    exit "$MOCK_CONFIG_EXIT"\n'
            "fi\n",
        )
        self.executable("jq", "#!/bin/bash\nexit 0\n")
        self.executable(
            "curl",
            "#!/usr/bin/env python3\n"
            "import os, sys\n"
            'url = next(arg for arg in sys.argv if arg.startswith("https://"))\n'
            'if url.endswith("/downloadConfigFiles.sh"):\n'
            '    print("#!/bin/bash")\n'
            '    if os.environ["MOCK_GENERATOR"]:\n'
            "        print('cp \"$MOCK_GENERATOR\" generateContainerInventory.py')\n"
            '    print("exit " + os.environ["MOCK_DOWNLOAD_EXIT"])\n'
            "else:\n"
            '    print("services: {}")\n',
        )

    def executable(self, name, content):
        path = self.bin / name
        path.write_text(content)
        path.chmod(0o755)

    def start(self, name):
        return subprocess.run(
            ["bash", str(SCRIPTS / name), "start", "custom"],
            cwd=self.root,
            env=self.environ,
            capture_output=True,
            text=True,
            timeout=10,
        )

    def docker_commands(self):
        return self.commands.read_text() if self.commands.exists() else ""

    def test_failed_configuration_download_cancels_every_startup(self):
        self.environ["MOCK_DOWNLOAD_EXIT"] = "31"
        for script in ["StartOakestraRoot.sh", "StartOakestraCluster.sh", "StartOakestraFull.sh"]:
            with self.subTest(script=script):
                self.commands.unlink(missing_ok=True)
                result = self.start(script)
                self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
                self.assertNotIn(" up ", self.docker_commands())

    def check_every_startup(self, succeeds):
        for script in ["StartOakestraRoot.sh", "StartOakestraCluster.sh", "StartOakestraFull.sh"]:
            with self.subTest(script=script, version=self.environ["OAKESTRA_VERSION"]):
                self.commands.unlink(missing_ok=True)
                result = self.start(script)
                self.assertEqual(
                    result.returncode, 0 if succeeds else 1, result.stdout + result.stderr
                )
                self.assertEqual(" up " in self.docker_commands(), succeeds)
                if succeeds and not self.environ["MOCK_GENERATOR"]:
                    self.assertIn("skipping inventory generation", result.stdout)
                if succeeds and self.environ["MOCK_GENERATOR"]:
                    directory = {
                        "StartOakestraRoot.sh": "root_orchestrator",
                        "StartOakestraCluster.sh": "cluster_orchestrator",
                        "StartOakestraFull.sh": "",
                    }[script]
                    inventory = (
                        self.root
                        / ".oakestra"
                        / directory
                        / "config/container-inventory/containers.prom"
                    )
                    self.assertTrue(inventory.is_file())
                    self.assertIn("oakestra_container_inventory_services", inventory.read_text())

    def test_legacy_main_and_tag_do_not_require_inventory(self):
        for version in ["main", "v0.4.411"]:
            self.environ["OAKESTRA_VERSION"] = version
            self.check_every_startup(succeeds=True)

    def test_lifecycle_deployment_requires_matching_generator(self):
        self.environ["MOCK_COMPOSE_SERVICES"] = "system_manager\ndocker_state_exporter"
        self.check_every_startup(succeeds=False)

    def test_lifecycle_deployment_generates_inventory(self):
        self.environ["MOCK_COMPOSE_SERVICES"] = "docker_state_exporter"
        self.environ["MOCK_GENERATOR"] = str(SCRIPTS / "utils" / "generateContainerInventory.py")
        self.check_every_startup(succeeds=True)

    def test_invalid_inventory_cancels_startup(self):
        self.environ["MOCK_COMPOSE_SERVICES"] = "docker_state_exporter"
        self.environ["MOCK_GENERATOR"] = str(SCRIPTS / "utils" / "generateContainerInventory.py")
        self.environ["MOCK_COMPOSE_JSON"] = "invalid JSON"
        self.check_every_startup(succeeds=False)

    def test_invalid_compose_cancels_startup(self):
        self.environ["MOCK_CONFIG_EXIT"] = "1"
        self.check_every_startup(succeeds=False)


if __name__ == "__main__":
    unittest.main()
