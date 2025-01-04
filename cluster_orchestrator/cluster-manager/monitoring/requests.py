import grpc
import os
from proto import monitoringNotification_pb2 as pb
from proto import monitoringNotification_pb2_grpc as pb_grpc

def monitoring_manager_notify_deployment(job, instance_number):

    print(os.getenv('MONITORING_MANAGER_ADDR', '172.20.0.1:5568'))

    # Create gRPC channel
    channel = grpc.insecure_channel(os.getenv('MONITORING_MANAGER_ADDR', '172.20.0.1:5568'))
    stub = pb_grpc.MonitoringServiceStub(channel)

    # Create request message
    network = pb.NetworkInfo(
        bandwidth_in=str(job['bandwidth_in']),
        bandwidth_out=str(job['bandwidth_out'])
    )

    resource = pb.ResourceInfo(
        cpu=str(job['vcpus']),
        memory=str(job['memory']),
        gpu=str(job['vgpus']),
        network=network,
        disk=str(job['storage'])
    )

    request = pb.MonitoringRequest(
        job_name=job['job_name'],
        job_hash=job['job_hash'],
        instance_number=int(instance_number),
        resource=resource
    )

    try:
        response = stub.NotifyDeployment(request)
        return response.acknowledged
    except grpc.RpcError as e:
        print(f"RPC failed: {e}")
        return False
    