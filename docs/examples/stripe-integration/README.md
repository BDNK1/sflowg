# Stripe Integration Example

Minimal Stripe + PostgreSQL integration built with SFlowG.

## Quick Setup

1. Start services:

```bash
docker compose up -d
```

2. Configure env:

```bash
cp .env.example .env
```

Required values:
- `STRIPE_SECRET_KEY`
- `STRIPE_WEBHOOK_SECRET`
- `STRIPE_PUBLISHABLE_KEY`

Stripe setup:
- In Stripe Dashboard, enable **Test mode**
- Copy API keys from **Developers -> API keys** (`sk_test_...`, `pk_test_...`)
- Get webhook secret (`whsec_...`) from `stripe listen --forward-to localhost:8090/api/webhooks/stripe`

3. Build and run:

```bash
# in cli folder
go build -o ../bin/sflowg .
export PATH="$(pwd)/../bin:$PATH"

# in stripe-integration folder
sflowg build . \
  --runtime-path ../../../runtime \
  --core-plugins-path ../../../plugins
./stripe-integration
```

Server: `http://localhost:8090`

## Minimal Test

```bash
curl -X POST http://localhost:8090/api/payments \
  -H "Content-Type: application/json" \
  -H "X-Request-ID: test-001" \
  -d '{
    "amount": 2000,
    "currency": "usd",
    "description": "Test payment",
    "customer_email": "test@example.com",
    "customer_name": "Test User",
    "metadata": {
      "order_id": "ORD-12345"
    }
  }'
```

Expected: `201` with `order_id`, `client_secret`, and `checkout_url`.

## Endpoints

- `POST /api/payments` creates PaymentIntent + DB record
- `POST /api/webhooks/stripe` verifies signature, updates payment status, and calls ecom internal callback (`ECOM_CALLBACK_URL`) to sync order status
- `GET /checkout/order/:order_id` renders checkout or final result page
