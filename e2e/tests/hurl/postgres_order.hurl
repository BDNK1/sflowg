POST {{host}}/api/postgres/orders
Content-Type: application/json
```json
{
  "id": "ord_pg_1",
  "customer_email": "db@example.com",
  "amount_cents": 2500
}
```

HTTP 201
[Asserts]
jsonpath "$.inserted_status" == "new"
jsonpath "$.affected_rows" == 1
jsonpath "$.order.id" == "ord_pg_1"
jsonpath "$.order.customer_email" == "db@example.com"
jsonpath "$.order.status" == "paid"
