#!/usr/bin/env bash
set -euo pipefail

EXAMPLE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ROOT_DIR="$(cd "$EXAMPLE_DIR/../../.." && pwd)"
TEST_DIR="$EXAMPLE_DIR/e2e-test"
BASE_URL="${BASE_URL:-http://localhost:8080}"
RUN_ID="$(date +%s)-$$"
SAFE_RUN_ID="${RUN_ID//[^a-zA-Z0-9]/_}"
WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/sflowg-ecom-e2e.XXXXXX")"
APP_DIR="$WORK_DIR/ecom"
BIN_DIR="$WORK_DIR/bin"
LOG_FILE="$WORK_DIR/ecommerce-api.log"

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

echo "starting ecommerce Postgres"
docker compose -f "$EXAMPLE_DIR/docker-compose.yml" up -d postgres

cp -R "$EXAMPLE_DIR" "$APP_DIR"
"$BIN_DIR/sflowg" build "$APP_DIR" \
  --core-path "$ROOT_DIR/core" \
  --transports-path "$ROOT_DIR/transports" \
  --core-plugins-path "$ROOT_DIR/plugins"

echo "starting ecommerce example"
(
  cd "$APP_DIR"
  DATABASE_URL="${DATABASE_URL:-postgres://postgres:postgres@localhost:5432/ecommerce?sslmode=disable}" \
  STRIPE_INTEGRATION_URL="${STRIPE_INTEGRATION_URL:-http://localhost:8090}" \
  ./ecommerce-api >"$LOG_FILE" 2>&1
) &
APP_PID="$!"

hurl --test --retry 60 --retry-interval 250 \
  --variable "base_url=$BASE_URL" \
  "$TEST_DIR/ready.hurl"

hurl --test \
  --variable "base_url=$BASE_URL" \
  --variable "run_id=$SAFE_RUN_ID" \
  "$TEST_DIR/regression.hurl"

echo "ecommerce e2e passed"
