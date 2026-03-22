#!/bin/bash

# Configuration
PROJECT_DIR=$(pwd)
SERVER_BIN="$PROJECT_DIR/server"
INGEST_BIN="$PROJECT_DIR/ingest"

function usage() {
    echo "Usage: $0 [command] [args]"
    echo "Commands:"
    echo "  init      Initialize Qdrant collections"
    echo "  server    Run the API server"
    echo "  ingest    Ingest a file or bucket (e.g. ./scripts/run.sh ingest gs://my-bucket/)"
    echo "  build     Build the Go binaries"
}

function build() {
    echo "Building binaries..."
    go build -o server ./cmd/server
    go build -o ingest ./cmd/ingest
    go build -o init-db ./cmd/init-db
}

function init_db() {
    echo "Initializing Qdrant collections..."
    go run ./cmd/init-db/main.go
}

case "$1" in
    "init")
        init_db
        ;;
    "server")
        if [ ! -f "$SERVER_BIN" ]; then build; fi
        echo "Starting server on port 8000..."
        ./server
        ;;
    "ingest")
        if [ ! -f "$INGEST_BIN" ]; then build; fi
        if [ -z "$2" ]; then
            echo "Error: Path or bucket URI required for ingest."
            exit 1
        fi
        ./ingest "$2"
        ;;
    "build")
        build
        ;;
    *)
        usage
        exit 1
        ;;
esac
