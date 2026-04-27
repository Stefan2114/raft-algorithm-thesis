#!/usr/bin/env python3

import json
import sys
import os

if len(sys.argv) < 2:
    print("Usage: python3 generate_compose.py <path-to-cluster.json>")
    sys.exit(1)

config_path = sys.argv[1]

try:
    with open(config_path, 'r') as f:
        data = json.load(f)
except Exception as e:
    print(f"Failed to read {config_path}: {e}")
    sys.exit(1)

nodes = data.get('nodes', [])
num_nodes = len(nodes)

if num_nodes < 3:
    print(f"Error: Cluster must have at least 3 nodes, got {num_nodes}")
    sys.exit(1)

if num_nodes % 2 == 0:
    print(f"Error: Cluster must have an odd number of nodes, got {num_nodes}")
    sys.exit(1)

yaml_content = "version: '3.8'\n\nservices:\n"

for node in nodes:
    node_id = node['id']
    yaml_content += f"""  node{node_id}:
    build:
      context: .
      network: host
    network_mode: "host"
    environment:
      - NODE_ID={node_id}
      - CONFIG_PATH=/app/cluster.json
      - DATA_DIR=/data
    volumes:
      - {config_path}:/app/cluster.json:ro
      - node{node_id}_data:/data

"""

yaml_content += "volumes:\n"
for node in nodes:
    node_id = node['id']
    yaml_content += f"  node{node_id}_data:\n"

compose_file = "docker-compose.yml"
with open(compose_file, 'w') as f:
    f.write(yaml_content)

print(f"Successfully generated {compose_file} for {num_nodes} nodes using config {config_path}")
