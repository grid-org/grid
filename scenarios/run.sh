#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
API="http://localhost:8765"
COMPOSE="docker compose -f $SCRIPT_DIR/compose.yaml"
GRIDC="$PROJECT_DIR/bin/gridc"

usage() {
    echo "Usage: $0 <job-file> [--keep]"
    echo ""
    echo "Runs a scenario job against a local compose cluster."
    echo ""
    echo "  job-file   Path to a YAML job file (e.g. scenarios/jobs/smoke-test.yaml)"
    echo "  --keep     Leave the cluster running after the job completes"
    echo ""
    echo "Examples:"
    echo "  $0 scenarios/jobs/smoke-test.yaml"
    echo "  $0 scenarios/jobs/multi-step.yaml --keep"
    exit 1
}

cleanup() {
    if [ "$KEEP" = "false" ]; then
        echo "--- Tearing down cluster ---"
        $COMPOSE down --timeout 5 2>/dev/null
    else
        echo "--- Cluster still running (use 'task scenario:down' to stop) ---"
    fi
}

# Parse args
JOB_FILE=""
KEEP="false"
for arg in "$@"; do
    case "$arg" in
        --keep) KEEP="true" ;;
        -*) usage ;;
        *) JOB_FILE="$arg" ;;
    esac
done

if [ -z "$JOB_FILE" ]; then
    usage
fi

if [ ! -f "$JOB_FILE" ]; then
    echo "Error: job file not found: $JOB_FILE"
    exit 1
fi

# Build gridc
echo "--- Building gridc ---"
go build -o "$GRIDC" "$PROJECT_DIR/cmd/client/main.go"

trap cleanup EXIT

# Start cluster
echo "--- Starting cluster ---"
$COMPOSE up -d --build --wait 2>&1

# Wait for nodes to register
echo "--- Waiting for nodes ---"
for i in $(seq 1 15); do
    NODE_OUTPUT=$($GRIDC -a "$API" node list 2>&1 || true)
    NODE_COUNT=$(echo "$NODE_OUTPUT" | grep -c "online" || true)
    if [ "$NODE_COUNT" -ge 3 ]; then
        echo "    $NODE_COUNT nodes registered"
        break
    fi
    if [ "$i" -eq 15 ]; then
        echo "Error: timed out waiting for nodes (got $NODE_COUNT)"
        $COMPOSE logs
        exit 1
    fi
    sleep 1
done

# Show nodes
echo "--- Nodes ---"
$GRIDC -a "$API" node list

# Submit job and wait
echo ""
echo "--- Running: $(basename "$JOB_FILE") ---"
$GRIDC -a "$API" job run -f "$JOB_FILE" --wait
