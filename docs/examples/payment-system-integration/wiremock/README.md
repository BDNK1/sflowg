# WireMock Stubs for Payment System Integration

This directory contains WireMock stub mappings for all external HTTP services used in the payment flow.

## 🎯 Mocked Services

### 1. Inventory Service (Port 9001)
**Base URL**: `http://localhost:9001`

**Endpoints**:
- `GET /inventory?item_name={name}&quantity={qty}` - Check inventory availability

**Scenarios**:
- ✅ **Success** - Item available with sufficient stock
- ⚠️ **Low Stock** - Item available but low quantity (item_name="Limited Item")
- ❌ **Out of Stock** - Item not available (item_name="Out of Stock Item")

### 2. Payment Provider API (Port 9002)
**Base URL**: `http://localhost:9002/v1`

**Endpoints**:
- `POST /v1/charges` - Create a charge

**Scenarios**:
- ✅ **Success** - Normal payment processing
- ❌ **Insufficient Funds** - Amount > $10,000
- 🔄 **Rate Limited** - Add header `X-Test-Scenario: rate-limit` (tests retry logic)
- ⏱️ **Timeout** - Add header `X-Test-Scenario: timeout` (5 second delay)

**Retry Testing**:
The rate-limit scenario uses WireMock's state machine:
1. First request → 429 Too Many Requests
2. Second request → 200 Success (demonstrates retry working)

### 3. Notification Service (Port 9000)
**Base URL**: `http://localhost:9000`

**Endpoints**:
- `POST /notify` - Send notification email

**Scenarios**:
- ✅ **Success** - Email sent successfully
- 🔄 **Failure then Success** - Add header `X-Test-Scenario: notification-failure` (tests retry logic)

## 🚀 Quick Start

### Option 1: Docker Compose (Recommended)

Run all mock services:
```bash
docker-compose up -d inventory-mock payment-provider-mock notification-mock
```

Check logs:
```bash
docker-compose logs -f inventory-mock
docker-compose logs -f payment-provider-mock
docker-compose logs -f notification-mock
```

Stop services:
```bash
docker-compose down
```

### Option 2: Standalone WireMock (Individual Services)

**Install WireMock**:
```bash
# Download WireMock standalone JAR
curl -o wiremock-standalone.jar https://repo1.maven.org/maven2/org/wiremock/wiremock-standalone/3.3.1/wiremock-standalone-3.3.1.jar
```

**Run services**:

Terminal 1 - Inventory Service:
```bash
java -jar wiremock-standalone.jar \
  --port 9001 \
  --root-dir ./wiremock \
  --global-response-templating \
  --verbose
```

Terminal 2 - Payment Provider:
```bash
java -jar wiremock-standalone.jar \
  --port 9002 \
  --root-dir ./wiremock \
  --global-response-templating \
  --verbose
```

Terminal 3 - Notification Service:
```bash
java -jar wiremock-standalone.jar \
  --port 9000 \
  --root-dir ./wiremock \
  --global-response-templating \
  --verbose
```

### Option 3: NPM WireMock

```bash
npm install -g wiremock

# Run each service in separate terminals
wiremock --port 9001 --root-dir ./wiremock --global-response-templating
wiremock --port 9002 --root-dir ./wiremock --global-response-templating
wiremock --port 9000 --root-dir ./wiremock --global-response-templating
```

## 🔧 Configuration

Update URLs in `flow-config.yaml` to use local mocks:

```yaml
# For local development with WireMock
properties:
  inventoryServiceURL: "http://localhost:9001/inventory"
  paymentProviderURL: "http://localhost:9002/v1/charges"
  notificationServiceURL: "http://localhost:9000/notify"
```

Or use environment variables:
```bash
export INVENTORY_SERVICE_URL=http://localhost:9001/inventory
export PAYMENT_PROVIDER_URL=http://localhost:9002/v1/charges
export NOTIFICATION_SERVICE_URL=http://localhost:9000/notify
```

## 🧪 Testing Scenarios

### Test 1: Successful Purchase (All Services Working)
```bash
# All mocks return success
curl -X POST http://localhost:8080/api/purchase/ORD-001?currency=USD&notify=true \
  -H "Authorization: Bearer sk_test_token" \
  -H "X-Customer-ID: CUST-001" \
  -H "Content-Type: application/json" \
  -d '{
    "item": {"name": "Premium Widget", "price": 49.99, "quantity": 3},
    "customer": {"name": "John Doe", "email": "john@example.com"},
    "payment": {"card": {"number": "4532 1488 0343 6467", "expiry": "12/25", "cvv": "123"}}
  }'
```

### Test 2: Retry Logic - Rate Limited Payment Provider
```bash
# Payment provider rate limits first request, succeeds on retry
curl -X POST http://localhost:8080/api/purchase/ORD-002?currency=USD&notify=false \
  -H "Authorization: Bearer sk_test_token" \
  -H "X-Customer-ID: CUST-002" \
  -H "X-Test-Scenario: rate-limit" \
  -H "Content-Type: application/json" \
  -d '{
    "item": {"name": "Test Product", "price": 29.99, "quantity": 1},
    "customer": {"name": "Retry Test", "email": "retry@example.com"},
    "payment": {"card": {"number": "4532 1488 0343 6467", "expiry": "12/25", "cvv": "123"}}
  }'

# Check WireMock logs to see:
# 1. First request: 429 Rate Limited
# 2. Second request: 200 Success (after retry)
```

