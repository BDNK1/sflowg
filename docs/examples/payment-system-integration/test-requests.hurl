# Payment System Integration - Hurl Test Suite
# Run with: hurl --test test-requests.hurl
# Run single test: hurl --test test-requests.hurl --very-verbose
# Generate HTML report: hurl --test --report-html report/ test-requests.hurl

# ========================================
# Test 1: Successful Purchase (All Features)
# Tests: Full flow with all validations, calculations, and notifications
# Expected: 200 OK, payment successful
# ========================================
POST http://localhost:8080/api/purchase/ORD-12345
Authorization: Bearer sk_test_token_12345
X-Request-ID: REQ-TEST-001
X-Customer-ID: CUST-67890
Content-Type: application/json
[QueryStringParams]
currency: USD
notify: true
```json
{
  "item": {
    "name": "Premium Widget",
    "price": 49.99,
    "quantity": 3
  },
  "customer": {
    "name": "John Doe",
    "email": "john.doe@example.com"
  },
  "payment": {
    "card": {
      "number": "4532 1488 0343 6467",
      "expiry": "12/25",
      "cvv": "123"
    }
  }
}
```

HTTP 200
[Asserts]
jsonpath "$.success" == true
jsonpath "$.status" == "success"
jsonpath "$.message" == "Payment processed successfully"
jsonpath "$.data.orderId" == "ORD-12345"
jsonpath "$.data.customer.name" == "John Doe"
jsonpath "$.data.customer.email" == "john.doe@example.com"
jsonpath "$.data.pricing.subtotal" == 149.97
jsonpath "$.data.pricing.currency" == "USD"
jsonpath "$.data.payment.cardType" == "visa"
jsonpath "$.data.payment.lastFourDigits" == "6467"
jsonpath "$.data.transactionId" exists
jsonpath "$.data.payment.authorizationCode" exists

# ========================================
# Test 2: Large Order with Discount
# Tests: Discount calculation, high amount validation
# Expected: 200 OK, 10% discount applied
# ========================================
POST http://localhost:8080/api/purchase/ORD-LARGE-001
Authorization: Bearer sk_test_token_12345
X-Request-ID: REQ-TEST-002
X-Customer-ID: CUST-PREMIUM-001
Content-Type: application/json
[QueryStringParams]
currency: USD
notify: true
```json
{
  "item": {
    "name": "Enterprise Package",
    "price": 499.99,
    "quantity": 1
  },
  "customer": {
    "name": "Alice Johnson",
    "email": "alice@enterprise.com"
  },
  "payment": {
    "card": {
      "number": "5500 0000 0000 0004",
      "expiry": "06/26",
      "cvv": "456"
    }
  }
}
```

HTTP 200
[Asserts]
jsonpath "$.success" == true
jsonpath "$.status" == "success"
jsonpath "$.data.pricing.subtotal" == 499.99
jsonpath "$.data.pricing.discount" > 0
jsonpath "$.data.pricing.discountApplied" == true
# 10% discount on 499.99 = 49.999
jsonpath "$.data.pricing.discount" >= 49.90
jsonpath "$.data.pricing.discount" <= 50.00
jsonpath "$.data.payment.cardType" == "mastercard"

# ========================================
# Test 3: Free Shipping Order
# Tests: Shipping threshold, conditional shipping cost
# Expected: 200 OK, free shipping applied
# ========================================
POST http://localhost:8080/api/purchase/ORD-SHIPPING-001
Authorization: Bearer sk_test_token_12345
X-Request-ID: REQ-TEST-003
X-Customer-ID: CUST-FREE-SHIP-001
Content-Type: application/json
[QueryStringParams]
currency: EUR
notify: false
```json
{
  "item": {
    "name": "Standard Product",
    "price": 25.00,
    "quantity": 5
  },
  "customer": {
    "name": "Bob Smith",
    "email": "bob.smith@example.com"
  },
  "payment": {
    "card": {
      "number": "3782 822463 10005",
      "expiry": "03/27",
      "cvv": "1234"
    }
  }
}
```

HTTP 200
[Asserts]
jsonpath "$.success" == true
jsonpath "$.data.pricing.subtotal" == 125.00
jsonpath "$.data.pricing.shipping" == 0.0
jsonpath "$.data.pricing.shippingType" == "free"
jsonpath "$.data.pricing.currency" == "EUR"
jsonpath "$.data.payment.cardType" == "amex"

# ========================================
# Test 4: Invalid Card Number (Luhn Check Fails)
# Tests: Card validation, Luhn algorithm
# Expected: 400 Bad Request, card validation failed
# ========================================
POST http://localhost:8080/api/purchase/ORD-INVALID-CARD-001
Authorization: Bearer sk_test_token_12345
X-Request-ID: REQ-TEST-004
X-Customer-ID: CUST-INVALID-001
Content-Type: application/json
[QueryStringParams]
currency: USD
notify: false
```json
{
  "item": {
    "name": "Test Product",
    "price": 29.99,
    "quantity": 2
  },
  "customer": {
    "name": "Charlie Brown",
    "email": "charlie@example.com"
  },
  "payment": {
    "card": {
      "number": "1234 5678 9012 3456",
      "expiry": "12/25",
      "cvv": "789"
    }
  }
}
```

