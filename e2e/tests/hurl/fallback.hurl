POST {{host}}/api/retry-fallback
Content-Type: application/json
```json
{
  "mode": "fallback"
}
```

HTTP 200
[Asserts]
jsonpath "$.mode" == "fallback"
jsonpath "$.used_retry" == false
jsonpath "$.used_fallback" == true
