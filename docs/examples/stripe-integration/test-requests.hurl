# Stripe Integration Test Suite
# Run with: hurl --test test-requests.hurl
# Run verbose: hurl --test --very-verbose test-requests.hurl

# ============================================
# Test 1: Create Payment - Success
# ============================================
POST http://localhost:8080/api/payments
Content-Type: application/json
X-Request-ID: test-001
{
  "amount": 2000,
  "currency": "usd",
  "description": "Test payment for order #12345",
  "customer_email": "test@example.com",
  "customer_name": "John Doe",
  "metadata": {
    "order_id": "ORD-12345"
  }
}

HTTP 201
[Asserts]
jsonpath "$.success" == true
jsonpath "$.data.payment_id" exists
jsonpath "$.data.client_secret" exists
jsonpath "$.data.payment_intent_id" matches "^pi_"
jsonpath "$.data.status" == "requires_payment_method"
jsonpath "$.data.amount" == 2000
jsonpath "$.data.currency" == "usd"

[Captures]
payment_id: jsonpath "$.data.payment_id"
payment_intent_id: jsonpath "$.data.payment_intent_id"


# ============================================
# Test 2: Create Payment - Minimum Amount
# ============================================
POST http://localhost:8080/api/payments
Content-Type: application/json
X-Request-ID: test-002
{
  "amount": 50,
  "currency": "usd",
  "description": "Minimum amount test"
}

HTTP 201
[Asserts]
jsonpath "$.success" == true
jsonpath "$.data.amount" == 50


# ============================================
# Test 3: Create Payment - Amount Too Low (Error)
# ============================================
POST http://localhost:8080/api/payments
Content-Type: application/json
X-Request-ID: test-003
{
  "amount": 49,
  "currency": "usd",
  "description": "Below minimum amount"
}

HTTP 400
[Asserts]
jsonpath "$.success" == false
jsonpath "$.error.code" == "invalid_amount"
jsonpath "$.error.message" contains "at least 50 cents"


# ============================================
# Test 4: Create Payment - Missing Amount (Error)
# ============================================
POST http://localhost:8080/api/payments
Content-Type: application/json
X-Request-ID: test-004
{
  "currency": "usd",
  "description": "Missing amount"
}

HTTP 400
[Asserts]
jsonpath "$.success" == false


# ============================================
# Test 5: Create Payment - Large Amount
# ============================================
POST http://localhost:8080/api/payments
Content-Type: application/json
X-Request-ID: test-005
{
  "amount": 999999,
  "currency": "usd",
  "description": "Large payment test",
  "customer_email": "vip@example.com"
}

HTTP 201
[Asserts]
jsonpath "$.success" == true
jsonpath "$.data.amount" == 999999


# ============================================
# Test 6: Create Payment - EUR Currency
# ============================================
POST http://localhost:8080/api/payments
Content-Type: application/json
X-Request-ID: test-006
{
  "amount": 1500,
  "currency": "eur",
  "description": "Euro payment test"
}

HTTP 201
[Asserts]
jsonpath "$.success" == true
jsonpath "$.data.currency" == "eur"


# ============================================
# Test 7: Create Payment - Default Currency (USD)
# ============================================
POST http://localhost:8080/api/payments
Content-Type: application/json
X-Request-ID: test-007
{
  "amount": 1000,
  "description": "Default currency test"
}

HTTP 201
[Asserts]
jsonpath "$.success" == true
jsonpath "$.data.currency" == "usd"


# ============================================
# Test 8: Webhook - Invalid Signature (Error)
# ============================================
POST http://localhost:8080/api/webhooks/stripe
Content-Type: application/json
Stripe-Signature: t=1234567890,v1=invalid_signature
{
  "id": "evt_test_invalid",
  "type": "payment_intent.succeeded",
  "data": {
    "object": {
      "id": "pi_test_123",
      "status": "succeeded"
    }
  }
}

HTTP 400
[Asserts]
jsonpath "$.error" == "invalid_signature"


# ============================================
# Test 9: Create Payment - With All Fields
# ============================================
POST http://localhost:8080/api/payments
Content-Type: application/json
X-Request-ID: test-009
{
  "amount": 5000,
  "currency": "usd",
  "description": "Complete payment with all fields",
  "customer_email": "complete@example.com",
  "customer_name": "Jane Smith",
  "metadata": {
    "order_id": "ORD-COMPLETE-001"
  }
}

HTTP 201
[Asserts]
jsonpath "$.success" == true
jsonpath "$.data.payment_id" exists
jsonpath "$.data.client_secret" exists
jsonpath "$.data.payment_intent_id" exists
jsonpath "$.data.amount" == 5000