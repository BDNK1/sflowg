#!/usr/bin/env bash
set -euo pipefail

EXAMPLE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ROOT_DIR="$(cd "$EXAMPLE_DIR/../../.." && pwd)"
RUN_ID="$(date +%s)-$$"
SAFE_RUN_ID="${RUN_ID//[^a-zA-Z0-9]/_}"
WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/sflowg-kafka-e2e.XXXXXX")"
APP_DIR="$WORK_DIR/kafka-consumer"
BIN_DIR="$WORK_DIR/bin"
LOG_FILE="$WORK_DIR/kafka-consumer-example.log"
TOPIC="orders.events"

cleanup() {
  if [[ -n "${APP_PID:-}" ]] && kill -0 "$APP_PID" 2>/dev/null; then
    kill "$APP_PID"
    wait "$APP_PID" 2>/dev/null || true
  fi
  rm -rf "$WORK_DIR"
}
trap cleanup EXIT

mkdir -p "$BIN_DIR"

echo "building CLI"
(
  cd "$ROOT_DIR/cli"
  go build -o "$BIN_DIR/sflowg" .
)

echo "starting Redpanda"
docker compose -f "$EXAMPLE_DIR/docker-compose.yml" up -d redpanda

for _ in {1..60}; do
  if docker compose -f "$EXAMPLE_DIR/docker-compose.yml" exec -T redpanda rpk cluster info >/dev/null 2>&1; then
    break
  fi
  sleep 0.5
done

docker compose -f "$EXAMPLE_DIR/docker-compose.yml" exec -T redpanda rpk topic create "$TOPIC" >/dev/null 2>&1 || true

cp -R "$EXAMPLE_DIR" "$APP_DIR"
"$BIN_DIR/sflowg" build "$APP_DIR" \
  --core-path "$ROOT_DIR/core" \
  --transports-path "$ROOT_DIR/transports" \
  --core-plugins-path "$ROOT_DIR/plugins"

echo "starting kafka consumer example"
(
  cd "$APP_DIR"
  ./kafka-consumer-example >"$LOG_FILE" 2>&1
) &
APP_PID="$!"

for _ in {1..60}; do
  if grep -q "Kafka consumer started" "$LOG_FILE"; then
    break
  fi
  if ! kill -0 "$APP_PID" 2>/dev/null; then
    echo "kafka consumer exited during startup"
    cat "$LOG_FILE"
    exit 1
  fi
  sleep 0.5
done

ORDER_ID="ord_e2e_${SAFE_RUN_ID}"
printf '%s\n' "{\"order_id\":\"$ORDER_ID\",\"customer_email\":\"kafka-e2e-$SAFE_RUN_ID@example.com\",\"amount_cents\":14999}" |
  docker compose -f "$EXAMPLE_DIR/docker-compose.yml" exec -T redpanda rpk topic produce "$TOPIC" >/dev/null

for _ in {1..60}; do
  if grep -q "$ORDER_ID" "$LOG_FILE" && grep -q "kafka order received" "$LOG_FILE"; then
    echo "kafka consumer e2e passed"
    exit 0
  fi
  if ! kill -0 "$APP_PID" 2>/dev/null; then
    echo "kafka consumer exited before processing message"
    cat "$LOG_FILE"
    exit 1
  fi
  sleep 0.5
done

echo "timed out waiting for Kafka message to be processed"
cat "$LOG_FILE"
exit 1
