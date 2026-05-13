# E2E Test Matrix

This suite is black-box: it builds the local `sflowg` CLI, builds the fixture
project, starts the generated binary, and verifies behavior through external
boundaries.

## Targets

| Target        | Command                   | Coverage                                                 | Expected duration |
|---------------|---------------------------|----------------------------------------------------------|-------------------|
| Smoke         | `make test-smoke`         | CLI build, fixture build, primary HTTP kitchen-sink path | Fast              |
| HTTP          | `make test-http`          | HTTP happy path and schema validation errors             | Fast              |
| Plugin        | `make test-plugin`        | Local plugin detection, config injection, task execution | Fast              |
| WireMock      | `make test-wiremock`      | Outbound HTTP, retry, fallback, request counts           | Fast              |
| Transports    | `make test-transports`    | Kafka consumer and Cron scheduled flow                   | Slower            |
| Postgres      | `make test-postgres`      | Postgres plugin `get`/`exec` behavior                    | Medium            |
| Packaging     | `make test-packaging`     | Embedded flows, `FLOWS_PATH`, adjacent flow resolution   | Medium            |
| Runtime       | `make test-runtime`       | HTTP response variants, compensation, async, foreach     | Medium            |
| Observability | `make test-observability` | User logs and configured masking                         | Medium            |
| Negative      | `make test-negative`      | Kafka schema `on_error` and Cron error handler           | Slower            |
| All           | `make test-all`           | Full suite                                               | Slowest           |

## Test Cases

