# Stripe Integration Example

A complete Stripe payment integration using SFlowG with PostgreSQL for payment storage.

## Overview

This example demonstrates:
- Creating PaymentIntents via Stripe API (form-encoded requests)
- Storing payment records in PostgreSQL
- Processing Stripe webhooks to update payment status
- Webhook signature verification
- Using base64 encoding for API authentication
- Loading secrets from .env file

## Architecture

```
stripe-integration/
├── flow-config.yaml     # Plugin configuration
├── schema.sql           # PostgreSQL schema
├── .env.example         # Environment variables template
├── flows/
│   ├── create_payment.yaml    # POST /api/payments
│   └── process_webhook.yaml   # POST /api/webhooks/stripe
├── plugins/
│   └── stripe-signature-verification/  # Webhook signature verification
│       ├── go.mod
│       └── plugin.go
└── README.md
```

## Payment Flow

```
┌─────────┐      ┌─────────┐      ┌──────────┐      ┌────────┐
│ Client  │──1──▶│ SFlowG  │──2──▶│ Postgres │      │ Stripe │
│         │      │         │──3──▶│          │      │        │
│         │◀─6───│         │◀─5───│          │◀─4───│        │
└─────────┘      └─────────┘      └──────────┘      └────────┘

1. Client sends payment request
2. SFlowG stores payment (status: pending)
3. SFlowG creates PaymentIntent with Stripe
4. Stripe returns payment_intent_id + client_secret
5. SFlowG updates payment with Stripe data
6. SFlowG returns client_secret to client
```

## Webhook Flow

```
┌────────┐      ┌─────────┐      ┌──────────┐
│ Stripe │──1──▶│ SFlowG  │──2──▶│ Postgres │
│        │◀─3───│         │      │          │
└────────┘      └─────────┘      └──────────┘

1. Stripe sends webhook event (payment_intent.succeeded, etc.)
2. SFlowG updates payment status in database
3. SFlowG returns 200 OK to acknowledge
```

## Prerequisites

1. Go 1.23+
2. SFlowG CLI installed
3. PostgreSQL database
4. Stripe account (test mode)

## Setup

### 1. Database Setup

Start PostgreSQL (using Docker):

```bash
docker run -d \
  --name stripe-postgres \
  -e POSTGRES_PASSWORD=postgres \
  -e POSTGRES_DB=stripe_payments \
  -p 5432:5432 \
  postgres:16
```

Create the schema:

```bash
psql postgres://postgres:postgres@localhost:5432/stripe_payments -f schema.sql
```

Or connect and run manually:

```bash
psql postgres://postgres:postgres@localhost:5432/stripe_payments
\i schema.sql
```

### 2. Stripe Setup

1. Go to [Stripe Dashboard](https://dashboard.stripe.com)
2. Enable **Test Mode** (toggle in top-right)
3. Get your **Secret Key** from Developers → API keys (`sk_test_...`)

### 3. Environment Variables

**Option A: Use .env file (recommended for development)**

The application automatically loads `.env` from the same directory as the binary.

Copy the example and fill in your keys:

```bash
cp .env.example .env
# Edit .env with your actual Stripe keys
```

**Option B: Export environment variables**

```bash
export STRIPE_SECRET_KEY=sk_test_your_secret_key_here
export STRIPE_WEBHOOK_SECRET=whsec_your_webhook_secret_here
export DATABASE_URL=postgres://postgres:postgres@localhost:5432/stripe_payments?sslmode=disable

# Optional
export PORT=8080
```

Note: Environment variables take precedence over .env file values.

### 4. Build and Run

```bash
# From sflowg root directory
./sflowg build docs/examples/stripe-integration

# Run the application
cd docs/examples/stripe-integration
./stripe-integration
```

Server starts at `http://localhost:8080`

## API Reference

### Create Payment

**POST /api/payments**

Creates a new payment and returns a client_secret for frontend use.

**Request:**

```json
{
  "amount": 2000,
  "currency": "usd",
  "description": "Order #12345",
  "customer_email": "customer@example.com",
  "customer_name": "John Doe",
  "metadata": {
    "order_id": "ORD-12345"
  }
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `amount` | int | Yes | Amount in cents (min 50) |
| `currency` | string | No | 3-letter currency code (default: usd) |
| `description` | string | No | Payment description |
| `customer_email` | string | No | Customer email for receipt |
| `customer_name` | string | No | Customer name |
| `metadata.order_id` | string | No | Your order reference |

**Response (201):**

```json
{
  "success": true,
  "data": {
    "payment_id": 1,
    "client_secret": "pi_xxx_secret_yyy",
    "payment_intent_id": "pi_xxx",
    "status": "requires_payment_method",
    "amount": 2000,
    "currency": "usd"
  }
}
```

**Error Response (400):**

```json
{
  "success": false,
  "error": {
    "code": "invalid_amount",
    "message": "Amount must be at least 50 cents"
  }
}
```

### Process Webhook

**POST /api/webhooks/stripe**

Receives and processes Stripe webhook events.

**Handled Events:**
- `payment_intent.succeeded` - Payment completed successfully
- `payment_intent.payment_failed` - Payment failed
- `payment_intent.canceled` - Payment was canceled

**Response (200):**

```json
{
  "received": true,
  "event_id": "evt_xxx",
  "event_type": "payment_intent.succeeded",
  "payment_intent_id": "pi_xxx",
  "status": "succeeded",
  "rows_updated": 1
}
```

## Testing

### Test with curl

**Create a payment:**

```bash
curl -X POST http://localhost:8080/api/payments \
  -H "Content-Type: application/json" \
  -H "X-Request-ID: test-001" \
  -d '{
    "amount": 2000,
    "currency": "usd",
    "description": "Test payment",
    "customer_email": "test@example.com"
  }'
