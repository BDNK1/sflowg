#!/usr/bin/env bats

load '../lib/harness'
load '../lib/assertions'

setup_file() {
  e2e_require_env
  e2e_build_cli
}

teardown() {
  e2e_teardown_on_failure "kitchen-sink"
}

@test "sflowg CLI binary is built" {
  assert_file_executable "$BIN/sflowg"
}

@test "sflowg build creates kitchen-sink app" {
  e2e_build_fixture "kitchen-sink"
  assert_file_executable "$ROOT/fixtures/kitchen-sink/kitchen-sink"
}
