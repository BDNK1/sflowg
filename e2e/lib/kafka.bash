#!/usr/bin/env bash

kafka_rpk() {
  docker compose -f "$ROOT/docker-compose.yml" exec -T redpanda rpk "$@"
}

kafka_topic_create() {
  local topic="$1"

  for _ in $(seq 1 30); do
    if kafka_rpk topic create "$topic" >/dev/null 2>&1; then
      return 0
    fi
    if kafka_rpk topic describe "$topic" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done

  echo "Timed out creating Kafka topic $topic" >&2
  return 1
}

kafka_produce_json() {
  local topic="$1"
  local payload="$2"

  printf "%s\n" "$payload" | kafka_rpk topic produce "$topic" >/dev/null
}
