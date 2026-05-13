#!/usr/bin/env bats

load '../lib/harness'

setup_file() {
  e2e_require_env
  e2e_start_compose
  e2e_build_cli
  e2e_build_fixture "kitchen-sink"
  e2e_start_app "kitchen-sink" 18091
}

teardown() {
  e2e_teardown_on_failure "kitchen-sink"
}

teardown_file() {
  e2e_cleanup
  e2e_stop_compose
}

@test "async steps and sequential foreach variants work" {
  hurl \
    --variable host="http://localhost:18091" \
    --test \
    "$ROOT/tests/hurl/async_foreach.hurl"
}
