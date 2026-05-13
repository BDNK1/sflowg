#!/usr/bin/env bash

postgres_dsn() {
  printf "%s\n" "postgres://sflowg:sflowg@localhost:54329/sflowg_e2e?sslmode=disable"
}

postgres_psql() {
  docker compose -f "$ROOT/docker-compose.yml" exec -T postgres \
    psql -U sflowg -d sflowg_e2e "$@"
}

postgres_reset_orders() {
  postgres_psql <<'SQL'
DROP TABLE IF EXISTS e2e_orders;
CREATE TABLE e2e_orders (
  id TEXT PRIMARY KEY,
  customer_email TEXT NOT NULL,
  amount_cents INTEGER NOT NULL,
  status TEXT NOT NULL
);
SQL
}
