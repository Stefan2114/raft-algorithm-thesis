# KVRaft Go Backend

This directory contains the Go core implementation of the distributed, fault-tolerant key-value store. For a high-level overview of the architecture and cross-language communication, see the root [README.md](../README.md).

---

## Connection to MIT 6.824
This implementation is inspired by the **MIT 6.824 (Distributed Systems)** labs, specifically Labs 3 and 4. It extends the academic model by implementing robust production-grade technologies, including:
-   **gRPC** for peer-to-peer and client-to-server communication.
-   **Zap Logger** for highly detailed structured logging.
-   **Prometheus/Grafana** for cluster telemetries.

---

## Getting Started

### 1. Protobuf Generation
To regenerate Go gRPC stubs from the shared `proto/` definitions:
```bash
protoc --go_out=. --go_opt=module=kvraft --go-grpc_out=. --go-grpc_opt=module=kvraft ../proto/raft.proto ../proto/kv.proto
```

### 2. Native CLI Tools
Build and use the local CLI to interact with the nodes manually:
```bash
go build -o kvcli ./cmd/cli

# Read/Write commands
./kvcli --config ../cluster.json put key1 value1
./kvcli --config ../cluster.json get key1
```

### 3. Docker Cluster Orchestration
Cluster topologies and timeouts are defined via `cluster.json` and generated using `setup_cluster.py`:
```bash
# Generate config (timeout values and ports)
./setup_cluster.py ../cluster.json

# Start the cluster
docker-compose up -d --build
```

---

## Configuration & Environment Variables

The cluster behavior can be customized during the setup phase using command-line arguments in `setup_cluster.py`. These values are injected into cluster nodes via environment variables:

| Argument | Description | Default | Environment Variable |
|----------|-------------|---------|----------------------|
| `config` | **(Required)** Path to `cluster.json` topology. | N/A | `CONFIG_PATH` |
| `--election-min` | Minimum election timeout (ms). | `800` | `RAFT_ELECTION_TIMEOUT_MIN` |
| `--election-rand` | Random jitter for election timeout (ms). | `600` | `RAFT_ELECTION_TIMEOUT_RAND` |
| `--heartbeat` | Heartbeat interval (ms). | `100` | `RAFT_HEARTBEAT_TIMEOUT` |
| `--submit-timeout` | RSM command commit timeout (seconds). | `10` | `RSM_SUBMIT_TIMEOUT` |
| `--metrics-port-base`| Base port for Prometheus metrics. | `8080` | `METRICS_PORT_BASE` |
| `--clerk-rpc-timeout`| Clerk RPC call timeout (seconds). | `5` | `CLERK_RPC_TIMEOUT` |
| `--clerk-retry-sleep`| Clerk cluster-wide retry delay (ms). | `20` | `CLERK_RETRY_SLEEP` |
| `--max-raft-state` | Snapshot when persist size exceeds this (bytes).| `100000` | `MAX_RAFT_STATE` |
| `--raft-debug`      | Enable Raft debug logging (`true`/`false`).      | `true`  | `RAFT_DEBUG`          |

---

## Performance Tuning & Benchmarks

Performance of the Raft cluster relies heavily on balancing election timeouts and snapshotting frequency:
*   **Snapshot Threshold (`--max-raft-state`)**:
    *   *Production (`1000000`+ bytes)*: Minimizes CPU/Disk IO overhead due to snapshot serialization.
    *   *Testing (`1000` bytes)*: Recommended during debug cycles to force frequent snapshotting and state recovery paths.
*   **Raft Timeouts**:
    *   If you observe frequent leader elections during idle times, increase `--election-min` and `--election-rand`.

### Throughput Benchmarks
Throughput benchmarked using `ghz` on a 16-Core Arch Linux machine with a stable leader:

**KVRaft (This Project):**
-   **Requests/sec:** 216.72
-   **Average Latency:** 45.95 ms
-   **99th Percentile Latency:** 121.63 ms

**etcd Cluster (For comparison):**
-   **Requests/sec:** 903.73
-   **Average Latency:** 10.93 ms
-   **99th Percentile Latency:** 41.59 ms

---

## Monitoring Setup

For detailed instructions on configuring Grafana dashboards, PromQL queries, and Loki, refer to the root [README.md](../README.md) or the detailed [grafana_setup_guide.md](../grafana_setup_guide.md).