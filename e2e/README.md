# SFlowG External E2E

Black-box e2e tests for generated SFlowG applications.

The suite builds the local CLI, builds the fixture project, starts the generated
binary, then exercises it through HTTP, WireMock, Kafka, and Cron boundaries.
Tests do not import SFlowG packages.

## Requirements

- Go
- Docker and Docker Compose
- Bats
- Hurl
- curl
- jq

macOS:

```bash
brew install bats-core hurl jq
```

## Run

From this directory:

```bash
make test-smoke
make test-http
make test-plugin
make test-wiremock
make test-postgres
make test-packaging
make test-runtime
make test-observability
make test-negative
```

From the repo root:

```bash
make -C e2e test-smoke
```

`test` runs the fast default suite. Kafka and Cron are opt-in because they need
more external services and Cron has minute-granularity scheduling:

```bash
make test-transports
```

Run every external test:

```bash
make test-all
```

Set `SFLOWG_REPO=/path/to/sflowg` only when running the suite against another
checkout.

## Layout

```text
e2e/
  docker-compose.yml
  lib/
  tests/
  fixtures/kitchen-sink/
  wiremock/mappings/
```

See [TEST_MATRIX.md](./TEST_MATRIX.md) for target, fixture, and coverage
mapping.

## Diagnostics

On test failure the harness prints app logs, Docker Compose status, WireMock
requests, and unmatched WireMock requests.
