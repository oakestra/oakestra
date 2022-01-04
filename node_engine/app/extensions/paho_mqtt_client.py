# import os
#
# import numpy as np
# from paho.mqtt.client import Client
# import json
# import re
#
# from flask import current_app
# from app.models.vivaldi_coordinate import VivaldiCoordinate
# from app.blueprints.network_measurement import network_measurement
# from app.blueprints.node_engine import cpu_mem, dockerclient, coordinates, mirageosclient
# from app.blueprints.monitoring import monitoring
# from app.extensions.logging import configure_logging
# # mqtt = None
# mqtt = Client()
# logger = configure_logging("paho_mqtt_client")
# # mqtt.on_connect = on_connect
# # mqtt.on_message = on_message
# # mqtt.on_disconnect = on_disconnect
# ip_info = {}
# node_info = {}
# mqtt_logger = None
# vivaldi_coordinate = None
#
#
# def mqtt_init(info, vivaldi_coord, node_ip_info):
#     global mqtt
#     global node_info
#     global vivaldi_coordinate
#     global ip_info
#     vivaldi_coords = vivaldi_coord
#     ip_info = node_ip_info
#     node_info = info
#     mqtt.on_message = on_message
#     mqtt.on_disconnect = on_disconnect
#     # Subscribing in on_connect() means that if we lose the connection and
#     # reconnect then subscriptions will be renewed.
#     logger.info(f"NODE id: {node_info.id}")
#     mqtt.subscribe(f"nodes/{node_info.id}/control/+")
#     # Subscribe to the ping channel. If a worker receives a message via that channel it should ping the IP contained in the message
#     mqtt.subscribe(f"nodes/{node_info.id}/ping")
#     mqtt.subscribe(f"nodes/{node_info.id}/vivaldi")
#     logger.info("mqtt initialized")
#
#
# def on_connect(client, userdata, flags, rc):
#     print("Connected with result code "+str(rc))
# # The callback for when a PUBLISH message is received from the server.
# def on_message(client, userdata, msg):
#     logger.info(f"msg.topic {msg.payload} node id {node_info.id}")
#     data = dict(
#         topic=msg.topic,
#         payload=json.loads(msg.payload.decode())
#     )
#     logger.info(data)
#
#     topic = data.get('topic')
#     # if topic starts with nodes and ends with controls
#     re_nodes_topic_control_deploy = re.search("^nodes/" + node_info.id + "/control/deploy$", topic)
#     re_nodes_topic_control_delete = re.search("^nodes/" + node_info.id + "/control/delete$", topic)
#     re_nodes_topic_vivaldi = re.search("^nodes/" + node_info.id + "/vivaldi", topic)
#
#     payload = data.get('payload')
#     # If the node received a message via the ping channel it should ping the IP contained in the received message
#     if re_nodes_topic_vivaldi is not None:
#         handle_nodes_topic_vivaldi(payload)
#     if re_nodes_topic_control_deploy is not None:
#         handle_nodes_topic_control_deploy(payload)
#     elif re_nodes_topic_control_delete is not None:
#         handle_nodes_topic_control_delete(payload)
#
# def on_disconnect():
#     mqtt.disconnect()
#     logger.info("PAHO MQTT - Disconnected!")
#
#
# def publish_cpu_mem():
#     global node_info
#     global vivaldi_coordinate
#     logger.info(f"Publishing CPU+Memory usage... my ID: {node_info.id}")
#     cpu_used, free_cores, memory_used, free_memory_in_MB = cpu_mem.get_cpu_memory()
#     mem_value = cpu_mem.get_memory()
#     topic = f"nodes/{node_info.id}/information"
#     lat, long = coordinates.get_coordinates()
#     # If ip address of this node is private, the node has to ping the network router such that this RTT can be added
#     # to nodes pinging this network. Otherwise the Vivaldi network coordinates would update themself with respect to
#     # the router and not the nodes within the network
#     is_netem_configured = os.environ.get('IS_NETEM_CONFIGURED') == 'TRUE'
#     mqtt.publish(topic, json.dumps({'cpu': cpu_used, 'free_cores': free_cores,
#                                     'memory': memory_used, 'memory_free_in_MB': free_memory_in_MB,
#                                     'lat': lat, 'long': long,
#                                     'public_ip': ip_info["public"], 'private_ip': ip_info["private"],
#                                     'router_rtt': ip_info["router_rtt"], 'netem_delay': network_measurement.get_netem_delay(is_netem_configured)}))
#
#
# def publish_sla_alarm(alarm_type, violated_job, ip_rtt_stats=None):
#     # logger.info(f"Publishing SLA violation alarm... my ID: {my_id}")
#     logger.info(f"Publishing SLA violation alarm... my ID: {node_info.id}\n")
#     # topic = f"nodes/{my_id}/alarm"
#     topic = f"nodes/{node_info.id}/alarm"
#     # ip_rtt_stats = {<violating ip>: <violating rtt>,...} only required for latency constraint violations
#     mqtt.publish(topic, json.dumps({"job": violated_job, "alarm_type": alarm_type, "ip_rtt_stats": ip_rtt_stats}))
#
#
# def publish_deploy_status(my_id, job_id, status, ns_ip):
#     logger.info(f"Publishing Deployment status... my ID: {node_info.id}")
#     topic = f"nodes/{node_info.id}/job"
#     mqtt.publish(topic, json.dumps({'job_id': job_id, 'status': status, 'ns_ip': ns_ip}))
#
#
# def handle_nodes_topic_control_deploy(payload):
#     # global app
#     # mqtt_port = app.config['MQTT_BROKER_PORT']
#     logger.info("MQTT - Received .../control/deploy command")
#     address = None
#     job = payload.get('job')
#     job_id = job['_id']
#     job_name = job['job_name']
#     virtualization = job['virtualization']
#     image_url = job['code']
#     port = job['port']
#     if virtualization == 'docker':
#         address, container_id = dockerclient.start_container(job=job)
#         logger.info(f"Register to monitoring component: Job: {job}, Container ID: {container_id}")
#         monitoring_port = os.environ.get("MONITORING_PORT")
#         # Register deployed service to Monitoring component
#         monitoring.register_service(node_info.id, container_id, job)
#     if virtualization == 'mirage':
#         commands = payload.get('commands')
#         mirageosclient.run_unikernel_mirageos(image_url, job_name, job_name, commands)
#     if address is not None:
#         publish_deploy_status(node_info.id, job.get('_id'), 'DEPLOYED', address)
#     else:
#         publish_deploy_status(node_info.id, job.get('_id'), 'FAILED', '')
#
#
# def handle_nodes_topic_control_delete(payload):
#     job = payload.get('job')
#     virtualization = job['virtualization']
#     job_name = job['job_nameb']
#     logger.info('MQTT - Received .../control/delete command')
#     if virtualization == 'docker':
#         dockerclient.stop_container(job_name)
#
#
# def handle_nodes_topic_vivaldi(payload):
#     global vivaldi_coordinate
#     public_ip = ip_info["public"]
#     logger.info("MQTT - Received Vivaldi command")
#     vivaldi_info = payload.get('vivaldi_info')
#     logger.info(f"Received vivaldi infos: {vivaldi_info}")
#     # Dict has target IP as key and a tuple consisting of the remote VivaldiCoordinate and the router_rtt if required
#     ip_vivaldi_dict = {}
#     for info in vivaldi_info:
#         # vivaldi_info = (public_ip, private_ip, router_rtt, vector, height, error)
#         remote_public_ip = info[0]
#         remote_private_ip = info[1]
#         remote_router_rtt = float(info[2])
#         remote_vector = info[3]
#         remote_height = info[4]
#         remote_error = info[5]
#         remote_vivaldi = VivaldiCoordinate(len(remote_vector))
#         remote_vivaldi.vector = np.array(remote_vector)
#         remote_vivaldi.height = float(remote_height)
#         remote_vivaldi.error = float(remote_error)
#         # this node is in same network as remote and both behind router -> same public ip and private ip not null -> ping private ip
#         if public_ip == remote_public_ip:
#             ip_vivaldi_dict.setdefault(remote_private_ip, []).append((remote_vivaldi, None))
#         # This node and the remote node are not within the same network
#         if public_ip != remote_public_ip:
#             ip_vivaldi_dict.setdefault(remote_public_ip, []).append((remote_vivaldi, remote_router_rtt))
#
#     # Ping received IPs in parallel
#     statistics = network_measurement.parallel_ping(ip_vivaldi_dict.keys())
#     logger.info(f"Ping statistics: {statistics}")
#     for ip, rtt in statistics.items():
#         viv_router_rtts = ip_vivaldi_dict[ip] # TODO: Naming!
#         for _viv, _router_rtt in viv_router_rtts:
#             if _router_rtt is not None:
#                 total_rtt = rtt + _router_rtt
#                 vivaldi_coordinate.update(total_rtt, _viv)
#             else:
#                 vivaldi_coordinate.update(rtt, _viv)