| Bats file                                | Test case                                                          | Fixture flows                                                    | Assertions                                                                                                                                                                   | External services            |
|------------------------------------------|--------------------------------------------------------------------|------------------------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|------------------------------|
| `tests/001_cli_build.bats`               | `sflowg CLI binary is built`                                       | none                                                             | `bin/sflowg` exists and is executable                                                                                                                                        | none                         |
| `tests/001_cli_build.bats`               | `sflowg build creates kitchen-sink app`                            | all `fixtures/kitchen-sink/flows/*.flow`                         | Generated `fixtures/kitchen-sink/kitchen-sink` binary exists and is executable                                                                                               | none                         |
| `tests/010_http_syntax.bats`             | `HTTP kitchen sink syntax works`                                   | `health.flow`, `http_kitchen_sink.flow`, `resolve_currency.flow` | `tests/hurl/http_syntax.hurl` verifies HTTP 200, path/query/header/body parsing, subflow result, local plugin result, parallel foreach collect order, and quoted item totals | WireMock, Redpanda, Postgres |
| `tests/010_http_syntax.bats`             | `WireMock received catalog calls`                                  | `http_kitchen_sink.flow`                                         | WireMock request journal contains `/catalog/sku_1` and `/catalog/sku_2`                                                                                                      | WireMock, Redpanda, Postgres |
| `tests/020_validation_errors.bats`       | `HTTP schema validation returns problem response`                  | `health.flow`, `http_validation.flow`                            | `tests/hurl/validation_errors.hurl` verifies HTTP 400 and `application/problem+json`                                                                                         | WireMock, Redpanda, Postgres |
| `tests/030_plugins.bats`                 | `local fixture plugin runs inside generated app`                   | `health.flow`, `plugin_flow.flow`                                | `tests/hurl/plugin.hurl` verifies `fixture.hello` output and config prefix                                                                                                   | WireMock, Redpanda, Postgres |
| `tests/040_retry_fallback_wiremock.bats` | `retry path recovers against WireMock scenario`                    | `health.flow`, `retry_fallback.flow`                             | `tests/hurl/retry_success.hurl` verifies recovery response; WireMock count verifies `/unstable` was called twice                                                             | WireMock, Redpanda, Postgres |
| `tests/040_retry_fallback_wiremock.bats` | `fallback path runs after retries are exhausted`                   | `health.flow`, `retry_fallback.flow`                             | `tests/hurl/fallback.hurl` verifies fallback response; WireMock verifies `/fallback` was called                                                                              | WireMock, Redpanda, Postgres |
| `tests/050_kafka_transport.bats`         | `Kafka transport consumes order event`                             | `health.flow`, `kafka_order.flow`                                | Produces JSON to `orders.events`; WireMock observes `/kafka/orders/ord_123`                                                                                                  | WireMock, Redpanda, Postgres |
| `tests/060_cron_transport.bats`          | `Cron transport fires scheduled flow`                              | `health.flow`, `cron_tick.flow`                                  | WireMock observes `/cron/tick` within 75 seconds                                                                                                                             | WireMock, Redpanda, Postgres |
| `tests/070_postgres_plugin.bats`         | `postgres plugin inserts, updates, and reads rows`                 | `health.flow`, `postgres_orders.flow`                            | `tests/hurl/postgres_order.hurl` verifies `postgres.get`, `postgres.exec`, `RETURNING`, and persisted row state                                                              | Postgres, WireMock, Redpanda |
| `tests/080_cli_packaging.bats`           | `embedded flow binary starts without external flows directory`     | all fixture flows                                                | Builds with `--embed-flows`, copies only the binary, starts without `--flows` or adjacent flow files, and verifies `/health`                                                 | WireMock, Redpanda, Postgres |
| `tests/080_cli_packaging.bats`           | `runtime flow resolution uses FLOWS_PATH without --flows`          | all fixture flows                                                | Copies the dev binary away from adjacent flows, sets `FLOWS_PATH`, starts without `--flows`, and verifies `/health`                                                          | WireMock, Redpanda, Postgres |
| `tests/080_cli_packaging.bats`           | `runtime flow resolution uses adjacent flows directory and --port` | all fixture flows                                                | Starts dev binary without `--flows`, relies on `flows/` next to the binary, and verifies custom `--port`                                                                     | WireMock, Redpanda, Postgres |
| `tests/090_http_responses_errors.bats`   | `HTTP text response includes status body and header`               | `health.flow`, `http_responses.flow`                             | `tests/hurl/response_text.hurl` verifies `response.text` status, body, and header                                                                                            | WireMock, Redpanda, Postgres |
| `tests/090_http_responses_errors.bats`   | `HTTP redirect response includes location`                         | `health.flow`, `http_redirect.flow`                              | `tests/hurl/response_redirect.hurl` verifies `response.redirect` status and `Location`                                                                                       | WireMock, Redpanda, Postgres |
| `tests/090_http_responses_errors.bats`   | `flow on_error returns response and compensation side effect runs` | `health.flow`, `error_compensation.flow`                         | `tests/hurl/error_compensation.hurl` verifies handled error response; WireMock verifies compensation side effect and body                                                    | WireMock, Redpanda, Postgres |
| `tests/100_async_foreach.bats`           | `async steps and sequential foreach variants work`                 | `health.flow`, `async_foreach.flow`                              | `tests/hurl/async_foreach.hurl` verifies top-level async, async parallel branch, sequential foreach, batch foreach, and no-collect foreach startup behavior                  | WireMock, Redpanda, Postgres |
| `tests/110_observability.bats`           | `observability logs user source and masks configured fields`       | `health.flow`, `observability.flow`                              | `tests/hurl/observability.hurl` verifies endpoint success; app log verifies `source=user` and masked nested `authorization`/`password` fields                                | WireMock, Redpanda, Postgres |
| `tests/120_kafka_cron_negative.bats`     | `Kafka schema violation runs on_error and acks message`            | `health.flow`, `kafka_invalid.flow`                              | Produces invalid JSON shape to `orders.invalid`; WireMock verifies `on_error` side effect and no normal side effect                                                          | WireMock, Redpanda, Postgres |
| `tests/120_kafka_cron_negative.bats`     | `Cron on_error path runs after scheduled failure`                  | `health.flow`, `cron_error.flow`                                 | WireMock observes `/cron/error-handled` with `CRON_FORCED_ERROR` within 75 seconds                                                                                           | WireMock, Redpanda, Postgres |

## Fixture Coverage

