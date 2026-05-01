#!/usr/bin/env bash
set -euo pipefail

EXAMPLE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ROOT_DIR="$(cd "$EXAMPLE_DIR/../../.." && pwd)"
TEST_DIR="$EXAMPLE_DIR/e2e-test"
BASE_URL="${BASE_URL:-http://localhost:8090}"
FAKE_STRIPE_URL="${FAKE_STRIPE_URL:-http://127.0.0.1:18090/v1}"
RUN_ID="$(date +%s)-$$"
SAFE_RUN_ID="${RUN_ID//[^a-zA-Z0-9]/_}"
WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/sflowg-stripe-e2e.XXXXXX")"
APP_DIR="$WORK_DIR/stripe-integration"
BIN_DIR="$WORK_DIR/bin"
APP_LOG="$WORK_DIR/stripe-integration.log"
FAKE_STRIPE_LOG="$WORK_DIR/fake-stripe.log"
PAYMENT_INTENT_ID="pi_stripe_e2e_${SAFE_RUN_ID}"

cleanup() {
  if [[ -n "${APP_PID:-}" ]] && kill -0 "$APP_PID" 2>/dev/null; then
    kill "$APP_PID"
    wait "$APP_PID" 2>/dev/null || true
  fi
  if [[ -n "${FAKE_STRIPE_PID:-}" ]] && kill -0 "$FAKE_STRIPE_PID" 2>/dev/null; then
    kill "$FAKE_STRIPE_PID"
    wait "$FAKE_STRIPE_PID" 2>/dev/null || true
  fi
  rm -rf "$WORK_DIR"
}
trap cleanup EXIT

mkdir -p "$BIN_DIR"

cat >"$WORK_DIR/fake_stripe.go" <<'GO'
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
)

func main() {
	paymentIntentID := os.Getenv("FAKE_STRIPE_PAYMENT_INTENT_ID")
	if paymentIntentID == "" {
		paymentIntentID = "pi_stripe_e2e_default"
	}
	http.HandleFunc("/v1/payment_intents", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":            paymentIntentID,
			"client_secret": paymentIntentID + "_secret",
			"status":        "requires_payment_method",
		})
	})
	log.Fatal(http.ListenAndServe("127.0.0.1:18090", nil))
}
GO

echo "starting fake Stripe API"
FAKE_STRIPE_PAYMENT_INTENT_ID="$PAYMENT_INTENT_ID" \
  go run "$WORK_DIR/fake_stripe.go" >"$FAKE_STRIPE_LOG" 2>&1 &
FAKE_STRIPE_PID="$!"

hurl --test --retry 60 --retry-interval 250 \
  --variable "fake_stripe_url=$FAKE_STRIPE_URL" \
  "$TEST_DIR/fake-stripe-ready.hurl"

echo "building CLI"
(
  cd "$ROOT_DIR/cli"
  go build -o "$BIN_DIR/sflowg" .
)

echo "starting stripe-integration Postgres"
docker compose -f "$EXAMPLE_DIR/docker-compose.yml" up -d postgres

cp -R "$EXAMPLE_DIR" "$APP_DIR"
"$BIN_DIR/sflowg" build "$APP_DIR" \
  --core-path "$ROOT_DIR/core" \
  --transports-path "$ROOT_DIR/transports" \
  --core-plugins-path "$ROOT_DIR/plugins"

echo "starting stripe-integration example"
(
  cd "$APP_DIR"
  DATABASE_URL="${DATABASE_URL:-postgres://postgres:postgres@localhost:5433/stripe_payments?sslmode=disable}" \
  STRIPE_API_URL="$FAKE_STRIPE_URL" \
  STRIPE_SECRET_KEY="${STRIPE_SECRET_KEY:-sk_test_stripe_e2e}" \
  STRIPE_WEBHOOK_SECRET="${STRIPE_WEBHOOK_SECRET:-whsec_stripe_e2e}" \
  STRIPE_PUBLISHABLE_KEY="${STRIPE_PUBLISHABLE_KEY:-pk_test_stripe_e2e}" \
  ./stripe-integration >"$APP_LOG" 2>&1
) &
APP_PID="$!"

hurl --test --retry 60 --retry-interval 250 \
  --variable "base_url=$BASE_URL" \
  "$TEST_DIR/ready.hurl"

hurl --test \
  --variable "base_url=$BASE_URL" \
  --variable "run_id=$SAFE_RUN_ID" \
  --variable "order_id=REG-STRIPE-E2E-$SAFE_RUN_ID" \
  --variable "payment_intent_id=$PAYMENT_INTENT_ID" \
  "$TEST_DIR/regression.hurl"

echo "stripe-integration e2e passed"
