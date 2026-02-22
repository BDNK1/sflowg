# Ecommerce Orders API (DSL Engine)

A simple ecommerce orders workflow built with SFlowG's DSL engine, demonstrating `.flow` syntax with PostgreSQL.

## DSL vs YAML

This example uses `engine: dsl` in `flow-config.yaml`. Flows are written in `.flow` files using the DSL syntax instead of YAML. Compare with the `stripe-integration` example, which uses the default YAML engine.

**DSL syntax highlights:**
- Steps contain Risor code bodies instead of declarative args
- `response.json(...)` is called directly (no separate return type dispatch)
- Plugin calls use function syntax: `postgres.get({query: "...", params: [...]})`
- `pay_order` delegates payment creation to the `stripe-integration` flow via `http.request(...)`

## Quick Setup

1. Start PostgreSQL:

```bash
docker compose up -d
```

2. Configure env:

```bash
cp .env.example .env
# Set STRIPE_INTEGRATION_URL to your running stripe-integration app
```

3. Build and run:

```bash
# From the cli/ folder
go build -o ../bin/sflowg .
export PATH="$(pwd)/../bin:$PATH"

# From this example folder
sflowg build . \
  --runtime-path ../../../runtime \
  --core-plugins-path ../../../plugins
./ecommerce-api
```

Server: `http://localhost:8080`

`/api/orders/:id/pay` expects the stripe-integration example running separately
(for example on `http://localhost:8090`).

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/orders` | Create a new order |
| GET | `/api/orders/:id` | Get order details |
| POST | `/api/orders/:id/pay` | Create payment via stripe-integration flow |
| POST | `/api/orders/:id/cancel` | Cancel order |

## Test

```bash
# Create an order
curl -s -X POST http://localhost:8080/api/orders \
  -H "Content-Type: application/json" \
  -d '{"customer_email": "alice@example.com", "amount_cents": 14999, "currency": "usd"}'

# Get the order
curl -s http://localhost:8080/api/orders/1

# Create payment via stripe-integration for it
curl -s -X POST http://localhost:8080/api/orders/1/pay

# Cancel another order
curl -s -X POST http://localhost:8080/api/orders/2/cancel
```
