import copy
import sys
from pathlib import Path
from unittest import mock

sys.path.append(str(Path(__file__).parent.parent))

# RUNNER_MAP instantiates DockerRunner at import time, which needs a docker daemon.
with mock.patch("docker.from_env"):
    from addons_runner.docker_runner import DockerRunner
    from services.monitor_service import DEFAULT_NETWORK, AddonsMonitor  # noqa: E402


class FakeRunner:
    def __init__(self):
        self.calls = []

    def get_container(self, name):
        return None

    def run_service(self, service, project_name, aliases=None):
        self.calls.append((copy.deepcopy(service), aliases))
        return service["service_name"]


def make_addon(ui=True, ui_networks=None):
    addon = {
        "_id": "abc",
        "name": "example",
        "services": [
            {"service_name": "example-ui", "image": "img", "networks": ui_networks or []},
            {"service_name": "example-api", "image": "img", "networks": ["private"]},
        ],
    }
    if ui:
        addon["ui"] = {"service": "example-ui", "sdk": "1"}
    return addon


def run(addon):
    runner = FakeRunner()
    monitor = AddonsMonitor()
    for service in addon["services"]:
        service["labels"] = {}
    monitor.run_addon_services(
        "abc", addon["services"], runner, ui_service=monitor._get_ui_service(addon)
    )
    return {call[0]["service_name"]: call for call in runner.calls}


def test_ui_service_gets_default_network_and_alias():
    calls = run(make_addon(ui_networks=["private"]))

    service, aliases = calls["example-ui"]
    assert service["networks"] == ["private", DEFAULT_NETWORK]
    assert aliases == {DEFAULT_NETWORK: ["addonui-example"]}


def test_ui_service_without_networks_defaults_to_oakestra_once():
    service, aliases = run(make_addon())["example-ui"]

    assert service["networks"] == [DEFAULT_NETWORK]
    assert aliases == {DEFAULT_NETWORK: ["addonui-example"]}


def test_ui_service_already_on_default_network_is_not_duplicated():
    service, _ = run(make_addon(ui_networks=[DEFAULT_NETWORK, "private"]))["example-ui"]

    assert service["networks"] == [DEFAULT_NETWORK, "private"]


def test_non_ui_service_is_unchanged():
    service, aliases = run(make_addon())["example-api"]

    assert service["networks"] == ["private"]
    assert aliases is None


def test_addon_without_ui_has_no_alias():
    calls = run(make_addon(ui=False, ui_networks=["private"]))

    for service, aliases in calls.values():
        assert aliases is None
        assert DEFAULT_NETWORK not in service["networks"]


def test_cluster_monitor_adds_no_alias(monkeypatch):
    monkeypatch.setattr("services.monitor_service.ORCHESTRATION_PLANE", "cluster")

    service, aliases = run(make_addon(ui_networks=["private"]))["example-ui"]

    assert aliases is None
    assert service["networks"] == ["private"]


def test_invalid_name_skips_alias_but_still_runs_service():
    addon = make_addon()
    addon["name"] = "My Addon"

    calls = run(addon)

    assert set(calls) == {"example-ui", "example-api"}
    assert all(aliases is None for _, aliases in calls.values())


def make_runner():
    with mock.patch("docker.from_env") as from_env:
        return DockerRunner(), from_env.return_value


def make_network(name):
    network = mock.Mock()
    network.name = name
    return network


def test_get_network_by_name_requires_exact_match():
    runner, client = make_runner()
    similar, exact = make_network("oakestra-dashboard-dev_default"), make_network("oakestra")
    # The Docker API filters by substring, so it can return similar names first.
    client.networks.list.return_value = [similar, exact]

    assert runner.get_network_by_name("oakestra") is exact
    client.networks.list.return_value = [similar]
    assert runner.get_network_by_name("oakestra") is None


def test_docker_runner_alias_on_first_network():
    runner, client = make_runner()
    service = {"service_name": "example-ui", "image": "img", "networks": [DEFAULT_NETWORK]}

    runner.run_service(service, "proj", aliases={DEFAULT_NETWORK: ["addonui-example"]})

    client.api.create_endpoint_config.assert_called_once_with(aliases=["addonui-example"])
    kwargs = client.containers.run.call_args.kwargs
    assert kwargs["network"] == DEFAULT_NETWORK
    assert kwargs["networking_config"] == {
        DEFAULT_NETWORK: client.api.create_endpoint_config.return_value
    }


def test_docker_runner_alias_on_later_network():
    runner, client = make_runner()
    network = make_network("oakestra")
    client.networks.list.return_value = [network]
    service = {"service_name": "example-ui", "image": "img", "networks": ["private", "oakestra"]}

    container = runner.run_service(service, "proj", aliases={"oakestra": ["addonui-example"]})

    assert client.containers.run.call_args.kwargs["networking_config"] is None
    network.connect.assert_called_once_with(
        client.containers.run.return_value, aliases=["addonui-example"]
    )
    assert container is client.containers.run.return_value


def test_docker_runner_without_aliases_passes_none():
    runner, client = make_runner()
    network = make_network("y")
    client.networks.list.return_value = [network]

    runner.run_service({"service_name": "a", "image": "i", "networks": ["x", "y"]}, "proj")

    assert client.containers.run.call_args.kwargs["networking_config"] is None
    network.connect.assert_called_once_with(client.containers.run.return_value, aliases=None)
