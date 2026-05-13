#!/usr/bin/env bats

load '../lib/harness'
load '../lib/wiremock'
load '../lib/kafka'

setup_file() {
  e2e_require_env
  e2e_start_compose
  kafka_topic_create "orders.events"
  e2e_build_cli
  e2e_build_fixture "kitchen-sink"
  e2e_start_app "kitchen-sink" 18080
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

@test "HTTP kitchen sink syntax works" {
  hurl \
    --variable host="http://localhost:18080" \
    --test \
    "$ROOT/tests/hurl/http_syntax.hurl"
}

@test "WireMock received catalog calls" {
  hurl \
    --variable host="http://localhost:18080" \
    --test \
    "$ROOT/tests/hurl/http_syntax.hurl"

  wiremock_verify_url "/catalog/sku_1"
  wiremock_verify_url "/catalog/sku_2"
}
