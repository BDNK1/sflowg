POST {{host}}/api/validation/123
Content-Type: application/json
```json
{
  "amount_cents": 0
}
```

HTTP 400
[Asserts]
header "Content-Type" contains "application/problem+json"
jsonpath "$.title" exists
