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
      - LOG_PATH=/logs/node{node_id}.log
    volumes:
      - {config_path}:/app/cluster.json:ro
      - node{node_id}_data:/data
      - logs_data:/logs

"""

yaml_content += """  prometheus:
    image: prom/prometheus:latest
    network_mode: "host"
    volumes:
      - ./config/prometheus.yml:/etc/prometheus/prometheus.yml:ro

  grafana:
    image: grafana/grafana:latest
    network_mode: "host"
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
    depends_on:
      - prometheus
      - loki

  loki:
    image: grafana/loki:latest
    network_mode: "host"
    command: -config.file=/etc/loki/local-config.yaml

  promtail:
    image: grafana/promtail:latest
    network_mode: "host"
    volumes:
      - logs_data:/logs:ro
      - ./config/promtail-config.yml:/etc/promtail/promtail-config.yml:ro
    command: -config.file=/etc/promtail/promtail-config.yml

"""

yaml_content += "volumes:\n  logs_data:\n"
for node in nodes:
    node_id = node['id']
    yaml_content += f"  node{node_id}_data:\n"

compose_file = "docker-compose.yml"
with open(compose_file, 'w') as f:
    f.write(yaml_content)

print(f"Successfully generated {compose_file} for {num_nodes} nodes using config {config_path}")

# Generate Monitoring Configs
os.makedirs("config", exist_ok=True)

prometheus_targets = []
for node in nodes:
    node_id = node['id']
    prometheus_targets.append(f"'localhost:{8080 + node_id}'")

prometheus_yml = f"""global:
  scrape_interval: 5s

scrape_configs:
  - job_name: 'kvraft'
    static_configs:
      - targets: [{', '.join(prometheus_targets)}]
"""

with open("config/prometheus.yml", "w") as f:
    f.write(prometheus_yml)

promtail_yml = """server:
  http_listen_port: 9080
  grpc_listen_port: 0

positions:
  filename: /tmp/positions.yaml

clients:
  - url: http://localhost:3100/loki/api/v1/push

scrape_configs:
- job_name: system
  static_configs:
  - targets:
      - localhost
    labels:
      job: kvraft_logs
      __path__: /logs/*log
"""

with open("config/promtail-config.yml", "w") as f:
    f.write(promtail_yml)

print("Successfully generated config/prometheus.yml and config/promtail-config.yml")
