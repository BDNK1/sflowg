#!/usr/bin/env bash

WIREMOCK_URL="${WIREMOCK_URL:-http://localhost:8089}"

wiremock_reset() {
  curl -fsS -X POST "$WIREMOCK_URL/__admin/reset" >/dev/null
}

wiremock_requests() {
  curl -fsS "$WIREMOCK_URL/__admin/requests"
}

wiremock_unmatched_requests() {
  curl -fsS "$WIREMOCK_URL/__admin/requests/unmatched"
}

wiremock_verify_url() {
  local url="$1"

  wiremock_requests \
    | jq -e --arg url "$url" '.requests[] | select(.request.url == $url)' >/dev/null
}

wiremock_verify_method_url() {
  local method="$1"
  local url="$2"

  wiremock_requests \
    | jq -e --arg method "$method" --arg url "$url" \
      '.requests[] | select(.request.method == $method and .request.url == $url)' >/dev/null
}

wiremock_count_url() {
  local url="$1"

  wiremock_requests \
    | jq --arg url "$url" '[.requests[] | select(.request.url == $url)] | length'
}

wiremock_verify_url_count() {
  local url="$1"
  local want="$2"
  local got

  got="$(wiremock_count_url "$url")"
  if [[ "$got" != "$want" ]]; then
    echo "expected WireMock request count for $url to be $want, got $got" >&2
    return 1
  fi
}

wiremock_verify_body_contains() {
  local url="$1"
  local pattern="$2"

  wiremock_requests \
    | jq -e --arg url "$url" --arg pattern "$pattern" \
      '.requests[] | select(.request.url == $url and (.request.body // "" | contains($pattern)))' >/dev/null
}

wiremock_wait_for_url() {
  local url="$1"
  local timeout="${2:-30}"

  for _ in $(seq 1 "$timeout"); do
    if wiremock_verify_url "$url"; then
      return 0
    fi
    sleep 1
  done

  echo "Timed out waiting for WireMock request $url" >&2
  return 1
}
