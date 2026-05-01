POST {{base_url}}/api/orders
Content-Type: application/json
{
  "customer_email": "ecom-e2e-alice-{{run_id}}@example.com",
  "customer_name": "Alice E2E",
  "amount_cents": 14999,
  "currency": "usd"
}
HTTP 201
[Captures]
order_id: jsonpath "$.order.id"
[Asserts]
jsonpath "$.order.customer_email" == "ecom-e2e-alice-{{run_id}}@example.com"
jsonpath "$.order.currency" == "usd"
jsonpath "$.order.status" == "pending"

GET {{base_url}}/api/orders/{{order_id}}
HTTP 200
[Asserts]
jsonpath "$.order.id" == {{order_id}}
jsonpath "$.order.customer_email" == "ecom-e2e-alice-{{run_id}}@example.com"

POST {{base_url}}/api/orders
Content-Type: application/json
{
  "customer_email": "ecom-e2e-bob-{{run_id}}@example.com",
  "customer_name": "Bob E2E",
  "amount_cents": 3950
}
HTTP 201
[Captures]
cancel_id: jsonpath "$.order.id"
[Asserts]
jsonpath "$.order.currency" == "usd"

POST {{base_url}}/api/orders/{{cancel_id}}/cancel
HTTP 200
[Asserts]
jsonpath "$.order.id" == {{cancel_id}}
jsonpath "$.order.status" == "canceled"

POST {{base_url}}/internal/payments/stripe-webhook
Content-Type: application/json
{
  "order_id": "{{order_id}}",
  "payment_intent_id": "pi_ecom_e2e_{{run_id}}",
  "event_type": "payment_intent.succeeded",
  "status": "succeeded"
}
HTTP 200
[Asserts]
jsonpath "$.accepted" == true
jsonpath "$.updated" == true

GET {{base_url}}/api/orders/{{order_id}}
HTTP 200
[Asserts]
jsonpath "$.order.id" == {{order_id}}
jsonpath "$.order.status" == "paid"

GET {{base_url}}/api/orders/999999999
HTTP 404
[Asserts]
jsonpath "$.error" == "order not found"
