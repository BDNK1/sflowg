#!/usr/bin/env bats

load '../lib/harness'

setup_file() {
  e2e_require_env
  e2e_start_compose
  e2e_build_cli
}

teardown() {
  e2e_teardown_on_failure "kitchen-sink"
  e2e_cleanup
}

teardown_file() {
  e2e_stop_compose
}

@test "embedded flow binary starts without external flows directory" {
  e2e_build_fixture_embed "kitchen-sink"

  local runtime_dir="$BATS_TEST_TMPDIR/embedded"
  mkdir -p "$runtime_dir"
  cp "$ROOT/fixtures/kitchen-sink/kitchen-sink" "$runtime_dir/kitchen-sink"

  e2e_start_app_with_flows_env "$runtime_dir/kitchen-sink" "" "kitchen-sink" 18087

  curl -fsS "http://localhost:18087/health" >/dev/null
}

@test "runtime flow resolution uses FLOWS_PATH without --flows" {
  e2e_build_fixture "kitchen-sink"

  local runtime_dir="$BATS_TEST_TMPDIR/flows-env"
  mkdir -p "$runtime_dir"
  cp "$ROOT/fixtures/kitchen-sink/kitchen-sink" "$runtime_dir/kitchen-sink"

  e2e_start_app_with_flows_env "$runtime_dir/kitchen-sink" "$ROOT/fixtures/kitchen-sink/flows" "kitchen-sink" 18088

  curl -fsS "http://localhost:18088/health" >/dev/null
}

@test "runtime flow resolution uses adjacent flows directory and --port" {
  e2e_build_fixture "kitchen-sink"

  e2e_start_app_auto_flows "kitchen-sink" 18089

  curl -fsS "http://localhost:18089/health" >/dev/null
}
