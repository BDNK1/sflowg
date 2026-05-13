GET {{host}}/api/response/text

HTTP 200
[Asserts]
header "X-E2E-Response" == "text"
body == "plain e2e response"
