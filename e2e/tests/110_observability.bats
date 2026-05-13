#!/usr/bin/env bats

load '../lib/harness'

setup_file() {
  e2e_require_env
  e2e_start_compose
  e2e_build_cli
  e2e_build_fixture "kitchen-sink"
  e2e_start_app "kitchen-sink" 18092
}

teardown() {
  e2e_teardown_on_failure "kitchen-sink"
}

teardown_file() {
  e2e_cleanup
  e2e_stop_compose
}

@test "observability logs user source and masks configured fields" {
  hurl \
    --variable host="http://localhost:18092" \
    --test \
    "$ROOT/tests/hurl/observability.hurl"

  local app_log="$ROOT/.app-kitchen-sink.log"
  grep -Fq '"msg":"observability sample"' "$app_log"
  grep -Fq '"source":"user"' "$app_log"
  grep -Fq '"authorization":"***"' "$app_log"
  grep -Fq '"password":"***"' "$app_log"
}
