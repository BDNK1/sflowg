# Payment System Integration - Comprehensive Feature Test

This example demonstrates **all major features** of the SFlowG plugin system and flow execution engine through a realistic payment processing scenario.

## 📋 Overview

This example implements a complete payment processing flow that tests ~155 features across 10 major categories:

- ✅ Plugin System (local plugins, config, lifecycle, typed tasks)
- ✅ CLI Build System (code generation, dependency resolution)
- ✅ HTTP Plugin (requests, retry, timeout)
- ✅ Flow Execution (variables, expressions, conditionals)
- ✅ Retry & Timeout Logic
- ✅ Conditional Branching (switch/if)
- ✅ Assignment Operations
- ✅ Lifecycle Management (Initialize/Shutdown)
- ✅ Container Management
- ✅ Integration Testing

See [TEST_FEATURES.md](./TEST_FEATURES.md) for the complete feature checklist.

## 🏗️ Architecture

```
payment-system-integration/
├── flow-config.yaml          # Plugin configuration with env var defaults
├── docker-compose.yml        # Docker Compose for app + WireMock mocks
├── flows/
│   └── purchase_flow.yaml    # Comprehensive flow testing all features
├── plugins/
│   ├── uuidgen/              # Local plugin: UUID generation
│   │   ├── go.mod
│   │   └── plugin.go         # 3 tasks: Generate, GenerateV4, Validate
│   └── payment/              # Local plugin: Payment processing
│       ├── go.mod
│       └── plugin.go         # 4 tasks: ValidateCard, ProcessPayment, RefundPayment, GetStatus
├── wiremock/                 # WireMock stubs for external services
│   ├── mappings/             # 13 stub definitions
│   ├── README.md             # WireMock documentation
│   ├── run-standalone.sh     # Run WireMock without Docker
│   └── test-mocks.sh         # Test WireMock services
├── test-requests.http        # IntelliJ/VS Code HTTP requests
├── test-requests.hurl        # Hurl test suite with assertions
├── TEST_FEATURES.md          # Detailed feature test checklist
└── README.md                 # This file
```

## 🔌 Plugins

### 1. HTTP Plugin (Core)
**Source**: `github.com/sflowg/sflowg/plugins/http`

**Tasks**:
- `http.request` - Execute HTTP requests with retry/timeout

**Configuration**:
```yaml
timeout: 30s
max_retries: 3
debug: false
retry_wait_ms: 100
```

**Features Tested**:
- ✅ Core plugin auto-detection
- ✅ Config with environment variables
- ✅ Retry logic with exponential backoff
- ✅ Request/response handling
- ✅ Headers, query parameters, body

### 2. UUID Generator Plugin (Local)
**Source**: `./plugins/uuidgen`

**Tasks**:
- `uuidgen.generateUUID` - Generate UUID v4
- `uuidgen.generateV4` - Alias for generateUUID
- `uuidgen.validateUUID` - Validate UUID format

**Features Tested**:
- ✅ Local module detection (requires go.mod)
- ✅ Simple plugin with no configuration
- ✅ Lifecycle hooks (Initialize/Shutdown)
- ✅ Multiple tasks per plugin
- ✅ Typed and untyped tasks
- ✅ Shared state (generation counter)

### 3. Payment Plugin (Local)
**Source**: `./plugins/payment`

**Tasks**:
- `payment.validateCard` - Validate credit card (Luhn, expiry, CVV)
- `payment.processPayment` - Process payment transaction
- `payment.refundPayment` - Process refund
- `payment.getStatus` - Get plugin statistics

**Configuration**:
```yaml
provider_name: StripeProvider
api_base_url: https://api.stripe.com/v1
api_key: ${PAYMENT_API_KEY}  # Required
timeout: 30s
max_retries: 3
min_amount: 0.01
max_amount: 999999.99
default_currency: USD
enable_refunds: true
enable_validation: true
debug_mode: false
```

**Features Tested**:
- ✅ Local module with extensive configuration
- ✅ Config defaults (`default:"value"` tags)
- ✅ Config validation (`validate:"required"` tags)
- ✅ Environment variable overrides
- ✅ Complex types (time.Duration, float64, bool)
- ✅ Lifecycle management with logging
- ✅ Shared state (transaction counters)
- ✅ Typed tasks (ValidateCard, ProcessPayment)
- ✅ Untyped tasks (RefundPayment, GetStatus)
- ✅ Business logic (Luhn algorithm, expiry validation, card type detection)

## 🌊 Flow Features

The `purchase_flow.yaml` implements a complete purchase workflow testing:

