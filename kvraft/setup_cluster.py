#!/usr/bin/env python3

import json
import sys
import os
import argparse

def main():
    parser = argparse.ArgumentParser(description='Generate Docker Compose and Monitoring configurations for the Raft cluster.')
    parser.add_argument('config', help='Path to cluster.json')
    
    # Raft Timeouts
    parser.add_argument('--election-min', type=int, default=150, help='Minimum election timeout in ms (default: 150)')
    parser.add_argument('--election-rand', type=int, default=150, help='Random jitter for election timeout in ms (default: 150)')
    parser.add_argument('--heartbeat', type=int, default=50, help='Heartbeat interval in ms (default: 50)')
    
    # RSM Timeouts
    parser.add_argument('--submit-timeout', type=int, default=10, help='RSM submit timeout in seconds (default: 10)')
    
    # Persistence
    parser.add_argument('--max-raft-state', type=int, default=1000000, help='Snapshot when persist size exceeds this in bytes (default: 1000000)')
    
    # gRPC/Network Settings
    parser.add_argument('--grpc-base-delay', type=int, default=100, help='gRPC base backoff delay in ms (default: 100)')
    parser.add_argument('--grpc-max-delay', type=int, default=3, help='gRPC max backoff delay in seconds (default: 3)')
    parser.add_argument('--grpc-conn-timeout', type=int, default=2, help='gRPC min connect timeout in seconds (default: 2)')
    
    # Clerk/Client Settings
    parser.add_argument('--clerk-rpc-timeout', type=int, default=5, help='Clerk RPC timeout in seconds (default: 5)')
    parser.add_argument('--clerk-retry-sleep', type=int, default=20, help='Clerk retry sleep in ms (default: 20)')
    
    # Metrics
    parser.add_argument('--metrics-port-base', type=int, default=8080, help='Base port for Prometheus metrics (default: 8080)')

    args = parser.parse_args()

    try:
        with open(args.config, 'r') as f:
            data = json.load(f)
    except Exception as e:
        print(f"Failed to read {args.config}: {e}")
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
      - RAFT_ELECTION_TIMEOUT_MIN={args.election_min}
      - RAFT_ELECTION_TIMEOUT_RAND={args.election_rand}
      - RAFT_HEARTBEAT_TIMEOUT={args.heartbeat}
      - RSM_SUBMIT_TIMEOUT={args.submit_timeout}
      - GRPC_BASE_DELAY={args.grpc_base_delay}
      - GRPC_MAX_DELAY={args.grpc_max_delay}
      - GRPC_MIN_CONNECT_TIMEOUT={args.grpc_conn_timeout}
      - METRICS_PORT_BASE={args.metrics_port_base}
      - CLERK_RPC_TIMEOUT={args.clerk_rpc_timeout}
      - CLERK_RETRY_SLEEP={args.clerk_retry_sleep}
      - MAX_RAFT_STATE={args.max_raft_state}
    volumes:
      - {args.config}:/app/cluster.json:ro
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
    volumes:
      - grafana_data:/var/lib/grafana
      - ./config/grafana/provisioning:/etc/grafana/provisioning:ro
      - ./config/grafana/dashboards:/var/lib/grafana/dashboards:ro
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
    yaml_content += "  grafana_data:\n"

    compose_file = "docker-compose.yml"
    with open(compose_file, 'w') as f:
        f.write(yaml_content)

    print(f"Successfully generated {compose_file} for {num_nodes} nodes using config {args.config}")

    # Generate Monitoring Configs
    os.makedirs("config", exist_ok=True)

    prometheus_targets = []
    for node in nodes:
        node_id = node['id']
        prometheus_targets.append(f"'localhost:{args.metrics_port_base + node_id}'")

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

    # Generate Grafana Provisioning
    os.makedirs("config/grafana/provisioning/datasources", exist_ok=True)
    os.makedirs("config/grafana/provisioning/dashboards", exist_ok=True)
    os.makedirs("config/grafana/dashboards", exist_ok=True)

    grafana_ds_yml = """apiVersion: 1
datasources:
  - name: Prometheus
    type: prometheus
    access: proxy
    url: http://localhost:9090
    isDefault: true
  - name: Loki
    type: loki
    access: proxy
    url: http://localhost:3100
"""
    with open("config/grafana/provisioning/datasources/ds.yaml", "w") as f:
        f.write(grafana_ds_yml)

    grafana_dashboards_yml = """apiVersion: 1
providers:
  - name: 'Local Dashboards'
    orgId: 1
    folder: ''
    type: file
    disableDeletion: false
    editable: true
    options:
      path: /etc/grafana/provisioning/dashboards
"""
    # TODO: We point to /etc/grafana/provisioning/dashboards inside the container
    # and we will mount our local config/grafana/dashboards to it if we want persistent JSONs,
    # but for now, we'll just allow provisioning from the folder we mounted.
    
    # Actually, let's fix the path in dashboards.yaml to point to where we will put JSONs.
    grafana_dashboards_yml = """apiVersion: 1
providers:
  - name: 'Default'
    orgId: 1
    folder: ''
    type: file
    disableDeletion: false
    editable: true
    options:
      path: /var/lib/grafana/dashboards
"""
    with open("config/grafana/provisioning/dashboards/dashboards.yaml", "w") as f:
        f.write(grafana_dashboards_yml)

    print("Successfully generated Grafana provisioning configs in config/grafana/")

if __name__ == "__main__":
    main()
