# Postgres Plugin

PostgreSQL database plugin for SFlowG.

## Installation

```yaml
# flow-config.yaml
plugins:
  - source: github.com/BDNK1/sflowg/plugins/postgres
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

The plugin currently supports positional PostgreSQL placeholders (`$1`, `$2`,
...) with `params` as an array. Keep request data and other user-controlled
values in `params`; do not interpolate them directly into SQL strings.

**Output:**

| Field | Type | Description |
|-------|------|-------------|
| `found` | bool | Whether a row was found |
| `row` | map | Column values (empty if not found) |

**Example:**

```sflowg
step get_payment as postgres.get {
    query: `
        SELECT id, amount, status
        FROM payments
        WHERE id = $1
    `
    params: [request.body.payment_id]
}

step process(condition: get_payment.found == true) {
    {
        amount: get_payment.row.amount,
        status: get_payment.row.status
    }
}

step not_found(condition: get_payment.found == false) {
    response.json({
        status: 404,
        body: { error: "payment not found" }
    })
}
```

### `postgres.exec`

Executes INSERT, UPDATE, or DELETE queries.

**Input:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `query` | string | ✅ | SQL query with $1, $2 placeholders |
| `params` | array | ❌ | Query parameters |

**Output:**

| Field | Type | Description |
|-------|------|-------------|
| `affected_rows` | int | Number of rows affected |

**Example:**

```sflowg
step insert_payment as postgres.exec {
    query: `
        INSERT INTO payments (id, amount, status, created_at)
        VALUES ($1, $2, $3, NOW())
    `
    params: [generate_id.id, request.body.amount, "pending"]
}

step update_status as postgres.exec {
    query: `
        UPDATE payments
        SET status = $1
        WHERE id = $2
    `
    params: ["completed", payment_id]
}
```

### Using RETURNING

PostgreSQL supports `RETURNING` clause to get values from INSERT/UPDATE:

```sflowg
step insert_with_returning as postgres.get {
    query: `
        INSERT INTO payments (amount)
        VALUES ($1)
        RETURNING id, created_at
    `
    params: [request.body.amount]
}

step use_id {
    { new_id: insert_with_returning.row.id }
}
```

## Named Parameter Convention

SFlowG recommends that query-like plugins support named placeholders with a
`params` map when the underlying client can do so safely:

```sflowg
step get_payment as customdb.get {
    query: `
        SELECT id, amount, status
        FROM payments
        WHERE id = :payment_id
    `
    params: {
        payment_id: request.body.payment_id
    }
}
```

This is a plugin convention, not language syntax. The postgres plugin does not
currently implement named placeholders; use `$1`, `$2`, ... with array params
unless this plugin's input contract changes.

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

```sflowg
step insert_payment(retry: { max_attempts: 3, delay: 100, backoff: "exponential" }) as postgres.exec {
    query: `
        INSERT INTO payments (...)
        VALUES (...)
    `
    params: [...]
}
```