HTTP 400
[Asserts]
jsonpath "$.success" == false
jsonpath "$.status" == "validation_failed"
jsonpath "$.message" contains "Card validation failed"

# ========================================
# Test 5: Expired Card
# Tests: Expiry date validation
# Expected: 400 Bad Request, card expired
# ========================================
POST http://localhost:8080/api/purchase/ORD-EXPIRED-CARD-001
Authorization: Bearer sk_test_token_12345
X-Request-ID: REQ-TEST-005
X-Customer-ID: CUST-EXPIRED-001
Content-Type: application/json
[QueryStringParams]
currency: USD
notify: false
```json
{
  "item": {
    "name": "Test Product",
    "price": 19.99,
    "quantity": 1
  },
  "customer": {
    "name": "Diana Prince",
    "email": "diana@example.com"
  },
  "payment": {
    "card": {
      "number": "4532 1488 0343 6467",
      "expiry": "01/20",
      "cvv": "123"
    }
  }
}
```

HTTP 400
[Asserts]
jsonpath "$.success" == false
jsonpath "$.status" == "validation_failed"
jsonpath "$.message" contains "Card validation failed"

# ========================================
# Test 6: Order Amount Too Low
# Tests: Minimum amount validation
# Expected: 400 Bad Request, amount below minimum
# ========================================
POST http://localhost:8080/api/purchase/ORD-TOO-LOW-001
Authorization: Bearer sk_test_token_12345
X-Request-ID: REQ-TEST-006
X-Customer-ID: CUST-LOW-001
Content-Type: application/json
[QueryStringParams]
currency: USD
notify: false
```json
{
  "item": {
    "name": "Cheap Item",
    "price": 1.00,
    "quantity": 1
  },
  "customer": {
    "name": "Eve Anderson",
    "email": "eve@example.com"
  },
  "payment": {
    "card": {
      "number": "4532 1488 0343 6467",
      "expiry": "12/25",
      "cvv": "123"
    }
  }
}
```

HTTP 400
[Asserts]
jsonpath "$.success" == false
jsonpath "$.status" == "amount_invalid"
jsonpath "$.message" contains "below minimum"

# ========================================
# Test 7: Order Amount Too High
# Tests: Maximum amount validation
# Expected: 400 Bad Request, amount exceeds maximum
# ========================================
POST http://localhost:8080/api/purchase/ORD-TOO-HIGH-001
Authorization: Bearer sk_test_token_12345
X-Request-ID: REQ-TEST-007
X-Customer-ID: CUST-HIGH-001
Content-Type: application/json
[QueryStringParams]
currency: USD
notify: false
```json
{
  "item": {
    "name": "Expensive Item",
    "price": 50000.00,
    "quantity": 1
  },
  "customer": {
    "name": "Frank Castle",
    "email": "frank@example.com"
  },
  "payment": {
    "card": {
      "number": "4532 1488 0343 6467",
      "expiry": "12/25",
      "cvv": "123"
    }
  }
}
```

HTTP 400
[Asserts]
jsonpath "$.success" == false
jsonpath "$.status" == "amount_invalid"
jsonpath "$.message" contains "exceeds maximum"

# ========================================
# Test 8: Multiple Quantity with Tax
# Tests: Quantity multiplication, tax calculation
# Expected: 200 OK, correct tax and total
# ========================================
POST http://localhost:8080/api/purchase/ORD-MULTI-QTY-001
Authorization: Bearer sk_test_token_12345
X-Request-ID: REQ-TEST-008
X-Customer-ID: CUST-MULTI-001
Content-Type: application/json
[QueryStringParams]
currency: GBP
notify: true
```json
{
  "item": {
    "name": "Bulk Item",
    "price": 15.50,
    "quantity": 10
  },
  "customer": {
    "name": "Grace Hopper",
    "email": "grace@example.com"
  },
  "payment": {
    "card": {
      "number": "5500 0000 0000 0004",
      "expiry": "09/26",
      "cvv": "321"
    }
  }
}
```

HTTP 200
[Asserts]
jsonpath "$.success" == true
jsonpath "$.data.pricing.subtotal" == 155.00
# Tax should be 15% of subtotal = 23.25
jsonpath "$.data.pricing.tax" >= 23.20
jsonpath "$.data.pricing.tax" <= 23.30
jsonpath "$.data.pricing.currency" == "GBP"

