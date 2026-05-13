GET {{host}}/api/response/redirect

HTTP 302
[Asserts]
header "Location" == "/api/response/text"
