POST {{host}}/api/retry-fallback
Content-Type: application/json
```json
{
  "mode": "retry"
}
```

HTTP 200
[Asserts]
jsonpath "$.mode" == "retry"
jsonpath "$.used_retry" == true
jsonpath "$.used_fallback" == false
