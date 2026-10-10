#!/usr/bin/env bash
# Runs ON the API server. Swaps in an uploaded binary and restarts the service
# with as little downtime as possible, then proves the new build is healthy and
# rolls back to the previous binary if it is not.
#
#   remote_restart.sh <dest-dir> <systemd-service> <api-port>
#
# Expects <dest-dir>/pos-rest.new (already uploaded and checksum-verified).
#
# Why this is fast:
#   * The binary is renamed into place while the old process keeps running
#     (a rename never disturbs a running executable, so nothing is killed).
#   * `systemctl restart` sends SIGTERM; the API stops accepting, finishes the
#     requests it is serving, and exits (lifecycle.Serve). The new process binds
#     its port before connecting to MongoDB/Redis, so requests wait instead of
#     failing while it starts.
#   * With socket activation enabled (enable_socket_activation.sh), systemd holds
#     the port open across the restart and no request is refused at all.
#
# The deploy workflows pipe this file to `ssh ... bash -s`, so the whole body
# sits in main() and is parsed before anything runs; no command can swallow the
# rest of the script from stdin.
#
# Overridable for tests: SYSTEMCTL, HEALTH_URL, HEALTH_TIMEOUT, HEALTH_INTERVAL.
set -euo pipefail

main() {
    if [ "$#" -ne 3 ]; then
        echo "usage: $0 <dest-dir> <service> <port>" >&2
        exit 2
    fi
    DEST="$1"
    SERVICE="$2"
    PORT="$3"
    case "$PORT" in
        ''|*[!0-9]*) echo "invalid port: $PORT" >&2; exit 2 ;;
    esac

    SYSTEMCTL="${SYSTEMCTL:-sudo systemctl}"
    HEALTH_URL="${HEALTH_URL:-http://127.0.0.1:$PORT/v1/health}"
    HEALTH_TIMEOUT="${HEALTH_TIMEOUT:-90}"   # seconds the new build gets to become healthy
    HEALTH_INTERVAL="${HEALTH_INTERVAL:-1}"

    BIN="$DEST/pos-rest"
    NEW="$BIN.new"
    PREV="$BIN.prev"

    log() { echo "[$SERVICE] $*"; }

    healthy() {
        local deadline=$(( $(date +%s) + HEALTH_TIMEOUT ))
        while [ "$(date +%s)" -lt "$deadline" ]; do
            if $SYSTEMCTL is-active --quiet "$SERVICE" &&
               curl -fsS --max-time 10 -o /dev/null "$HEALTH_URL"; then
                return 0
            fi
            sleep "$HEALTH_INTERVAL"
        done
        return 1
    }

    if [ ! -s "$NEW" ]; then
        log "ERROR: $NEW is missing or empty — nothing to deploy."
        exit 1
    fi
    chmod +x "$NEW"

    # Keep the running build as the rollback copy, then rename the new one into
    # place. The live process keeps executing its (now unlinked) old file.
    rm -f "$PREV"
    if [ -e "$BIN" ]; then
        ln "$BIN" "$PREV" 2>/dev/null || cp -p "$BIN" "$PREV"
    fi
    mv -f "$NEW" "$BIN"
    sync

    start=$(date +%s%N)
    log "Restarting (graceful drain of in-flight requests)..."
    $SYSTEMCTL restart "$SERVICE" || true

    if healthy; then
        ms=$(( ($(date +%s%N) - start) / 1000000 ))
        log "Healthy on $HEALTH_URL after ${ms} ms."
        rm -f "$PREV"   # disk is tight on this host; keep only what a rollback needs
        sha256sum "$BIN"
        exit 0
    fi

    log "ERROR: new build did not become healthy within ${HEALTH_TIMEOUT}s."
    $SYSTEMCTL status "$SERVICE" --no-pager -l 2>/dev/null | tail -n 30 || true
    if [ ! -e "$PREV" ]; then
        log "No previous binary to roll back to. Manual intervention required!"
        exit 1
    fi

    log "Rolling back to the previous binary..."
    mv -f "$PREV" "$BIN"
    sync
    $SYSTEMCTL restart "$SERVICE" || true
    if healthy; then
        log "Rolled back; the previous build is serving again. Deploy FAILED."
    else
        log "Rollback did not become healthy either. Manual intervention required!"
    fi
    exit 1
}

main "$@" </dev/null
