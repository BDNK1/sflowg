#!/bin/bash

# Create an order
curl -s -X POST "http://localhost:8080/api/orders" \
  -H "Content-Type: application/json" \
  -d '{"customer_email":"alice@example.com","amount_cents":14999,"currency":"usd"}' | jq .

# Get an order by ID
curl -s "http://localhost:8080/api/orders/1" | jq .

# Create payment via stripe-integration for an order
curl -s -X POST "http://localhost:8080/api/orders/1/pay" | jq .

# Create another order
curl -s -X POST "http://localhost:8080/api/orders" \
  -H "Content-Type: application/json" \
  -d '{"customer_email":"bob@example.com","amount_cents":3950}' | jq .

# Cancel an order
curl -s -X POST "http://localhost:8080/api/orders/2/cancel" | jq .

# Invalid create (should fail with 400)
curl -s -X POST "http://localhost:8080/api/orders" \
  -H "Content-Type: application/json" \
  -d '{"customer_email":"","amount_cents":0}' | jq .

# Get non-existent order (should return 404)
curl -s "http://localhost:8080/api/orders/99999" | jq .
