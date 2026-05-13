#!/usr/bin/env bash

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN="$ROOT/bin"
DEFAULT_SFLOWG_REPO="$(cd "$ROOT/.." && pwd)"
SFLOWG_REPO="${SFLOWG_REPO:-$DEFAULT_SFLOWG_REPO}"

e2e_root() {
  printf "%s\n" "$ROOT"
}

e2e_require_command() {
  local command_name="$1"

  if ! command -v "$command_name" >/dev/null 2>&1; then
    echo "$command_name is required" >&2
    return 1
  fi
}

e2e_require_env() {
  SFLOWG_REPO="$(cd "$SFLOWG_REPO" && pwd)"
  export SFLOWG_REPO

  if [[ ! -d "$SFLOWG_REPO/cli" || ! -d "$SFLOWG_REPO/core" ]]; then
    echo "SFLOWG_REPO does not look like sflowg repo: $SFLOWG_REPO" >&2
    return 1
  fi

  e2e_require_command go
  e2e_require_command curl
  e2e_require_command jq
}

e2e_compose() {
  docker compose -f "$ROOT/docker-compose.yml" "$@"
}

e2e_start_compose() {
  e2e_require_command docker
  e2e_compose up -d "$@"
  e2e_wait_postgres
  e2e_create_kafka_topic "orders.events"
  e2e_create_kafka_topic "orders.invalid"
}

e2e_stop_compose() {
  e2e_compose down -v --remove-orphans
}

e2e_wait_postgres() {
  for _ in $(seq 1 30); do
    if e2e_compose exec -T postgres pg_isready -U sflowg -d sflowg_e2e >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done

  echo "Timed out waiting for Postgres" >&2
  return 1
}

e2e_create_kafka_topic() {
  local topic="$1"

  for _ in $(seq 1 30); do
    if e2e_compose exec -T redpanda rpk topic create "$topic" >/dev/null 2>&1; then
      return 0
    fi
    if e2e_compose exec -T redpanda rpk topic describe "$topic" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done

  echo "Timed out creating Kafka topic $topic" >&2
  return 1
}

e2e_build_cli() {
  mkdir -p "$BIN"
  local workfile
  workfile="$(mktemp "${TMPDIR:-/tmp}/sflowg-e2e-work.XXXXXX")"

  cat >"$workfile" <<EOF
go 1.25.0

use (
	$SFLOWG_REPO/cli
	$SFLOWG_REPO/core
	$SFLOWG_REPO/docs/examples/stripe-integration/plugins/stripe-checkout
	$SFLOWG_REPO/docs/examples/stripe-integration/plugins/stripe-signature-verification
	$SFLOWG_REPO/plugins/http
	$SFLOWG_REPO/plugins/postgres
	$SFLOWG_REPO/transports/cron
	$SFLOWG_REPO/transports/http
	$SFLOWG_REPO/transports/kafka
)
EOF

  trap 'rm -f "$workfile"' RETURN

  (
    cd "$SFLOWG_REPO/cli"
    GOWORK="$workfile" go build -o "$BIN/sflowg" .
  )
}

e2e_build_fixture() {
  local fixture_name="$1"
  local fixture="$ROOT/fixtures/$fixture_name"
  local build_log="$ROOT/.build-$fixture_name.log"

  (
    cd "$fixture"
    "$BIN/sflowg" build . \
      --core-path "$SFLOWG_REPO/core" \
      --transports-path "$SFLOWG_REPO/transports" \
      --core-plugins-path "$SFLOWG_REPO/plugins"
  ) >"$build_log" 2>&1
}

e2e_build_fixture_embed() {
  local fixture_name="$1"
  local fixture="$ROOT/fixtures/$fixture_name"
  local build_log="$ROOT/.build-$fixture_name.log"

  (
    cd "$fixture"
    "$BIN/sflowg" build . \
      --embed-flows \
      --core-path "$SFLOWG_REPO/core" \
      --transports-path "$SFLOWG_REPO/transports" \
      --core-plugins-path "$SFLOWG_REPO/plugins"
  ) >"$build_log" 2>&1
}

e2e_wait_http() {
  local url="$1"
  local timeout="${2:-60}"

  for _ in $(seq 1 "$timeout"); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done

  echo "Timed out waiting for $url" >&2
  return 1
}