# ========================================
# Test 9: No Notification (Query Param Test)
# Tests: Query parameter handling, conditional notification
# Expected: 200 OK, no notification sent
# ========================================
POST http://localhost:8080/api/purchase/ORD-NO-NOTIFY-001
Authorization: Bearer sk_test_token_12345
X-Request-ID: REQ-TEST-009
X-Customer-ID: CUST-NO-NOTIFY-001
Content-Type: application/json
[QueryStringParams]
currency: USD
```json
{
  "item": {
    "name": "Silent Order",
    "price": 75.00,
    "quantity": 1
  },
  "customer": {
    "name": "Henry Ford",
    "email": "henry@example.com"
  },
  "payment": {
    "card": {
      "number": "4532 1488 0343 6467",
      "expiry": "12/25",
      "cvv": "123"
    }
  }
}
```

HTTP 200
[Asserts]
jsonpath "$.success" == true
jsonpath "$.data.pricing.subtotal" == 75.00
# Notification should not be sent (notify param not set to true)
jsonpath "$.data.metadata.notificationSent" == false

# ========================================
# Test 10: Discount + Free Shipping Combo
# Tests: Multiple feature interactions
# Expected: 200 OK, both discount and free shipping
# ========================================
POST http://localhost:8080/api/purchase/ORD-COMBO-001
Authorization: Bearer sk_test_token_12345
X-Request-ID: REQ-TEST-010
X-Customer-ID: CUST-COMBO-001
Content-Type: application/json
[QueryStringParams]
currency: USD
notify: true
```json
{
  "item": {
    "name": "Premium Bundle",
    "price": 89.99,
    "quantity": 3
  },
  "customer": {
    "name": "Irene Adler",
    "email": "irene@example.com"
  },
  "payment": {
    "card": {
      "number": "3782 822463 10005",
      "expiry": "08/27",
      "cvv": "9876"
    }
  }
}
```

HTTP 200
[Asserts]
jsonpath "$.success" == true
jsonpath "$.data.pricing.subtotal" == 269.97
# Should have discount (over $200 threshold)
jsonpath "$.data.pricing.discountApplied" == true
jsonpath "$.data.pricing.discount" > 0
# Should have free shipping (over $100 threshold)
jsonpath "$.data.pricing.shipping" == 0.0
jsonpath "$.data.pricing.shippingType" == "free"

# ========================================
# Test 11: Visa Card Type Detection
# Tests: Card type detection for Visa
# Expected: 200 OK, card type = visa
# ========================================
POST http://localhost:8080/api/purchase/ORD-VISA-001
Authorization: Bearer sk_test_token_12345
X-Request-ID: REQ-TEST-011
X-Customer-ID: CUST-VISA-001
Content-Type: application/json
[QueryStringParams]
currency: USD
notify: false
```json
{
  "item": {
    "name": "Test Product",
    "price": 50.00,
    "quantity": 1
  },
  "customer": {
    "name": "Visa Customer",
    "email": "visa@example.com"
  },
  "payment": {
    "card": {
      "number": "4532 1488 0343 6467",
      "expiry": "12/25",
      "cvv": "123"
    }
  }
}
```

HTTP 200
[Asserts]
jsonpath "$.success" == true
jsonpath "$.data.payment.cardType" == "visa"
jsonpath "$.data.payment.lastFourDigits" == "6467"

# ========================================
# Test 12: MasterCard Type Detection
# Tests: Card type detection for MasterCard
# Expected: 200 OK, card type = mastercard
# ========================================
POST http://localhost:8080/api/purchase/ORD-MC-001
Authorization: Bearer sk_test_token_12345
X-Request-ID: REQ-TEST-012
X-Customer-ID: CUST-MC-001
Content-Type: application/json
[QueryStringParams]
currency: USD
notify: false
```json
{
  "item": {
    "name": "Test Product",
    "price": 50.00,
    "quantity": 1
  },
  "customer": {
    "name": "MasterCard Customer",
    "email": "mc@example.com"
  },
  "payment": {
    "card": {
      "number": "5500 0000 0000 0004",
      "expiry": "06/26",
      "cvv": "456"
    }
  }
}
```

HTTP 200
[Asserts]
jsonpath "$.success" == true
jsonpath "$.data.payment.cardType" == "mastercard"
jsonpath "$.data.payment.lastFourDigits" == "0004"

# ========================================
# Test 13: American Express Type Detection
# Tests: Card type detection for Amex
# Expected: 200 OK, card type = amex
# ========================================
POST http://localhost:8080/api/purchase/ORD-AMEX-001
Authorization: Bearer sk_test_token_12345
X-Request-ID: REQ-TEST-013
X-Customer-ID: CUST-AMEX-001
Content-Type: application/json
[QueryStringParams]
currency: USD
notify: false
```json
{
  "item": {
    "name": "Test Product",
    "price": 50.00,
    "quantity": 1
  },
  "customer": {
    "name": "Amex Customer",
    "email": "amex@example.com"
  },
  "payment": {
    "card": {
      "number": "3782 822463 10005",
      "expiry": "03/27",
      "cvv": "1234"
    }
  }
}
```

HTTP 200
[Asserts]
jsonpath "$.success" == true
jsonpath "$.data.payment.cardType" == "amex"
jsonpath "$.data.payment.lastFourDigits" == "0005"
