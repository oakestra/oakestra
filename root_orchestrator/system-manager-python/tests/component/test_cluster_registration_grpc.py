"""Cluster registration over the gRPC handshake (ClusterRegistrationServicer), driven only
through the real grpc_stub fixture.
"""

import grpc
import pytest
from proto.clusterRegistration_pb2 import CS1Message

from tests.component.conftest import cs2_message

pytestmark = pytest.mark.component


def test_handle_init_greeting_returns_hello_message(grpc_stub):
    response = grpc_stub.handle_init_greeting(CS1Message(hello_service_manager="hi"))

    assert response.hello_cluster_manager == "please send your cluster info"


def test_handle_init_final_reachable_cluster_registers_it(
    grpc_stub, make_cluster, resource_abstractor, outbound
):
    cluster = make_cluster(registered=False)

    response = grpc_stub.handle_init_final(
        cs2_message(cluster, network_component_port=55555, cluster_location="48.1,11.5,1000")
    )

    candidate = resource_abstractor.candidates[response.id]
    assert candidate["ip"] == cluster.ip
    assert candidate["port"] == str(cluster.port)
    assert candidate["candidate_name"] == cluster.name
    assert candidate["candidate_location"] == "48.1,11.5,1000"
    # clusterinfo is accepted and stored, but ResourceSchema does not expose it back out.
    assert candidate["clusterinfo"] == {"key": "k1", "value": "v1"}

    reachability_probes = outbound.cluster(cluster, method="GET", path="/api/cluster/status")
    assert len(reachability_probes) == 1

    net_plugin_calls = outbound.net_plugin(method="POST", path="/api/net/cluster")
    assert len(net_plugin_calls) == 1
    assert net_plugin_calls[0].json == {
        "cluster_id": response.id,
        "cluster_address": cluster.ip,
        "cluster_port": 55555,
    }


def test_handle_init_final_same_cluster_name_upserts(grpc_stub, make_cluster, resource_abstractor):
    cluster = make_cluster(registered=False, name="stable-cluster-name")

    first = grpc_stub.handle_init_final(cs2_message(cluster))
    second = grpc_stub.handle_init_final(cs2_message(cluster))

    assert second.id == first.id
    assert len(resource_abstractor.candidates) == 1


def test_handle_init_final_empty_cluster_address_rejected(
    grpc_stub, make_cluster, resource_abstractor
):
    cluster = make_cluster(registered=False)
    message = cs2_message(cluster)
    message.cluster_address = ""

    with pytest.raises(grpc.RpcError) as excinfo:
        grpc_stub.handle_init_final(message)

    assert excinfo.value.code() == grpc.StatusCode.INVALID_ARGUMENT
    assert resource_abstractor.candidates == {}


@pytest.mark.parametrize(
    "make_unreachable",
    [
        lambda cluster: cluster.go_down(),
        lambda cluster: cluster.respond(500),
    ],
    ids=["connection_refused", "http_500"],
)
def test_handle_init_final_unreachable_cluster_rejected(
    grpc_stub, make_cluster, resource_abstractor, outbound, make_unreachable
):
    # fast_reachability_probe (autouse) sets the retry window to 0, so this fails on
    # the first attempt instead of the usual 30 s.
    cluster = make_cluster(registered=False)
    make_unreachable(cluster)

    with pytest.raises(grpc.RpcError) as excinfo:
        grpc_stub.handle_init_final(cs2_message(cluster))

    assert excinfo.value.code() == grpc.StatusCode.FAILED_PRECONDITION
    assert resource_abstractor.candidates == {}
    assert outbound.net_plugin(method="POST", path="/api/net/cluster") == []


def test_handle_init_final_resource_abstractor_down(grpc_stub, make_cluster, resource_abstractor):
    cluster = make_cluster(registered=False)
    resource_abstractor.go_down()

    with pytest.raises(grpc.RpcError) as excinfo:
        grpc_stub.handle_init_final(cs2_message(cluster))

    assert excinfo.value.code() == grpc.StatusCode.INTERNAL


@pytest.mark.xfail(
    strict=True,
    reason=(
        "bug: system_manager.py handle_init_final reads message['cluster_info'][0] "
        "without a check. MessageToDict drops empty repeated fields, so a cluster with "
        "no cluster_info hits a KeyError and the client gets StatusCode.UNKNOWN. "
        "Rejecting with INVALID_ARGUMENT or registering it anyway would both be fine."
    ),
)
def test_handle_init_final_empty_cluster_info_rejected_cleanly(
    grpc_stub, make_cluster, resource_abstractor
):
    cluster = make_cluster(registered=False)

    try:
        response = grpc_stub.handle_init_final(cs2_message(cluster, cluster_info=[]))
    except grpc.RpcError as excinfo:
        assert excinfo.code() == grpc.StatusCode.INVALID_ARGUMENT
    else:
        assert response.id in resource_abstractor.candidates


def test_handle_init_final_net_plugin_failure_still_registers(
    grpc_stub, make_cluster, resource_abstractor, net_plugin_fails
):
    # net_register_cluster (ext_requests/net_plugin_requests.py) swallows every
    # RequestException, so a broken net plugin must not block registration.
    cluster = make_cluster(registered=False)

    response = grpc_stub.handle_init_final(cs2_message(cluster))

    assert response.id in resource_abstractor.candidates
