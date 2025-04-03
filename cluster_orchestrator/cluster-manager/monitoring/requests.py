import grpc
import os

from monitoring_proto_lib.monitoring.v1 import monitoring_pb2_grpc as pb_grpc
from monitoring_proto_lib.monitoring.v1 import deploy_pb2 as deploy_pb2
from monitoring_proto_lib.monitoring.v1 import delete_pb2 as delete_pb2

def monitoring_manager_notify_deployment(job: dict, instance_number: int, node: dict):

    print(os.getenv('MONITORING_MANAGER_ADDR', '172.20.0.1:5568'))

    # Create gRPC channel
    channel = grpc.insecure_channel(os.getenv('MONITORING_MANAGER_ADDR', '172.20.0.1:5568'))
    stub = pb_grpc.MonitoringServiceStub(channel)

    # Create request message
    network = deploy_pb2.NetworkInfo(
        bandwidth_in=str(job['bandwidth_in']),
        bandwidth_out=str(job['bandwidth_out'])
    )

    # set resource limits
    cpu_limit = job['vcpus'] if job['vcpus'] > 0 else node['node_info']['cpu_max_frequency']
    memory_limit = job['memory'] if job['memory'] > 0 else node['node_info']['memory_total_in_MB']

    
    resource = deploy_pb2.ResourceInfo(
        cpu=str(cpu_limit),
        memory=str(memory_limit),
        gpu=str(job['vgpus']),
        network=network,
        disk=str(job['storage'])
    )

    # set calculation requests
    calculation_requests = []
    for metric in job.get('monitoring', []):
        calculation_requests.append(deploy_pb2.CalculationRequest(
            metric_name=metric['output_metric_name'],
            formula=metric['formula'],
            description=metric['description'],
            states=metric.get('states', []),
            goal=metric['goal'],
            unit=metric['unit']
        ))

    request = deploy_pb2.NotifyDeploymentRequest(
        job_name=job['job_name'],
        job_hash=job['job_hash'],
        instance_number=int(instance_number),
        resource=resource,
        calculation_requests=calculation_requests
    )

    try:
        response = stub.NotifyDeployment(request)
        return response.acknowledged
    except grpc.RpcError as e:
        print(f"RPC failed: {e}")
        return False
    
    
def monitoring_manager_notify_deletion(job, instance_number):
    # Create gRPC channel
    channel = grpc.insecure_channel(os.getenv('MONITORING_MANAGER_ADDR', '172.20.0.1:5568'))
    stub = pb_grpc.MonitoringServiceStub(channel)
    
    # Create request message
    request = delete_pb2.NotifyDeletionRequest(job_name=job['job_name'], instance_number=int(instance_number))
    try:
        response = stub.NotifyDeletion(request)
        return response.acknowledged
    except grpc.RpcError as e:
        print(f"RPC failed: {e}")
        return False