# Plugin Configuration - Final Design

## Overview

Plugin configuration uses a **tag-based declarative approach** with framework-provided utilities:

1. **Plugin developers** define Config struct with tags (`yaml`, `default`, `validate`)
2. **Framework provides** all configuration utilities (defaults, validation, env overrides)
3. **Users specify** env var mappings in `flow-config.yaml`
4. **CLI generates** initialization code using framework functions

**Key benefit**: Plugin developers write zero boilerplate - just struct tags.

**Dependencies:**
- `github.com/creasty/defaults` - Apply default values from tags
- `github.com/go-playground/validator/v10` - Validate using constraint tags

---

## Plugin Side: Just Tags

Plugin developers only define the Config struct with tags:

```go
// plugins/redis/config.go
package redis

import "time"

// Config defines redis plugin configuration.
// Plugin developer provides ONLY the struct with tags - no functions needed.
type Config struct {
    Addr     string        `yaml:"addr" default:"localhost:6379" validate:"required,hostname_port"`
    Password string        `yaml:"password"`  // Optional, no validation
    DB       int           `yaml:"db" default:"0" validate:"gte=0,lte=15"`
    PoolSize int           `yaml:"pool_size" default:"10" validate:"gte=1,lte=1000"`
    Timeout  time.Duration `yaml:"timeout" default:"30s" validate:"gte=1s,lte=1h"`
}

// That's it! No DefaultConfig(), no Validate(), no init() needed.
// Framework handles everything.
```

**Tag meanings:**
- `yaml:"..."` - YAML field name for config file
- `default:"..."` - Default value (applied by framework)
- `validate:"..."` - Validation rules (checked by framework)

**Plugin uses validated config:**

```go
// plugins/redis/plugin.go
package redis

import (
    "context"
    "github.com/redis/go-redis/v9"
)

type Plugin struct {
    config Config
    client *redis.Client
}

func NewPlugin(config Config) *Plugin {
    return &Plugin{config: config}
}

func (p *Plugin) Initialize(ctx context.Context) error {
    // Config already validated by framework - just use it
    p.client = redis.NewClient(&redis.Options{
        Addr:     p.config.Addr,
        Password: p.config.Password,
        DB:       p.config.DB,
        PoolSize: p.config.PoolSize,
    })

    // Test connection
    return p.client.Ping(ctx).Err()
}

func (p *Plugin) Shutdown(ctx context.Context) error {
    return p.client.Close()
}
```

---

## Framework Side: Utilities

Framework provides all configuration utilities in `runtime/config.go`:

```go
// runtime/config.go
package runtime

import (
    "fmt"
    "net"
    "net/url"
    "strings"

    "github.com/creasty/defaults"
    "github.com/go-playground/validator/v10"
)

var validate *validator.Validate

func init() {
    validate = validator.New()

    // Framework registers common validators
    validate.RegisterValidation("hostname_port", func(fl validator.FieldLevel) bool {
        addr := fl.Field().String()
        host, port, err := net.SplitHostPort(addr)
        return err == nil && host != "" && port != ""
    })

    validate.RegisterValidation("url_format", func(fl validator.FieldLevel) bool {
        s := fl.Field().String()
        u, err := url.Parse(s)
        return err == nil && u.Scheme != "" && u.Host != ""
    })

    validate.RegisterValidation("dsn", func(fl validator.FieldLevel) bool {
        s := fl.Field().String()
        return strings.Contains(s, "://") || strings.Contains(s, "@")
    })
}

// ApplyDefaults applies default:"..." tags to config
func ApplyDefaults(config any) error {
    return defaults.Set(config)
}

// ValidateConfig validates using validate:"..." tags
func ValidateConfig(config any) error {
    return validate.Struct(config)
}

// PrepareConfig applies defaults and validates (convenience)
func PrepareConfig(config any) error {
    if err := ApplyDefaults(config); err != nil {
        return fmt.Errorf("failed to apply defaults: %w", err)
    }

    if err := ValidateConfig(config); err != nil {
        return fmt.Errorf("validation failed: %w", err)
    }

    return nil
}

// ApplyEnvOverrides applies environment variable overrides
func ApplyEnvOverrides(config any, overrides map[string]string) error {
    // Uses reflection to set field values from env vars
    // Handles type conversions (string → int, duration, bool, etc.)
    // Implementation details in runtime package
    return nil
}

// RegisterCustomValidator allows plugins to add custom validators if needed
func RegisterCustomValidator(tag string, fn validator.Func) error {
    return validate.RegisterValidation(tag, fn)
}
```

---

## User Side: flow-config.yaml

Users specify env var mappings (not hardcoded by plugin developer):

```yaml
plugins:
  # Redis - user chooses env var names
  - source: redis
    config:
      addr: ${REDIS_ADDR:localhost:6379}  # User's choice of env var name
      password: ${REDIS_PASSWORD}         # User's choice
      pool_size: 20                       # Override default with literal
      # db and timeout use tag defaults

  # Postgres - different user, different names
  - source: postgres
    config:
      dsn: ${DATABASE_URL}                # User's choice
      max_open_conns: ${DB_MAX_CONNS:25}

  # Multiple instances with different configs
  - source: redis
    name: redis_cache
    config:
      addr: localhost:6379
      db: 0

  - source: redis
    name: redis_sessions
    config:
      addr: localhost:6379
      db: 1
```

