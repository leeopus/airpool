#!/usr/bin/env bash
set -euo pipefail

APP="airpool-server"
DIR="$(cd "$(dirname "$0")" && pwd)"
BIN="$DIR/bin/$APP"
PID_FILE="$DIR/$APP.pid"
CONFIG="$DIR/airpool.toml"
LOG_FILE="$DIR/$APP.log"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log()  { echo -e "${GREEN}[AirPool]${NC} $*"; }
warn() { echo -e "${YELLOW}[AirPool]${NC} $*"; }
err()  { echo -e "${RED}[AirPool]${NC} $*"; exit 1; }

is_running() {
    [[ -f "$PID_FILE" ]] && kill -0 "$(cat "$PID_FILE")" 2>/dev/null
}

do_build() {
    log "Building..."
    cd "$DIR" && make build
    log "Build complete: $BIN"
}

do_start() {
    if is_running; then
        err "Already running (PID $(cat "$PID_FILE")). Use 'stop' first or 'restart'."
    fi
    [[ -f "$BIN" ]] || err "Binary not found. Run '$0 init' or '$0 build' first."
    log "Starting $APP..."
    nohup "$BIN" -config "$CONFIG" >> "$LOG_FILE" 2>&1 &
    echo $! > "$PID_FILE"
    sleep 1
    if is_running; then
        log "Started (PID $(cat "$PID_FILE")), log: $LOG_FILE"
    else
        rm -f "$PID_FILE"
        err "Failed to start. Check $LOG_FILE"
    fi
}

do_stop() {
    if ! is_running; then
        warn "Not running."
        return
    fi
    local pid
    pid=$(cat "$PID_FILE")
    log "Stopping (PID $pid)..."
    kill "$pid"
    # Wait up to 5 seconds for graceful shutdown
    for i in {1..10}; do
        kill -0 "$pid" 2>/dev/null || break
        sleep 0.5
    done
    if kill -0 "$pid" 2>/dev/null; then
        warn "Force killing..."
        kill -9 "$pid"
    fi
    rm -f "$PID_FILE"
    log "Stopped."
}

do_init() {
    if is_running; then
        log "Stopping existing instance..."
        do_stop
    fi

    do_build

    # Remove old config, certs, and database so they regenerate on start
    log "Cleaning old config, certs, and database..."
    rm -f "$CONFIG"
    rm -f "$DIR/server.crt" "$DIR/server.key"
    rm -f "$DIR/airpool.db"

    do_start
    log "Init complete. New tokens are printed above."
}

usage() {
    echo "Usage: $0 {start|stop|restart|init|build|status}"
    echo ""
    echo "  start    Start service with current config"
    echo "  stop     Stop service"
    echo "  restart  Stop then start"
    echo "  init     Rebuild, regenerate certs/tokens/db, and start"
    echo "  build    Build binary only"
    echo "  status   Show running status"
}

case "${1:-}" in
    start)
        do_start
        ;;
    stop)
        do_stop
        ;;
    restart)
        do_stop
        do_start
        ;;
    init)
        warn "This will DELETE existing config, certs, database, and regenerate everything."
        read -rp "Continue? [y/N] " confirm
        [[ "$confirm" =~ ^[Yy]$ ]] || exit 0
        do_init
        ;;
    build)
        do_build
        ;;
    status)
        if is_running; then
            log "Running (PID $(cat "$PID_FILE"))"
        else
            warn "Not running."
        fi
        ;;
    *)
        usage
        exit 1
        ;;
esac
