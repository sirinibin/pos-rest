#!/usr/bin/env bash
# One-time server setup (run ON the API server, with sudo rights). Puts a
# systemd socket unit in front of an API service so systemd, not the API
# process, owns the listening ports. During every later restart the ports stay
# open and requests queue for the new process instead of being refused.
#
#   enable_socket_activation.sh <systemd-service> <api-port>
#
#   e.g. enable_socket_activation.sh start-api-test 2002
#        enable_socket_activation.sh start-api      2000
#
# The service unit itself is not edited: systemd pairs <service>.socket with
# <service>.service by name. Setup restarts the API once (about a second).
# Undo with:  sudo systemctl disable --now <service>.socket &&
#             sudo rm /etc/systemd/system/<service>.socket &&
#             sudo systemctl daemon-reload && sudo systemctl restart <service>
#
# Overridable for tests: SUDO, SYSTEMCTL, UNIT_DIR, HEALTH_URL, HEALTH_TIMEOUT.
set -euo pipefail

if [ "$#" -ne 2 ]; then
    echo "usage: $0 <service> <port>" >&2
    exit 2
fi
SERVICE="$1"
PORT="$2"
case "$PORT" in
    ''|*[!0-9]*) echo "invalid port: $PORT" >&2; exit 2 ;;
esac
HTTPS_PORT=$((PORT + 1))   # main.go serves HTTPS on API_PORT+1

SUDO="${SUDO-sudo}"
SYSTEMCTL="${SYSTEMCTL:-$SUDO systemctl}"
UNIT_DIR="${UNIT_DIR:-/etc/systemd/system}"
HEALTH_URL="${HEALTH_URL:-http://127.0.0.1:$PORT/v1/health}"
HEALTH_TIMEOUT="${HEALTH_TIMEOUT:-90}"
SOCKET_UNIT="$UNIT_DIR/$SERVICE.socket"

if ! $SYSTEMCTL cat "$SERVICE.service" >/dev/null 2>&1; then
    echo "ERROR: $SERVICE.service does not exist." >&2
    exit 1
fi

# A binary built before socket-activation support would try to bind the ports
# itself, find them held by systemd, and crash. Refuse until it is deployed.
exec_path=$($SYSTEMCTL show -p ExecStart --value "$SERVICE.service" | sed -n 's/.*path=\([^ ;]*\).*/\1/p' | head -n 1)
if [ -z "$exec_path" ] || ! grep -aq "systemd-activated socket" "$exec_path"; then
    echo "ERROR: ${exec_path:-the service binary} does not support socket activation yet." >&2
    echo "Deploy a build that includes the lifecycle package first." >&2
    exit 1
fi

unit="[Unit]
Description=Listening sockets for $SERVICE (kept open across restarts)

[Socket]
ListenStream=$PORT
ListenStream=$HTTPS_PORT
NoDelay=true
Backlog=4096

[Install]
WantedBy=sockets.target
"

if [ -f "$SOCKET_UNIT" ] && [ "$(cat "$SOCKET_UNIT")" = "$(printf '%s' "$unit")" ] &&
   $SYSTEMCTL is-active --quiet "$SERVICE.socket"; then
    echo "[$SERVICE] socket activation already enabled on :$PORT and :$HTTPS_PORT."
    exit 0
fi

printf '%s' "$unit" | $SUDO tee "$SOCKET_UNIT" >/dev/null
$SYSTEMCTL daemon-reload

# The service must release the ports before systemd can take them.
echo "[$SERVICE] Handing ports $PORT/$HTTPS_PORT to systemd (one short restart)..."
$SYSTEMCTL stop "$SERVICE"
if ! $SYSTEMCTL enable --now "$SERVICE.socket"; then
    echo "[$SERVICE] ERROR: could not start $SERVICE.socket; restoring the old setup." >&2
    $SUDO rm -f "$SOCKET_UNIT"
    $SYSTEMCTL daemon-reload
    $SYSTEMCTL start "$SERVICE"
    exit 1
fi
$SYSTEMCTL start "$SERVICE"

deadline=$(( $(date +%s) + HEALTH_TIMEOUT ))
while [ "$(date +%s)" -lt "$deadline" ]; do
    if curl -fsS --max-time 10 -o /dev/null "$HEALTH_URL"; then
        echo "[$SERVICE] Healthy with socket activation."
        exit 0
    fi
    sleep 1
done

echo "[$SERVICE] ERROR: not healthy with socket activation; restoring the old setup." >&2
$SYSTEMCTL stop "$SERVICE" || true
$SYSTEMCTL disable --now "$SERVICE.socket" || true
$SUDO rm -f "$SOCKET_UNIT"
$SYSTEMCTL daemon-reload
$SYSTEMCTL start "$SERVICE"
exit 1