### Test 3: Notification Retry
```bash
# Notification service fails first, succeeds on retry
curl -X POST http://localhost:8080/api/purchase/ORD-003?currency=USD&notify=true \
  -H "Authorization: Bearer sk_test_token" \
  -H "X-Customer-ID: CUST-003" \
  -H "X-Test-Scenario: notification-failure" \
  -H "Content-Type: application/json" \
  -d '{
    "item": {"name": "Test Product", "price": 49.99, "quantity": 1},
    "customer": {"name": "Notification Test", "email": "notify@example.com"},
    "payment": {"card": {"number": "4532 1488 0343 6467", "expiry": "12/25", "cvv": "123"}}
  }'
```

### Test 4: Out of Stock
```bash
# Inventory service returns out of stock
curl -X POST http://localhost:8080/api/purchase/ORD-004?currency=USD&notify=false \
  -H "Authorization: Bearer sk_test_token" \
  -H "X-Customer-ID: CUST-004" \
  -H "Content-Type: application/json" \
  -d '{
    "item": {"name": "Out of Stock Item", "price": 99.99, "quantity": 1},
    "customer": {"name": "Stock Test", "email": "stock@example.com"},
    "payment": {"card": {"number": "4532 1488 0343 6467", "expiry": "12/25", "cvv": "123"}}
  }'
```

### Test 5: Payment Insufficient Funds
```bash
# Payment provider returns insufficient funds error
curl -X POST http://localhost:8080/api/purchase/ORD-005?currency=USD&notify=false \
  -H "Authorization: Bearer sk_test_token" \
  -H "X-Customer-ID: CUST-005" \
  -H "Content-Type: application/json" \
  -d '{
    "item": {"name": "Expensive Item", "price": 15000.00, "quantity": 1},
    "customer": {"name": "Big Spender", "email": "spender@example.com"},
    "payment": {"card": {"number": "4532 1488 0343 6467", "expiry": "12/25", "cvv": "123"}}
  }'
```

## 📊 WireMock Admin API

**View all mappings**:
```bash
curl http://localhost:9001/__admin/mappings
curl http://localhost:9002/__admin/mappings
curl http://localhost:9000/__admin/mappings
```

**View requests received**:
```bash
curl http://localhost:9001/__admin/requests
```

**Reset scenarios** (clears state):
```bash
curl -X POST http://localhost:9002/__admin/scenarios/reset
curl -X POST http://localhost:9000/__admin/scenarios/reset
```

**Reset all**:
```bash
curl -X POST http://localhost:9001/__admin/reset
curl -X POST http://localhost:9002/__admin/reset
curl -X POST http://localhost:9000/__admin/reset
```

## 📁 Directory Structure

```
wiremock/
├── mappings/                                # Stub definitions
│   ├── inventory-check-success.json         # ✅ Item available
│   ├── inventory-check-low-stock.json       # ⚠️ Low stock warning
│   ├── inventory-check-out-of-stock.json    # ❌ Out of stock
│   ├── payment-provider-success.json        # ✅ Payment successful
│   ├── payment-provider-insufficient-funds.json # ❌ Insufficient funds
│   ├── payment-provider-rate-limit.json     # 🔄 Rate limit (retry test)
│   ├── payment-provider-rate-limit-retry-success.json
│   ├── payment-provider-timeout.json        # ⏱️ Timeout
│   ├── notification-success.json            # ✅ Email sent
│   ├── notification-failure.json            # 🔄 Failure (retry test)
│   └── notification-retry-success.json
└── README.md                                # This file
```

## 🎓 Learning WireMock Features

The stubs demonstrate these WireMock capabilities:

### Request Matching
```json
{
  "request": {
    "method": "GET",
    "urlPath": "/inventory",
    "queryParameters": {
      "item_name": { "matches": ".*" }
    },
    "headers": {
      "Authorization": { "matches": "Bearer .*" }
    }
  }
}
```

### Response Templating
```json
{
  "response": {
    "jsonBody": {
      "item_name": "{{request.query.item_name}}",
      "transaction_id": "{{jsonPath request.body '$.transaction_id'}}"
    }
  }
}
```

### Scenarios (State Machine)
```json
{
  "scenarioName": "rate-limit-retry",
  "requiredScenarioState": "Started",
  "newScenarioState": "First-Retry"
}
```

### Priority (Specific Matches Override General)
```json
{
  "priority": 2,
  "request": {
    "queryParameters": {
      "item_name": { "equalTo": "Out of Stock Item" }
    }
  }
}
```

### Random Values
```json
{
  "id": "ch_{{randomValue type='ALPHANUMERIC' length=24}}",
  "uuid": "{{randomValue type='UUID'}}"
}
```

### Delays (Timeout Testing)
```json
{
  "response": {
    "fixedDelayMilliseconds": 5000,
    "status": 504
  }
}
```

## 🔗 Resources

- [WireMock Documentation](https://wiremock.org/docs/)
- [Response Templating](https://wiremock.org/docs/response-templating/)
- [Stateful Behaviour](https://wiremock.org/docs/stateful-behaviour/)
- [Request Matching](https://wiremock.org/docs/request-matching/)

## 🐛 Troubleshooting

**Port already in use**:
```bash
# Find process using port
lsof -i :9001

# Kill process
kill -9 <PID>
```

**Stubs not loading**:
- Check mappings directory path is correct
- Ensure JSON files are valid
- Use `--verbose` flag to see detailed logs

**Response templating not working**:
- Add `--global-response-templating` flag
- Check Handlebars syntax in responses