**Environment Variable Syntax:**

| Syntax | Behavior | Example |
|--------|----------|---------|
| `${VAR}` | Required env var, error if missing | `${DATABASE_URL}` |
| `${VAR:default}` | Optional with default | `${REDIS_ADDR:localhost:6379}` |
| Plain value | Literal value | `localhost:6379` |

**Precedence order:**
1. Environment variable (runtime - highest priority)
2. flow-config.yaml literal value (build-time)
3. struct tag `default:"..."` (lowest priority)

---

## CLI Generated Code

CLI generates clean initialization using framework functions:

```go
// main.go (GENERATED by CLI)
package main

import (
    "log"
    "os"

    "github.com/sflowg/sflowg/runtime"
    "github.com/sflowg/sflowg/plugins/redis"
    "github.com/sflowg/sflowg/plugins/postgres"
)

func main() {
    container := runtime.NewContainer()

    // ===== Redis Plugin =====
    redisConfig := redis.Config{}

    // Framework applies defaults from tags
    if err := runtime.PrepareConfig(&redisConfig); err != nil {
        log.Fatalf("Redis config preparation failed: %v", err)
    }

    // Apply env var overrides (user-specified names)
    runtime.ApplyEnvOverrides(&redisConfig, map[string]string{
        "Addr":     os.Getenv("REDIS_ADDR"),
        "Password": os.Getenv("REDIS_PASSWORD"),
    })

    // Override with literal from flow-config.yaml
    redisConfig.PoolSize = 20

    // Validate after overrides
    if err := runtime.ValidateConfig(redisConfig); err != nil {
        log.Fatalf("Redis config invalid: %v", err)
    }

    redisPlugin := redis.NewPlugin(redisConfig)
    container.RegisterPlugin("redis", redisPlugin)

    // ===== Postgres Plugin =====
    postgresConfig := postgres.Config{}

    runtime.PrepareConfig(&postgresConfig)

    runtime.ApplyEnvOverrides(&postgresConfig, map[string]string{
        "DSN": os.Getenv("DATABASE_URL"),
    })

    // Check required fields
    if postgresConfig.DSN == "" {
        log.Fatal("Required environment variable DATABASE_URL not set")
    }

    runtime.ValidateConfig(postgresConfig)

    postgresPlugin := postgres.NewPlugin(postgresConfig)
    container.RegisterPlugin("postgres", postgresPlugin)

    // Initialize and run
    ctx := context.Background()
    if err := container.Initialize(ctx); err != nil {
        log.Fatal(err)
    }

    app := runtime.NewApp(container)
    app.Run(":8080")
}
```

---

## Complete Example: Postgres Plugin

```go
// plugins/postgres/config.go
package postgres

import "time"

// Just the struct with tags - that's all!
type Config struct {
    DSN             string        `yaml:"dsn" validate:"required,url_format"`
    MaxOpenConns    int           `yaml:"max_open_conns" default:"25" validate:"gte=1,lte=1000"`
    MaxIdleConns    int           `yaml:"max_idle_conns" default:"5" validate:"gte=1"`
    ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime" default:"1h" validate:"gte=0"`
}

// plugins/postgres/plugin.go
package postgres

import (
    "context"
    "database/sql"

    _ "github.com/lib/pq"
)

type Plugin struct {
    config Config
    db     *sql.DB
}

func NewPlugin(config Config) *Plugin {
    return &Plugin{config: config}
}

func (p *Plugin) Initialize(ctx context.Context) error {
    // Config already validated by framework
    db, err := sql.Open("postgres", p.config.DSN)
    if err != nil {
        return err
    }

    db.SetMaxOpenConns(p.config.MaxOpenConns)
    db.SetMaxIdleConns(p.config.MaxIdleConns)
    db.SetConnMaxLifetime(p.config.ConnMaxLifetime)

    if err := db.PingContext(ctx); err != nil {
        return err
    }

    p.db = db
    return nil
}

func (p *Plugin) Shutdown(ctx context.Context) error {
    return p.db.Close()
}
```

---

## Validation Rules

Common validation tags (from go-playground/validator):

```go
type Config struct {
    // Required field
    APIKey string `validate:"required"`

    // Numeric ranges
    Port   int    `validate:"gte=1,lte=65535"`

    // String length
    Name   string `validate:"min=3,max=50"`

    // Format validation
    Email  string `validate:"email"`
    URL    string `validate:"url"`

    // Custom validators (framework-provided)
    Addr   string `validate:"hostname_port"`
    DSN    string `validate:"dsn"`

    // Enums
    Level  string `validate:"oneof=debug info warn error"`

    // Cross-field validation (requires struct-level validator)
    MaxIdle int `validate:"ltefield=MaxOpen"`
}
```

---

## Summary

**Plugin developer writes:**
```go
type Config struct {
    Field string `yaml:"field" default:"value" validate:"required"`
}
```

**Framework provides:**
```go
runtime.PrepareConfig(&config)         // Applies defaults + validates
runtime.ApplyEnvOverrides(&config, map) // Applies env vars
runtime.ValidateConfig(config)          // Validates constraints
```

**User configures:**
```yaml
config:
  field: ${ENV_VAR}  # User chooses env var names
```

**Zero boilerplate for plugin developers!** Framework handles all defaults, validation, and env var processing.