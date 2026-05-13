#!/usr/bin/env bats

load '../lib/harness'
load '../lib/wiremock'

setup_file() {
  e2e_require_env
  e2e_start_compose
  e2e_build_cli
  e2e_build_fixture "kitchen-sink"
  e2e_start_app "kitchen-sink" 18090
}

setup() {
  wiremock_reset
}

teardown() {
  e2e_teardown_on_failure "kitchen-sink"
}

teardown_file() {
  e2e_cleanup
  e2e_stop_compose
}

@test "HTTP text response includes status body and header" {
  hurl \
    --variable host="http://localhost:18090" \
    --test \
    "$ROOT/tests/hurl/response_text.hurl"
}

@test "HTTP redirect response includes location" {
  hurl \
    --variable host="http://localhost:18090" \
    --test \
    "$ROOT/tests/hurl/response_redirect.hurl"
}

@test "flow on_error returns response and compensation side effect runs" {
  hurl \
    --variable host="http://localhost:18090" \
    --test \
    "$ROOT/tests/hurl/error_compensation.hurl"

  wiremock_verify_url "/compensation/res-e2e"
  wiremock_verify_body_contains "/compensation/res-e2e" "reserve"
}
