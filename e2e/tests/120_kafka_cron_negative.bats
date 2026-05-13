#!/usr/bin/env bats

load '../lib/harness'
load '../lib/wiremock'
load '../lib/kafka'

setup_file() {
  e2e_require_env
  e2e_start_compose
  e2e_build_cli
  e2e_build_fixture "kitchen-sink"
  e2e_start_app "kitchen-sink" 18093
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

@test "Kafka schema violation runs on_error and acks message" {
  kafka_produce_json "orders.invalid" '{"amount_cents":1000}'

  wiremock_wait_for_url "/kafka/invalid/error" 30
  wiremock_verify_body_contains "/kafka/invalid/error" "SCHEMA_VIOLATION"
  wiremock_verify_url_count "/kafka/invalid/should-not-run" "0"
}

@test "Cron on_error path runs after scheduled failure" {
  wiremock_wait_for_url "/cron/error-handled" 75
  wiremock_verify_body_contains "/cron/error-handled" "CRON_FORCED_ERROR"
}