### Variable Resolution
```yaml
# ✅ From request path variables
orderId: request.pathVariables.orderId

# ✅ From request headers
authToken: request.headers.Authorization

# ✅ From request body (nested)
itemName: request.body.item.name

# ✅ From query parameters with defaults
currency: 'request.queryParameters.currency != "" ? request.queryParameters.currency : "USD"'

# ✅ From properties
taxRate: properties.taxRate

# ✅ From previous step results
transactionId: generateTransactionId.result.uuid
```

### Expressions
```yaml
# ✅ Arithmetic
subtotal: extractRequestData.itemPrice * extractRequestData.quantity

# ✅ Comparisons
applyFreeShipping: calculateSubtotal.subtotal >= properties.freeShippingThreshold

# ✅ Logical operators
amountValid: >
  calculateTotal.totalAmount >= properties.minOrderAmount &&
  calculateTotal.totalAmount <= properties.maxOrderAmount

# ✅ Complex calculations
totalAmount: >
  calculateSubtotal.subtotal +
  shippingCost -
  applyDiscount.discountAmount +
  calculateTax.taxAmount

# ✅ String operations
description: '"Purchase: " + extractRequestData.itemName + " x " + extractRequestData.quantity'
```

### Conditional Execution
```yaml
# ✅ Conditional step execution
- id: calculateTax
  type: assign
  condition: properties.enableTax == true
  args:
    taxAmount: calculateSubtotal.subtotal * properties.taxRate

# ✅ Complex conditions
- id: sendNotification
  type: http.request
  condition: >
    properties.enableNotifications == true &&
    extractRequestData.shouldNotify == true &&
    paymentSuccessful.finalStatus == "success"
```

### Switch/Branching
```yaml
# ✅ Multiple conditional branches
- id: determineShipping
  type: switch
  args:
    applyFreeShipping: calculateSubtotal.subtotal >= properties.freeShippingThreshold
    applyStandardShipping: calculateSubtotal.subtotal < properties.freeShippingThreshold

# ✅ First matching branch executes
- id: applyFreeShipping
  type: assign
  args:
    shippingCost: 0.0

- id: applyStandardShipping
  type: assign
  args:
    shippingCost: properties.shippingCost
```

### Retry Logic
```yaml
# ✅ Retry with condition
- id: processPayment
  type: payment.processPayment
  args: {...}
  retry:
    maxRetries: 3
    delay: 1000
    backoff: true          # Exponential backoff
    condition: processPayment.result.status != "completed"

# ✅ Retry on HTTP errors
- id: callPaymentProviderAPI
  type: http.request
  args: {...}
  retry:
    maxRetries: 5
    delay: 500
    backoff: true
    condition: callPaymentProviderAPI.result.statusCode >= 500
```

## 🚀 Getting Started

### Prerequisites

1. Go 1.23 or higher
2. SFlowG CLI installed

### Setup

1. **Navigate to example directory**:
```bash
cd docs/examples/payment-system-integration
```

2. **Set required environment variable**:

All configuration is in `flow-config.yaml` with sensible defaults. The only **required** environment variable is:

```bash
export PAYMENT_API_KEY=sk_test_your_key_here
```

Optional overrides (all have defaults in flow-config.yaml):
```bash
export PAYMENT_PROVIDER=StripeProvider  # Provider name (default: StripeProvider)
export PAYMENT_TIMEOUT=30s          # Timeout (default: 30s)
export HTTP_DEBUG=false             # Debug logging (default: false)
# See flow-config.yaml for all available options
# Note: Server port is configured in flow-config.yaml (runtime.port)
```

Or create a `.env` file:
```bash
# .env (create this file yourself)
PAYMENT_API_KEY=sk_test_your_key_here
```

3. **Build the application**:
```bash
# From the sflowg project root
./sflowg build docs/examples/payment-system-integration

# Or if sflowg is in your PATH
sflowg build docs/examples/payment-system-integration
```

This will:
- Parse `flow-config.yaml`
- Auto-detect plugin types (core, local)
- Analyze plugin dependencies
- Generate `main.go` with plugin initialization
- Generate `go.mod` with dependencies
- Build the binary
- Place `payment-system-app` in the project directory

4. **Run the application**:
```bash
./payment-system-app
```

The server starts on `http://localhost:8080` (configurable in `flow-config.yaml` under `runtime.port`).

## 🎭 Mock External Services (WireMock)

The flow makes HTTP requests to external services. Use WireMock to mock these services for testing:

**Quick Start with Docker Compose**:
```bash
# Start all mock services
docker-compose up -d inventory-mock payment-provider-mock notification-mock

# Run your app (pointing to mocks by default)
./payment-system-app
```

**Services mocked**:
- 🏪 **Inventory Service** (port 9001) - Stock checking
- 💳 **Payment Provider** (port 9002) - Payment processing
- 📧 **Notification Service** (port 9000) - Email notifications

