export SFLOWG_REPO ?=
BATS ?= bats

.PHONY: test
test: test-smoke test-http test-plugin test-wiremock

.PHONY: test-smoke
test-smoke:
	$(BATS) tests/001_cli_build.bats tests/010_http_syntax.bats

.PHONY: test-http
test-http:
	$(BATS) tests/010_http_syntax.bats tests/020_validation_errors.bats

.PHONY: test-plugin
test-plugin:
	$(BATS) tests/030_plugins.bats

.PHONY: test-wiremock
test-wiremock:
	$(BATS) tests/040_retry_fallback_wiremock.bats

.PHONY: test-transports
test-transports:
	$(BATS) tests/050_kafka_transport.bats tests/060_cron_transport.bats

.PHONY: test-postgres
test-postgres:
	$(BATS) tests/070_postgres_plugin.bats

.PHONY: test-packaging
test-packaging:
	$(BATS) tests/080_cli_packaging.bats

.PHONY: test-runtime
test-runtime:
	$(BATS) tests/090_http_responses_errors.bats tests/100_async_foreach.bats

.PHONY: test-observability
test-observability:
	$(BATS) tests/110_observability.bats

.PHONY: test-negative
test-negative:
	$(BATS) tests/120_kafka_cron_negative.bats

.PHONY: test-all
test-all:
	$(BATS) tests

.PHONY: compose-up
compose-up:
	docker compose -f docker-compose.yml up -d

.PHONY: compose-down
compose-down:
	docker compose -f docker-compose.yml down -v --remove-orphans

.PHONY: clean
clean:
	docker compose -f docker-compose.yml down -v --remove-orphans
	rm -rf bin .app-*.log fixtures/kitchen-sink/kitchen-sink
