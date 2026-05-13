POST {{host}}/api/error-compensation
Content-Type: application/json
```json
{
  "mode": "fail"
}
```

HTTP 503
[Asserts]
jsonpath "$.code" == "FORCED_COMPENSATION"
jsonpath "$.step" == "fail_after_reserve"
jsonpath "$.compensated" == true