| Feature                   | Covered by                                                                                                        | Notes                                                                           |
|---------------------------|-------------------------------------------------------------------------------------------------------------------|---------------------------------------------------------------------------------|
| CLI build                 | `001_cli_build.bats`                                                                                              | Builds `cli` into `e2e/bin/sflowg`.                                             |
| Generated app build       | `001_cli_build.bats` and every setup file                                                                         | Uses local `core`, `plugins`, and `transports` paths.                           |
| HTTP route registration   | `010_http_syntax.bats`, `020_validation_errors.bats`, `030_plugins.bats`, `040_retry_fallback_wiremock.bats`      | Every generated app also exposes `GET /health`.                                 |
| HTTP input parsing        | `010_http_syntax.bats`                                                                                            | Path variables, query parameters, headers, and JSON body schema.                |
| HTTP validation errors    | `020_validation_errors.bats`                                                                                      | Verifies boundary validation before normal flow execution.                      |
| Subflow call              | `010_http_syntax.bats`                                                                                            | `http_kitchen_sink.flow` calls `resolve_currency.flow`.                         |
| Parallel block            | `010_http_syntax.bats`                                                                                            | `request_summary` and `plugin_message` run in a `parallel` block.               |
| Parallel foreach          | `010_http_syntax.bats`                                                                                            | Catalog lookups run through `parallel(...) foreach` and collect ordered arrays. |
| Local plugin              | `030_plugins.bats` and `010_http_syntax.bats`                                                                     | `fixtures/kitchen-sink/plugins/fixture` exposes `fixture.hello`.                |
| Plugin config/env default | `030_plugins.bats`                                                                                                | `FIXTURE_PREFIX` defaults to `e2e` through generated config.                    |
| Outbound HTTP             | `010_http_syntax.bats`, `040_retry_fallback_wiremock.bats`, `050_kafka_transport.bats`, `060_cron_transport.bats` | All external calls target WireMock.                                             |
| Retry                     | `040_retry_fallback_wiremock.bats`                                                                                | WireMock scenario returns 500 once, then 200.                                   |
| Fallback                  | `040_retry_fallback_wiremock.bats`                                                                                | Forced transient failure exhausts retries and runs fallback body.               |
| Kafka entrypoint          | `050_kafka_transport.bats`                                                                                        | Uses Redpanda and `orders.events`.                                              |
| Cron entrypoint           | `060_cron_transport.bats`                                                                                         | Uses minute-granularity schedule, so the assertion waits up to 75 seconds.      |
| Postgres plugin           | `070_postgres_plugin.bats`                                                                                        | Exercises real Postgres over Docker Compose.                                    |
| Embedded flow build mode  | `080_cli_packaging.bats`                                                                                          | Starts a copied embedded binary without external flow files.                    |
| Runtime flow resolution   | `080_cli_packaging.bats`                                                                                          | Covers `FLOWS_PATH`, adjacent `flows/`, and generated `--port`.                 |
| HTTP text/redirect        | `090_http_responses_errors.bats`                                                                                  | Covers non-JSON HTTP response contracts.                                        |
| Compensation and on_error | `090_http_responses_errors.bats`                                                                                  | Verifies user-visible error response and compensation side effect.              |
| Top-level async           | `100_async_foreach.bats`                                                                                          | Response reads async step result.                                               |
| Sequential foreach        | `100_async_foreach.bats`                                                                                          | Covers collected sequential foreach results.                                    |
| Batch foreach             | `100_async_foreach.bats`                                                                                          | Covers batch source slicing and ordered collection.                             |
| Observability logging     | `110_observability.bats`                                                                                          | Verifies user log source and nested field masking.                              |
| Kafka negative path       | `120_kafka_cron_negative.bats`                                                                                    | Schema violation runs Kafka `on_error` and avoids normal step side effects.     |
| Cron error path           | `120_kafka_cron_negative.bats`                                                                                    | Failing cron flow runs `on_error` without a response.                           |

## Infrastructure

| Service        | Compose service | Host port | Used by                                                                      |
|----------------|-----------------|-----------|------------------------------------------------------------------------------|
| WireMock       | `wiremock`      | `8089`    | HTTP fixtures, retry/fallback scenarios, Kafka/Cron side-effect verification |
| Redpanda       | `redpanda`      | `29092`   | Kafka transport tests and generated app startup                              |
| Redpanda admin | `redpanda`      | `19644`   | Debugging only                                                               |
| Postgres       | `postgres`      | `54329`   | Postgres plugin e2e and generated app startup                                |

## Known Gaps

| Gap                             | Reason                                                                            |
|---------------------------------|-----------------------------------------------------------------------------------|
| Remote plugin source            | Current suite covers core and local plugins only.                                 |
| OTLP export                     | Current observability e2e asserts stdout JSON logs, not collector export.         |
| Kafka key/header namespaces     | Current Kafka tests focus on value schema and side effects.                       |
| Cron timezone/overlap semantics | Current Cron tests cover success and error ticks only.                            |
| Parallel/foreach failure modes  | Current e2e covers happy paths; detailed failure aggregation stays in unit tests. |
