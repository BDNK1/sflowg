#!/usr/bin/env bats

load '../lib/harness'
load '../lib/wiremock'
load '../lib/kafka'
load '../lib/assertions'

setup_file() {
  e2e_require_env
  e2e_start_compose
  kafka_topic_create "orders.events"
  e2e_build_cli
  e2e_build_fixture "kitchen-sink"
  e2e_start_app "kitchen-sink" 18083
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

@test "retry path recovers against WireMock scenario" {
  hurl \
    --variable host="http://localhost:18083" \
    --test \
    "$ROOT/tests/hurl/retry_success.hurl"

  assert_equals "2" "$(wiremock_count_url "/unstable")"
}

@test "fallback path runs after retries are exhausted" {
  hurl \
    --variable host="http://localhost:18083" \
    --test \
    "$ROOT/tests/hurl/fallback.hurl"

  wiremock_verify_url "/fallback"
}
