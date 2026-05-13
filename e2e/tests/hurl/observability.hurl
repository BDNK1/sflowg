POST {{host}}/api/observability
Authorization: Bearer secret-token
Content-Type: application/json
```json
{
  "password": "secret-password",
  "route": "observability"
}
```

HTTP 200
[Asserts]
jsonpath "$.ok" == true
