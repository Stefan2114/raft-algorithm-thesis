#!/usr/bin/env python3
import time
import random
import subprocess
import signal
import sys
import logging
import argparse
import json
import os

# Configure logging
logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s')

# Globals to be populated from config
NODES = []
MAX_FAILURES = 0 

def run_cmd(cmd):
    try:
        subprocess.run(cmd, shell=True, check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    except subprocess.CalledProcessError:
        pass # Ignore errors during cleanup or if container is already in target state

def cleanup(sig, frame):
    print("\n")
    logging.info("Stopping chaos monkey... ensuring all nodes are running and unpaused.")
    for node in NODES:
        run_cmd(f"docker-compose unpause {node}")
        run_cmd(f"docker-compose start {node}")
    logging.info("Cleanup complete. Cluster restored. Exiting.")
    sys.exit(0)

def load_config(config_path):
    global NODES, MAX_FAILURES
    try:
        with open(config_path, 'r') as f:
            data = json.load(f)
            
        nodes_data = data.get("nodes", [])
        num_nodes = len(nodes_data)
        
        if num_nodes == 0:
            logging.error(f"No nodes found in {config_path}")
            sys.exit(1)
            
        # Compose container names usually end up with node0, node1, etc.
        # based on our docker-compose.yml service names
        NODES = [f"node{n['id']}" for n in nodes_data]
        
        # Calculate max failures to keep a majority
        # Majority = (N // 2) + 1
        # Max failures = N - Majority
        majority = (num_nodes // 2) + 1
        MAX_FAILURES = num_nodes - majority
        
        logging.info(f"Loaded {num_nodes} nodes from config: {NODES}")
        logging.info(f"Cluster majority is {majority}. Max allowed concurrent failures: {MAX_FAILURES}")
        
    except Exception as e:
        logging.error(f"Failed to load config {config_path}: {e}")
        sys.exit(1)

def chaos_loop(min_downtime, max_downtime, min_interval, max_interval):
    logging.info("Starting Chaos Monkey for Raft Cluster...")
    logging.info("This script will randomly stop or pause nodes to simulate failures.")
    logging.info("Press Ctrl+C to stop and restore all nodes.")
    
    while True:
        # Wait a bit before the next chaos event
        interval = random.randint(min_interval, max_interval)
        logging.info(f"Waiting for {interval} seconds before next event...")
        time.sleep(interval)
        
        # If MAX_FAILURES is 0 (e.g. 1 node or 2 node cluster), we can't safely inject failures and keep majority
        if MAX_FAILURES < 1:
            logging.warning("Cluster too small to inject failures while maintaining a majority. Skipping.")
            continue
            
        num_nodes_to_affect = random.randint(1, MAX_FAILURES)
        target_nodes = random.sample(NODES, num_nodes_to_affect)
        
        # We can either stop (crash) or pause (network partition/CPU starvation)
        action = random.choice(["stop", "pause"])
        
        if action == "stop":
            logging.warning(f"ACTION: Stopping nodes {target_nodes} (simulating crash)")
            for node in target_nodes:
                run_cmd(f"docker-compose stop {node}")
                
            downtime = random.randint(min_downtime, max_downtime)
            logging.info(f"Sleeping for {downtime} seconds while nodes are stopped...")
            time.sleep(downtime)
            
            logging.info(f"RECOVERY: Starting nodes {target_nodes}")
            for node in target_nodes:
                run_cmd(f"docker-compose start {node}")
                
        elif action == "pause":
            logging.warning(f"ACTION: Pausing nodes {target_nodes} (simulating network partition/delay)")
            for node in target_nodes:
                run_cmd(f"docker-compose pause {node}")
                
            downtime = random.randint(min_downtime, max_downtime)
            logging.info(f"Sleeping for {downtime} seconds while nodes are paused...")
            time.sleep(downtime)
            
            logging.info(f"RECOVERY: Unpausing nodes {target_nodes}")
            for node in target_nodes:
                run_cmd(f"docker-compose unpause {node}")

if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Chaos Monkey for Raft Cluster")
    parser.add_argument("--config", type=str, default="../cluster.json", help="Path to cluster.json (default: ../cluster.json)")
    parser.add_argument("--min-downtime", type=int, default=10, help="Minimum seconds a node stays down/paused (default: 10)")
    parser.add_argument("--max-downtime", type=int, default=20, help="Maximum seconds a node stays down/paused (default: 20)")
    parser.add_argument("--min-interval", type=int, default=15, help="Minimum seconds between chaos events (default: 15)")
    parser.add_argument("--max-interval", type=int, default=30, help="Maximum seconds between chaos events (default: 30)")
    args = parser.parse_args()
    
    load_config(args.config)

    # Register signals after NODES are loaded so cleanup works correctly
    signal.signal(signal.SIGINT, cleanup)
    signal.signal(signal.SIGTERM, cleanup)

    # Ensure all nodes are clean before starting
    logging.info("Ensuring all nodes are running before starting chaos...")
    for node in NODES:
        run_cmd(f"docker-compose unpause {node}")
        run_cmd(f"docker-compose start {node}")
    
    chaos_loop(args.min_downtime, args.max_downtime, args.min_interval, args.max_interval)
