package postgres

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
	"github.com/sflowg/sflowg/runtime/plugin"
)

// Config holds the Postgres plugin configuration
type Config struct {
	ConnectionString  string `yaml:"connection_string" validate:"required"`
	MaxOpenConns      int    `yaml:"max_open_conns" default:"10" validate:"gte=1,lte=100"`
	MaxIdleConns      int    `yaml:"max_idle_conns" default:"5" validate:"gte=0,lte=50"`
	ConnMaxLifetimeMs int    `yaml:"conn_max_lifetime_ms" default:"300000" validate:"gte=0"` // 5 min default
}

// GetInput defines input for postgres.get task
type GetInput struct {
	Query  string `json:"query" validate:"required"`
	Params []any  `json:"params"`
}

// GetOutput defines output for postgres.get task
type GetOutput struct {
	Row   map[string]any `json:"row"`
	Found bool           `json:"found"`
}

// ExecInput defines input for postgres.exec task
type ExecInput struct {
	Query  string `json:"query" validate:"required"`
	Params []any  `json:"params"`
}

// ExecOutput defines output for postgres.exec task
type ExecOutput struct {
	AffectedRows int64 `json:"affected_rows"`
}

// PostgresPlugin provides PostgreSQL database operations
type PostgresPlugin struct {
	Config Config
	db     *sql.DB
}

// Initialize opens the database connection pool
func (p *PostgresPlugin) Initialize(exec *plugin.Execution) error {
	// Debug: log connection string (mask password)
	fmt.Printf("[postgres] DEBUG: Initializing with connection_string: %s\n", maskConnectionString(p.Config.ConnectionString))
	fmt.Printf("[postgres] DEBUG: MaxOpenConns=%d, MaxIdleConns=%d, ConnMaxLifetimeMs=%d\n",
		p.Config.MaxOpenConns, p.Config.MaxIdleConns, p.Config.ConnMaxLifetimeMs)

	db, err := sql.Open("postgres", p.Config.ConnectionString)
	if err != nil {
		return fmt.Errorf("postgres: failed to open connection: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(p.Config.MaxOpenConns)
	db.SetMaxIdleConns(p.Config.MaxIdleConns)
	db.SetConnMaxLifetime(time.Duration(p.Config.ConnMaxLifetimeMs) * time.Millisecond)

	// Verify connection
	if err := db.Ping(); err != nil {
		db.Close()
		return fmt.Errorf("postgres: failed to ping database: %w", err)
	}

	p.db = db
	return nil
}

// Shutdown closes the database connection pool
func (p *PostgresPlugin) Shutdown(exec *plugin.Execution) error {
	if p.db != nil {
		return p.db.Close()
	}
	return nil
}

// Get executes a SELECT query and returns a single row
func (p *PostgresPlugin) Get(exec *plugin.Execution, input GetInput) (GetOutput, error) {
	// DEBUG: Log query and params
	fmt.Printf("[postgres.get] DEBUG Query: [%s]\n", input.Query)
	fmt.Printf("[postgres.get] DEBUG Params: %+v\n", input.Params)

	rows, err := p.db.Query(input.Query, input.Params...)
	if err != nil {
		return GetOutput{}, fmt.Errorf("postgres.get: query failed: %w", err)
	}
	defer rows.Close()

	// Get column info
	cols, err := rows.Columns()
	if err != nil {
		return GetOutput{}, fmt.Errorf("postgres.get: failed to get columns: %w", err)
	}

	colTypes, err := rows.ColumnTypes()
	if err != nil {
		return GetOutput{}, fmt.Errorf("postgres.get: failed to get column types: %w", err)
	}

	// Check if we have a row
	if !rows.Next() {
		return GetOutput{Found: false, Row: map[string]any{}}, nil
	}

	// Scan the row
	row, err := scanRow(cols, colTypes, rows)
	if err != nil {
		return GetOutput{}, fmt.Errorf("postgres.get: failed to scan row: %w", err)
	}

	return GetOutput{Found: true, Row: row}, nil
}

// Exec executes INSERT, UPDATE, or DELETE query
func (p *PostgresPlugin) Exec(exec *plugin.Execution, input ExecInput) (ExecOutput, error) {
	result, err := p.db.Exec(input.Query, input.Params...)
	if err != nil {
		return ExecOutput{}, fmt.Errorf("postgres.exec: query failed: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return ExecOutput{}, fmt.Errorf("postgres.exec: failed to get affected rows: %w", err)
	}

	return ExecOutput{AffectedRows: affected}, nil
}

// scanRow scans a single row into a map, handling postgres-specific types
func scanRow(cols []string, colTypes []*sql.ColumnType, rows *sql.Rows) (map[string]any, error) {
	// Create slice for scanning
	values := make([]any, len(cols))
	valuePtrs := make([]any, len(cols))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	if err := rows.Scan(valuePtrs...); err != nil {
		return nil, err
	}

	// Build result map with type handling
	result := make(map[string]any)
	for i, col := range cols {
		val := values[i]

		// Handle postgres-specific types
		switch colTypes[i].DatabaseTypeName() {
		case "JSONB", "JSON", "UUID", "NUMERIC", "DECIMAL":
			// Convert []byte to string for these types
			if b, ok := val.([]byte); ok {
				result[col] = string(b)
			} else {
				result[col] = val
			}
		default:
			result[col] = val
		}
	}

	return result, nil
}

// maskConnectionString masks the password in a postgres connection string for logging
func maskConnectionString(connStr string) string {
	// Simple masking: replace password between :// and @
	// Format: postgres://user:password@host:port/db
	schemeEnd := "://"

	start := 0
	for i := 0; i < len(connStr)-len(schemeEnd); i++ {
		if connStr[i:i+len(schemeEnd)] == schemeEnd {
			start = i + len(schemeEnd)
			break
		}
	}

	// Find colon after user
	colonPos := -1
	for i := start; i < len(connStr); i++ {
		if connStr[i] == ':' {
			colonPos = i
			break
		}
	}

	// Find @
	atPos := -1
	for i := start; i < len(connStr); i++ {
		if connStr[i] == '@' {
			atPos = i
			break
		}
	}

	if colonPos > 0 && atPos > colonPos {
		return connStr[:colonPos+1] + "***" + connStr[atPos:]
	}
	return connStr
}
