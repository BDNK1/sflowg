# Postgres Plugin

PostgreSQL database plugin for SFlowG.

## Installation

```yaml
# flow-config.yaml
plugins:
  - source: github.com/sflowg/sflowg/plugins/postgres
    config:
      connection_string: ${DATABASE_URL}
      max_open_conns: 10
      max_idle_conns: 5
      conn_max_lifetime_ms: 300000
```

## Configuration

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `connection_string` | string | required | PostgreSQL connection string |
| `max_open_conns` | int | `10` | Maximum open connections |
| `max_idle_conns` | int | `5` | Maximum idle connections |
| `conn_max_lifetime_ms` | int | `300000` | Connection max lifetime (5 min) |

**Connection string format:**
```
postgres://user:password@host:5432/database?sslmode=disable
```

## Tasks

### `postgres.get`

Executes a SELECT query and returns a single row.

**Input:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `query` | string | ✅ | SQL SELECT query with $1, $2 placeholders |
| `params` | array | ❌ | Query parameters |

**Output:**

| Field | Type | Description |
|-------|------|-------------|
| `found` | bool | Whether a row was found |
| `row` | map | Column values (empty if not found) |

**Example:**

```yaml
- id: get_payment
  type: postgres.get
  args:
    query: '"SELECT id, amount, status FROM payments WHERE id = $1"'
    params:
      - request.body.payment_id

- id: check_found
  type: switch
  args:
    not_found: get_payment.result.found == false
    process: get_payment.result.found == true

- id: process
  type: assign
  args:
    amount: get_payment.result.row.amount
    status: get_payment.result.row.status
```

### `postgres.exec`

Executes INSERT, UPDATE, or DELETE queries.

**Input:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `query` | string | ✅ | SQL query with $1, $2 placeholders |
| `params` | array | Query parameters |

**Output:**

| Field | Type | Description |
|-------|------|-------------|
| `affected_rows` | int | Number of rows affected |

**Example:**

```yaml
- id: insert_payment
  type: postgres.exec
  args:
    query: '"INSERT INTO payments (id, amount, status, created_at) VALUES ($1, $2, $3, NOW())"'
    params:
      - generate_id.result.id
      - request.body.amount
      - '"pending"'

- id: update_status
  type: postgres.exec
  args:
    query: '"UPDATE payments SET status = $1 WHERE id = $2"'
    params:
      - '"completed"'
      - payment_id
```

### Using RETURNING

PostgreSQL supports `RETURNING` clause to get values from INSERT/UPDATE:

```yaml
- id: insert_with_returning
  type: postgres.get
  args:
    query: '"INSERT INTO payments (amount) VALUES ($1) RETURNING id, created_at"'
    params:
      - request.body.amount

- id: use_id
  type: assign
  args:
    new_id: insert_with_returning.result.row.id
```

## Type Handling

The plugin handles PostgreSQL-specific types:

| PostgreSQL Type | Returned As |
|-----------------|-------------|
| `JSONB`, `JSON` | string |
| `UUID` | string |
| `NUMERIC`, `DECIMAL` | string |
| `INTEGER`, `BIGINT` | int64 |
| `TEXT`, `VARCHAR` | string |
| `BOOLEAN` | bool |
| `TIMESTAMP` | time.Time |

## Error Handling

Errors include context for debugging:

```
postgres.get: query failed: pq: relation "users" does not exist
postgres.exec: query failed: pq: duplicate key value violates unique constraint
```

Use retry configuration for transient errors:

```yaml
- id: insert_payment
  type: postgres.exec
  args:
    query: '"INSERT INTO payments ..."'
  retry:
    max_retries: 3
    delay: 100
    backoff: true
```