GET {{host}}/api/plugin/hello?name=Alice

HTTP 200
[Asserts]
jsonpath "$.message" == "e2e: hello Alice"
