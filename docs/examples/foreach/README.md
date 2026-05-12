# Foreach Example

This small DSL project demonstrates `foreach` without external plugins:

- `normalize_items.flow` uses sequential `foreach` and publishes two collect
  arrays.
- `quote_items_parallel.flow` uses `parallel(...) foreach` with bounded
  concurrency. Its collect arrays preserve the original input order.

## Run

From this directory:

```bash
go run ../../../cli build . \
  --core-path ../../../core \
  --transports-path ../../../transports \
  --embed-flows

./foreach-example
```

Server: `http://localhost:18081`

## Try It

Sequential foreach:

```bash
curl -s -X POST http://localhost:18081/foreach/normalize \
  -H "Content-Type: application/json" \
  -d '{
    "items": [
      {"sku": "A-100", "quantity": 2, "unit_price_cents": 500},
      {"sku": "B-200", "quantity": 1, "unit_price_cents": 1250}
    ]
  }'
```

Parallel foreach:

```bash
curl -s -X POST http://localhost:18081/foreach/quotes \
  -H "Content-Type: application/json" \
  -d '{
    "items": [
      {"position": 1, "sku": "slow-looking", "quantity": 3, "unit_price_cents": 400},
      {"position": 2, "sku": "fast-looking", "quantity": 1, "unit_price_cents": 999},
      {"position": 3, "sku": "last", "quantity": 5, "unit_price_cents": 125}
    ]
  }'
```

The response `quotes` and `quote_totals` arrays follow the request item order,
even though iterations are scheduled concurrently.
