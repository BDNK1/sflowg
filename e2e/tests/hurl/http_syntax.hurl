POST {{host}}/api/kitchen/123?include=summary
Content-Type: application/json
X-Request-ID: e2e-request-1
```json
{
  "customer_email": "alice@example.com",
  "amount_cents": 1499,
  "currency": "usd",
  "items": [
    { "sku": "sku_1", "quantity": 2 },
    { "sku": "sku_2", "quantity": 1 }
  ]
}
```

HTTP 200
[Asserts]
jsonpath "$.ok" == true
jsonpath "$.path_id" == 123
jsonpath "$.include" == "summary"
jsonpath "$.request_id" == "e2e-request-1"
jsonpath "$.currency" == "usd"
jsonpath "$.plugin_message" == "e2e: hello alice@example.com"
jsonpath "$.items[0].sku" == "sku_1"
jsonpath "$.items[0].line_total_cents" == 1000
jsonpath "$.items[1].sku" == "sku_2"
jsonpath "$.line_totals[0]" == 1000
jsonpath "$.line_totals[1]" == 500