**Features tested**:
- ✅ Successful responses
- 🔄 Retry logic (rate limiting, transient failures)
- ❌ Error scenarios (out of stock, insufficient funds)
- ⏱️ Timeout handling

**Test WireMock services**:
```bash
# After starting mocks, verify they work
./wiremock/test-mocks.sh
```

**Alternative: Standalone WireMock** (no Docker):
```bash
cd wiremock
./run-standalone.sh
```

See [wiremock/README.md](./wiremock/README.md) for complete documentation.

## 🎬 Complete End-to-End Workflow

**Terminal 1 - Start Mock Services**:
```bash
# Using Docker Compose
docker-compose up -d inventory-mock payment-provider-mock notification-mock

# OR using standalone script
cd wiremock && ./run-standalone.sh
```

**Terminal 2 - Build and Run Application**:
```bash
export PAYMENT_API_KEY=sk_test_key
./sflowg build docs/examples/payment-system-integration
cd docs/examples/payment-system-integration
./payment-system-app
```

**Terminal 3 - Run Tests**:
```bash
# Using Hurl
hurl --test test-requests.hurl --very-verbose

# OR using curl
curl -X POST http://localhost:8080/api/purchase/ORD-TEST-001?currency=USD&notify=true \
  -H "Authorization: Bearer sk_test_token_12345" \
  -H "X-Request-ID: REQ-001" \
  -H "X-Customer-ID: CUST-001" \
  -H "Content-Type: application/json" \
  -d '{
    "item": {"name": "Premium Widget", "price": 49.99, "quantity": 3},
    "customer": {"name": "John Doe", "email": "john@example.com"},
    "payment": {"card": {"number": "4532 1488 0343 6467", "expiry": "12/25", "cvv": "123"}}
  }'
```

**View WireMock Request Logs**:
```bash
# See what requests were received
curl http://localhost:9001/__admin/requests | jq
curl http://localhost:9002/__admin/requests | jq
curl http://localhost:9000/__admin/requests | jq
```

## 🧪 Testing

### Using the HTTP file

Open `test-requests.http` in VS Code with REST Client extension and run any test:

**Test 1: Successful Purchase**
- Amount: $149.97 (3 × $49.99)
- Free shipping applied (over $100)
- Tax calculated (15%)
- Valid card (Visa)
- Notification sent

**Test 2: Large Order with Discount**
- Amount: $499.99
- Discount applied (10% for orders over $200)
- Free shipping
- Tax calculated

**Test 3: Card Validation Failures**
- Invalid Luhn check
- Expired card
- Invalid CVV

**Test 4: Amount Validation**
- Below minimum ($5.00)
- Above maximum ($10,000.00)

See all 13 test scenarios in `test-requests.http`.

### Using Hurl (Recommended for CI/CD)