e2e_start_app() {
  local fixture_name="$1"
  local port="$2"
  local fixture="$ROOT/fixtures/$fixture_name"
  local app="$fixture/$fixture_name"

  if [[ ! -x "$app" ]]; then
    echo "fixture app is missing or not executable: $app" >&2
    return 1
  fi

  WIREMOCK_URL="http://localhost:8089" \
  POSTGRES_DSN="postgres://sflowg:sflowg@localhost:54329/sflowg_e2e?sslmode=disable" \
  KAFKA_BROKERS="localhost:29092" \
  FIXTURE_PREFIX="${FIXTURE_PREFIX:-e2e}" \
  "$app" \
    --port "$port" \
    --flows "$fixture/flows" \
    >"$ROOT/.app-$fixture_name.log" 2>&1 &

  APP_PID="$!"
  export APP_PID
  printf "%s\n" "$APP_PID" >"$ROOT/.app-$fixture_name.pid"

  e2e_wait_http "http://localhost:$port/health" 60
}

e2e_start_app_auto_flows() {
  local fixture_name="$1"
  local port="$2"
  local fixture="$ROOT/fixtures/$fixture_name"
  local app="$fixture/$fixture_name"

  if [[ ! -x "$app" ]]; then
    echo "fixture app is missing or not executable: $app" >&2
    return 1
  fi

  WIREMOCK_URL="http://localhost:8089" \
  POSTGRES_DSN="postgres://sflowg:sflowg@localhost:54329/sflowg_e2e?sslmode=disable" \
  KAFKA_BROKERS="localhost:29092" \
  FIXTURE_PREFIX="${FIXTURE_PREFIX:-e2e}" \
  "$app" \
    --port "$port" \
    >"$ROOT/.app-$fixture_name.log" 2>&1 &

  APP_PID="$!"
  export APP_PID
  printf "%s\n" "$APP_PID" >"$ROOT/.app-$fixture_name.pid"

  e2e_wait_http "http://localhost:$port/health" 60
}

e2e_start_app_with_flows_env() {
  local app="$1"
  local flows_path="$2"
  local fixture_name="$3"
  local port="$4"

  if [[ ! -x "$app" ]]; then
    echo "fixture app is missing or not executable: $app" >&2
    return 1
  fi

  WIREMOCK_URL="http://localhost:8089" \
  POSTGRES_DSN="postgres://sflowg:sflowg@localhost:54329/sflowg_e2e?sslmode=disable" \
  KAFKA_BROKERS="localhost:29092" \
  FIXTURE_PREFIX="${FIXTURE_PREFIX:-e2e}" \
  FLOWS_PATH="$flows_path" \
  "$app" \
    --port "$port" \
    >"$ROOT/.app-$fixture_name.log" 2>&1 &

  APP_PID="$!"
  export APP_PID
  printf "%s\n" "$APP_PID" >"$ROOT/.app-$fixture_name.pid"

  e2e_wait_http "http://localhost:$port/health" 60
}

e2e_cleanup() {
  local pid="${APP_PID:-}"
  local pid_file

  for pid_file in "$ROOT"/.app-*.pid; do
    if [[ -z "$pid" && -f "$pid_file" ]]; then
      pid="$(cat "$pid_file")"
    fi
    if [[ -n "$pid" ]]; then
      kill "$pid" 2>/dev/null || true
      wait "$pid" 2>/dev/null || true
    fi
    rm -f "$pid_file"
    pid=""
  done

  if [[ -n "${APP_PID:-}" ]]; then
    kill "$APP_PID" 2>/dev/null || true
    wait "$APP_PID" 2>/dev/null || true
  fi
}

e2e_dump_logs() {
  local fixture_name="${1:-kitchen-sink}"

  echo "--- build log ---" >&2
  cat "$ROOT/.build-$fixture_name.log" >&2 2>/dev/null || true

  echo "--- app log ---" >&2
  cat "$ROOT/.app-$fixture_name.log" >&2 2>/dev/null || true

  echo "--- docker compose ps ---" >&2
  e2e_compose ps >&2 2>/dev/null || true

  echo "--- wiremock unmatched requests ---" >&2
  curl -fsS "http://localhost:8089/__admin/requests/unmatched" 2>/dev/null | jq . >&2 || true

  echo "--- wiremock requests ---" >&2
  curl -fsS "http://localhost:8089/__admin/requests" 2>/dev/null | jq . >&2 || true
}

e2e_teardown_on_failure() {
  local fixture_name="${1:-kitchen-sink}"

  if [[ "${BATS_TEST_COMPLETED:-0}" -ne 1 ]]; then
    e2e_dump_logs "$fixture_name"
  fi
}
