POST {{base_url}}/api/payments
Content-Type: application/json
X-Request-ID: stripe-e2e-{{run_id}}
{
  "amount": 2000,
  "currency": "usd",
  "description": "Stripe integration E2E",
  "customer_email": "stripe-e2e-{{run_id}}@example.com",
  "customer_name": "Stripe E2E",
  "metadata": {
    "order_id": "{{order_id}}"
  }
}
HTTP 201
[Asserts]
jsonpath "$.success" == true
jsonpath "$.data.order_id" == "{{order_id}}"
jsonpath "$.data.payment_intent_id" == "{{payment_intent_id}}"
jsonpath "$.data.client_secret" == "{{payment_intent_id}}_secret"
jsonpath "$.data.checkout_url" contains "{{order_id}}"

GET {{base_url}}/checkout/order/{{order_id}}
HTTP 200
[Asserts]
body contains "{{order_id}}"
