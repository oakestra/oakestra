import yaml

"""
Node list template:

root:
    hostname: string
    ip: string
    clusters:
        - cluster1_hostname:
            ip: string
            nodes:
                - node1_hostname:
                    ip: string
                - node2_hostname:
                    ip: string
        - cluster2_hostname:
            ip: string
            nodes:
                - node3_hostname:
                    ip: string
            
"""
node_list = {}
with open(r'../wcfg.yaml') as file:
    node_list = yaml.load(file, Loader=yaml.FullLoader)
    print(node_list)


nodes=""+str(node_list["root"]["hostname"])+","+str(node_list["root"]["ip"])+"\n"
root=str(node_list["root"]["hostname"])+"\n"
clusters=""
cluster_nodes=""

cluster_list=node_list["root"]["clusters"]
for cluster_hostname in cluster_list:
    nodestr = cluster_hostname+","+str(cluster_list[cluster_hostname]["ip"])
    nodes=nodes+nodestr+"\n"
    clusters= clusters+cluster_hostname+"\n"
    cluster_nodes=""

    node_list=cluster_list[cluster_hostname]["nodes"]
    for node_hostname in node_list:
        nodestr = node_hostname+","+node_list[node_hostname]["ip"]
        nodes=nodes+nodestr+"\n"

        cluster_nodes=cluster_nodes+node_hostname+"\n"

    with open(cluster_hostname+'.txt', 'w+') as file:
        file.write(cluster_nodes)

with open('cluster.txt', 'w+') as file:
    file.write(clusters)

with open('root.txt', 'w+') as file:
    file.write(root)

with open('node.txt', 'w+') as file:
    file.write(nodes)


