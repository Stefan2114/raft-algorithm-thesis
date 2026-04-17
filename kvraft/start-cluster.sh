#!/bin/bash
set -e

# Default values
NODES=3
PORTS_FLAG=""
START_PORT=8000
CONFIG_OUT="../cluster.json"
PRODUCTION=false
DEBUG=false
LOG_DIR=""
CLEAN=false

# Parse arguments
while [[ "$#" -gt 0 ]]; do
    case $1 in
        --nodes|-n) NODES="$2"; shift ;;
        --ports|-p) PORTS_FLAG="$2"; shift ;;
        --start-port|-sp) START_PORT="$2"; shift ;;
        --config-output|-c) CONFIG_OUT="$2"; shift ;;
        --production|-prod) PRODUCTION=true ;;
        --debug|-d) DEBUG=true ;;
        --log-dir|-l) LOG_DIR="$2"; shift ;;
        --clean) CLEAN=true ;;
        *) echo "Unknown parameter passed: $1"; exit 1 ;;
    esac
    shift
done

# Validation: Count must be odd and >= 3
if [[ "$NODES" -lt 3 ]]; then
    echo "ERROR: Raft requires at least 3 nodes to form a functional cluster."
    exit 1
fi
if [[ $((NODES % 2)) -eq 0 ]]; then
    echo "ERROR: Raft cluster size must be an odd number (e.g., 3, 5, 7) to enforce strict majorities."
    exit 1
fi

# Validation: Limit nodes unless in production
if [ "$PRODUCTION" = false ] && [ "$NODES" -gt 11 ]; then
    echo "ERROR: To protect your local machine, non-production environments are limited to 11 nodes. Use --production to bypass."
    exit 1
fi

# Parse ports list
declare -a PORT_LIST
if [[ -n "$PORTS_FLAG" ]]; then
    IFS=',' read -r -a PORT_LIST <<< "$PORTS_FLAG"
else
    # Auto-generate port list
    for (( i=0; i<NODES; i++ )); do
        PORT_LIST+=($((START_PORT + i)))
    done
fi

# Validation: Check ports count
if [[ "${#PORT_LIST[@]}" -ne "$NODES" ]]; then
    echo "ERROR: Validated $NODES nodes, but received ${#PORT_LIST[@]} ports from --ports list. The number of explicit ports must match the node count exactly."
    exit 1
fi

echo "--- Generating Cluster Configuration ---"
# Create path if it has directories
mkdir -p "$(dirname "$CONFIG_OUT")"

echo "{" > "$CONFIG_OUT"
echo "  \"nodes\": [" >> "$CONFIG_OUT"
for (( i=0; i<NODES; i++ )); do
    PORT=${PORT_LIST[$i]}
    COMMA=","
    if [[ $i -eq $((NODES-1)) ]]; then COMMA=""; fi
    echo "    {\"id\": $i, \"addr\": \"localhost:$PORT\"}$COMMA" >> "$CONFIG_OUT"
done
echo "  ]" >> "$CONFIG_OUT"
echo "}" >> "$CONFIG_OUT"
echo "Configuration dynamically saved to: $CONFIG_OUT"

echo "--- Building Server Binary ---"
go build -o kvserver ./cmd/kvserver

# Cleanup data if requested
if [[ "$CLEAN" = true ]]; then
    echo "Cleaning existing node data directories..."
    rm -rf ./data/node*
fi

PIDS_FILE=".cluster.pids"
> "$PIDS_FILE"

if [[ -n "$LOG_DIR" ]]; then
    echo "Creating log directory: $LOG_DIR"
    mkdir -p "$LOG_DIR"
fi

echo "--- Starting $NODES Nodes ---"
for (( i=0; i<NODES; i++ )); do
    DATA_DIR="./data/node$i"
    mkdir -p "$DATA_DIR"
    
    SERVER_CMD="./kvserver --config $CONFIG_OUT --id $i --datadir $DATA_DIR"
    
    if [[ "$PRODUCTION" = true ]]; then
        SERVER_CMD="$SERVER_CMD --production"
    fi
    if [[ "$DEBUG" = true ]]; then
        SERVER_CMD="$SERVER_CMD --debug"
    fi
    if [[ -n "$LOG_DIR" ]]; then
        SERVER_CMD="$SERVER_CMD --logpath \"$LOG_DIR/node$i.log\""
    fi
    
    # Start in background securely bypassing tty
    eval "$SERVER_CMD > /dev/null 2>&1 &"
    PID=$!
    echo "$PID" >> "$PIDS_FILE"
    echo "Started Node $i (PID $PID) on port ${PORT_LIST[$i]} -> Data: $DATA_DIR"
done

echo "============================================="
echo "Cluster successfully started in the background!"
echo "PIDs tracked safely in $PIDS_FILE."
echo "To shut down gracefully, run: ./stop-cluster.sh"
echo "============================================="