```

### Test Webhooks with Stripe CLI

Install and configure [Stripe CLI](https://stripe.com/docs/stripe-cli):

```bash
# Install (macOS)
brew install stripe/stripe-cli/stripe

# Login to your Stripe account
stripe login

# Forward webhooks to your local server
stripe listen --forward-to localhost:8080/api/webhooks/stripe
```

The CLI will output a webhook signing secret (`whsec_...`). Set it as an environment variable for production use.

**Trigger test events:**

```bash
# Trigger a successful payment
stripe trigger payment_intent.succeeded

# Trigger a failed payment
stripe trigger payment_intent.payment_failed
```

### Check Database

```bash
psql postgres://postgres:postgres@localhost:5432/stripe_payments

# View all payments
SELECT id, amount, status, stripe_status, stripe_payment_intent_id, created_at
FROM payments ORDER BY created_at DESC;

# View payment details
SELECT * FROM payments WHERE id = 1;
```

## Flow Details

### create_payment.yaml

1. **Extract request** - Parse amount, currency, customer info
2. **Validate amount** - Ensure amount >= 50 cents
3. **Insert payment** - Store in DB with status 'pending'
4. **Create PaymentIntent** - Call Stripe API (form-encoded)
5. **Handle response** - Update DB with Stripe data or error
6. **Return response** - client_secret for frontend

**Key Features Used:**
- `http.request` with `content_type: form` for form-encoded body
- `base64_encode()` for Basic auth header
- `postgres.get` with RETURNING for INSERT
- `postgres.exec` for UPDATE
- `switch` for conditional branching

### process_webhook.yaml

1. **Extract event** - Parse event type and payment intent data
2. **Route by event type** - Switch to appropriate handler
3. **Update database** - Set status based on event
4. **Return 200** - Acknowledge receipt to Stripe

**Key Features Used:**
- `request.rawBody` available for signature verification
- `switch` for event type routing
- `postgres.exec` for UPDATE

## Production Considerations

### Webhook Signature Verification

Webhook signature verification is implemented using the `stripe.verify_signature` task from the local plugin. The webhook flow verifies signatures before processing events:

```yaml
- id: verify_signature
  type: stripe.verify_signature
  args:
    signature: request.headers.Stripe-Signature
    payload: request.rawBody
```

Invalid signatures return 400 and are rejected.

### Idempotency

Stripe webhooks may be sent multiple times. The flows handle this by:
- Using `stripe_payment_intent_id` as unique identifier
- UPDATE operations are idempotent (same status update is safe)

### Error Handling

- Payment creation errors are stored in the database
- Webhook errors still return 200 to prevent Stripe retries
- Use logging/monitoring for production debugging

## Database Schema

```sql
CREATE TABLE payments (
    id SERIAL PRIMARY KEY,
    amount INTEGER NOT NULL,
    currency VARCHAR(3) NOT NULL DEFAULT 'usd',
    description TEXT,
    customer_email VARCHAR(255),
    customer_name VARCHAR(255),
    metadata_order_id VARCHAR(255),
    stripe_payment_intent_id VARCHAR(255) UNIQUE,
    stripe_client_secret TEXT,
    stripe_status VARCHAR(50),
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    error_message TEXT,
    error_code VARCHAR(100),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE
);
```

**Status Values:**
- `pending` - Initial state, before Stripe API call
- `requires_payment_method` - PaymentIntent created, awaiting payment
- `succeeded` - Payment completed
- `failed` - Payment failed
- `canceled` - Payment canceled