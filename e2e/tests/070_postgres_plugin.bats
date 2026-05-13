#!/usr/bin/env bats

load '../lib/harness'
load '../lib/postgres'

setup_file() {
  e2e_require_env
  e2e_start_compose
  postgres_reset_orders
  e2e_build_cli
  e2e_build_fixture "kitchen-sink"
  e2e_start_app "kitchen-sink" 18086
}

teardown() {
  e2e_teardown_on_failure "kitchen-sink"
}

teardown_file() {
  e2e_cleanup
  e2e_stop_compose
}

@test "postgres plugin inserts, updates, and reads rows" {
  hurl \
    --variable host="http://localhost:18086" \
    --test \
    "$ROOT/tests/hurl/postgres_order.hurl"
}
