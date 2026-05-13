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
  e2e_start_app "kitchen-sink" 18085
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

@test "Cron transport fires scheduled flow" {
  wiremock_wait_for_url "/cron/tick" 75
}
