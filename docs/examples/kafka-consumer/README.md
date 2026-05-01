# Kafka Consumer Example

This example consumes JSON messages from Kafka-compatible Redpanda with
`entrypoint.kafka`, validates `message.value`, logs the order, and returns
`response.ack()`.

## Run

From this directory:

```bash
docker compose up -d
docker compose exec redpanda rpk topic create orders.events

go run ../../../cli build . \
  --core-path ../../../core \
  --transports-path ../../../transports \
  --embed-flows

./kafka-consumer-example
```

In another terminal:

```bash
printf '%s\n' '{"order_id":"ord_1001","customer_email":"alice@example.com","amount_cents":14999}' |
  docker compose exec -T redpanda rpk topic produce orders.events
```

The app logs `kafka order received` and acknowledges the message. The
`currency` field is omitted in the produced message so the schema default
normalizes it to `usd`.

## Cleanup

```bash
docker compose down -v
```
