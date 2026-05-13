POST {{host}}/api/async-foreach
Content-Type: application/json
```json
{
  "items": [
    { "sku": "async_1", "quantity": 2 },
    { "sku": "async_2", "quantity": 3 },
    { "sku": "async_3", "quantity": 4 }
  ]
}
```

HTTP 200
[Asserts]
jsonpath "$.multiplier" == 2
jsonpath "$.async_label" == "async-branch"
jsonpath "$.first_sku" == "async_1"
jsonpath "$.normalized_items[0].total" == 4
jsonpath "$.normalized_items[1].total" == 6
jsonpath "$.normalized_items[2].total" == 8
jsonpath "$.batch_first_skus[0]" == "async_1"
jsonpath "$.batch_first_skus[1]" == "async_3"