[Hurl](https://hurl.dev/) is a command-line tool for running and testing HTTP requests with assertions.

**Install Hurl**:
```bash
# macOS
brew install hurl

# Linux
curl -LO https://github.com/Orange-OpenSource/hurl/releases/latest/download/hurl_amd64.deb
sudo dpkg -i hurl_amd64.deb

# Or use the official installer
curl --proto '=https' --tlsv1.2 -sSf https://hurl.dev/install.sh | sh
```

**Run all tests**:
```bash
# Run all tests with summary
hurl --test test-requests.hurl

# Run with verbose output
hurl --test test-requests.hurl --very-verbose

# Generate HTML report
hurl --test --report-html report/ test-requests.hurl

# Run with variables
hurl --test --variable base_url=http://localhost:8080 test-requests.hurl
```

**Run specific test**:
```bash
# Run only the first test (successful purchase)
hurl --test test-requests.hurl --to-entry 1

# Run tests 1-5
hurl --test test-requests.hurl --to-entry 5
```

**Example output**:
```
test-requests.hurl: Running [1/13]
test-requests.hurl: Running [2/13]
test-requests.hurl: Running [3/13]
...
--------------------------------------------------------------------------------
Executed files:  1
Executed tests:  13
Succeeded:       13 (100.0%)
Failed:          0 (0.0%)
Duration:        2431 ms
```

**Features**:
- ✅ **Assertions**: Validates response status, headers, JSON fields
- ✅ **Test Reports**: Generate HTML/JSON reports for CI/CD
- ✅ **Variables**: Use environment variables and captured values
- ✅ **Chaining**: Chain requests and use previous responses
- ✅ **CI/CD Ready**: Perfect for automated testing pipelines

### Using curl

```bash
# Successful purchase
curl -X POST http://localhost:8080/api/purchase/ORD-12345?currency=USD&notify=true \
  -H "Authorization: Bearer sk_test_token_12345" \
  -H "X-Request-ID: REQ-001" \
  -H "X-Customer-ID: CUST-67890" \
  -H "Content-Type: application/json" \
  -d '{
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
  }'
```

### Expected Response

```json
{
  "success": true,
  "status": "success",
  "message": "Payment processed successfully",
  "data": {
    "orderId": "ORD-12345",
    "transactionId": "a1b2c3d4-e5f6-...",
    "status": "success",
    "customer": {
      "id": "CUST-67890",
      "name": "John Doe",
      "email": "john.doe@example.com"
    },
    "items": [...],
    "pricing": {
      "subtotal": 149.97,
      "shipping": 0.0,
      "tax": 22.50,
      "discount": 0.0,
      "total": 172.47,
      "currency": "USD"
    },
    "payment": {
      "cardType": "visa",
      "lastFourDigits": "6467",
      "authorizationCode": "AUTH_...",
      "processedAt": "2024-11-07T10:30:00Z"
    }
  }
}
```

## 📊 Flow Execution Path

```
1. Generate Transaction ID (uuidgen.generateUUID)
2. Extract Request Data (assign with variable resolution)
3. Calculate Subtotal (assign with arithmetic)
4. Determine Shipping (switch: free vs standard)
5. Calculate Tax (conditional assign)
6. Check for Discount (switch: apply vs no discount)
7. Calculate Total (complex arithmetic)
8. Validate Amount (switch: too low/high/valid)
9. Check Inventory (conditional HTTP request)
10. Validate Card (payment.validateCard)
11. Determine Payment Eligibility (switch)
12. Process Payment (payment.processPayment with retry)
13. Call Payment Provider API (HTTP POST with retry)
14. Determine Final Status (switch with multiple branches)
15-18. Status Assignment (parallel branches)
19. Send Notification (conditional HTTP request)
20. Get Plugin Statistics (payment.getStatus)
21. Build Response Summary (complex nested assignment)
22. Return HTTP Response
```

## 🎯 Feature Coverage

| Category | Features Tested | Status |
|----------|----------------|--------|
| Plugin System | 35+ features | ✅ Complete |
| CLI Build | 15+ features | ✅ Complete |
| HTTP Plugin | 15+ features | ✅ Complete |
| Flow Execution | 30+ features | ✅ Complete |
| Retry/Timeout | 10+ features | ✅ Complete |
| Conditionals | 10+ features | ✅ Complete |
| Assignments | 10+ features | ✅ Complete |
| Lifecycle | 10+ features | ✅ Complete |
| Container | 10+ features | ✅ Complete |
| Integration | 10+ features | ✅ Complete |

**Total: ~155 features tested**

See [TEST_FEATURES.md](./TEST_FEATURES.md) for the complete checklist.

## 🐛 Troubleshooting

### Build Failures

**Error: "plugin go.mod not found"**
- Solution: Ensure `./plugins/payment/go.mod` and `./plugins/uuidgen/go.mod` exist

**Error: "PAYMENT_API_KEY is required"**
- Solution: Export as environment variable: `export PAYMENT_API_KEY=sk_test_key`
- Or create `.env` file with the key

### Runtime Errors

**Error: "plugin initialization failed"**
- Check plugin configuration in `flow-config.yaml`
- Verify all required env vars are set
- Check logs for specific plugin error messages

**Error: "task not found: payment.validateCard"**
- Ensure plugin was properly registered during build
- Check generated `main.go` for plugin imports

### Flow Execution Issues

**Card validation always fails**
- Check card number passes Luhn algorithm
- Verify expiry date format (MM/YY) and not expired
- Ensure CVV is 3 digits

**Amount validation errors**
- Check min/max amount limits in config
- Verify subtotal + tax + shipping - discount calculation

## 📚 Documentation

- [PLUGIN_SYSTEM_DESIGN.md](../../PLUGIN_SYSTEM_DESIGN.md) - Complete plugin system design
- [TEST_FEATURES.md](./TEST_FEATURES.md) - Detailed feature test checklist
- [test-requests.http](./test-requests.http) - All test scenarios

## 🔄 Development Workflow

1. **Make changes** to plugins or flow
2. **Rebuild**: `sflowg build docs/examples/payment-system-integration`
3. **Restart** the application
4. **Test** using HTTP file or curl
5. **Check logs** for plugin lifecycle messages

## 🎓 Learning Path

If you're new to SFlowG, explore the example in this order:

1. **Start with flow-config.yaml** - Understand plugin configuration
2. **Read purchase_flow.yaml** - See flow structure and features
3. **Explore plugins/uuidgen** - Simple plugin example
4. **Study plugins/payment** - Complex plugin with all features
5. **Run test scenarios** - See everything in action
6. **Check TEST_FEATURES.md** - Understand what's being tested

## 🤝 Contributing

Found a bug or want to add a feature test? Please open an issue or PR!

## 📝 License

Apache 2.0 - See LICENSE file for details
