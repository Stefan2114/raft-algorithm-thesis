#!/bin/bash
set -e

PIDS_FILE=".cluster.pids"

if [[ ! -f "$PIDS_FILE" ]]; then
    echo "ERROR: Cluster does not seem to be running ($PIDS_FILE is missing)."
    exit 1
fi

echo "--- Shutting Down Cluster ---"
while read -r PID; do
    if ps -p $PID > /dev/null; then
        echo "Sending SIGTERM to Node (PID: $PID) for graceful database shutdown..."
        kill -TERM "$PID" 2>/dev/null || true
    else
        echo "Node (PID: $PID) is already dead."
    fi
done < "$PIDS_FILE"

# Give the nodes a few seconds to flush their LevelDB/Filesystem snapshots to disk
echo "Waiting for data flush..."
sleep 2

# Cleanup PIDs file
rm "$PIDS_FILE"
echo "--- Cluster Shutdown Completed ---"
