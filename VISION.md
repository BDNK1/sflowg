# SFlowG Vision 2.0: The Missing Middle for Service Development

## Core Concept

**SFlowG = "Service Flow Generator"**
*The missing tool between full microservices and visual workflow builders*

SFlowG is a **declarative service construction framework** for technical developers who are stuck between two painful options:

**Option 1: Full Microservice** ➔ Weeks of boilerplate for simple services
**Option 2: Visual Workflow Tools (n8n, Zapier)** ➔ Not designed for developers, limited power

**SFlowG bridges this gap** ➔ Developer-friendly YAML + Expression engine + Production-ready runtime

---

## The Problem: The Missing Middle

### Current Developer Pain

Technical developers face a **false choice** when building services:

```
┌─────────────────────────────────────────────────────────┐
│                                                         │
│  FULL MICROSERVICE               VISUAL WORKFLOW        │
│  (Express, Flask, Go HTTP)       (n8n, Zapier)          │
│                                                         │
│  ✅ Full control                 ✅ Fast setup          │
│  ✅ Developer-friendly           ✅ No code required    │
│  ✅ Production-ready             ✅ Visual editor       │
│                                                         │
│  ❌ Weeks of boilerplate         ❌ Not for developers  │
│  ❌ Framework lock-in            ❌ Limited expressions │
│  ❌ Overkill for simple logic   ❌ Visual UI overhead   │
│                                                         │
└─────────────────────────────────────────────────────────┘
                            │
                            ▼
                    ┌───────────────┐
                    │   SFlowG      │
                    │ THE MISSING   │
                    │    MIDDLE     │
                    └───────────────┘

                    ✅ Developer YAML (not visual)
                    ✅ Expression-driven power
                    ✅ Hours not weeks
                    ✅ Production-ready runtime
                    ✅ Easy migration when needed
```

### What Developers Actually Need

**For 70% of services** (simple integrations, adapters, webhooks, schedulers):
- Fast development without framework boilerplate
- Developer-friendly configuration (YAML, expressions, Git-trackable)
- Production-ready runtime that stays stable
- No migration pressure when services stay simple

**For 30% of services** (prototypes that might grow):
- Quick validation of API contracts and business logic
- Real HTTP endpoints for testing with actual traffic
- Easy migration path when complexity grows
- No regret about time invested if prototype gets discarded

**SFlowG serves both**: One tool, flexible service lifecycle.

---

## The SFlowG Solution: Flexible Service Lifecycle

### One Tool, Three Lifecycle Paths

```
SERVICE STARTS SIMPLE
        │
        ▼
┌───────────────────┐
│  Build with       │
│  SFlowG (hours)   │
└───────────────────┘
        │
        ├─────────────────────┬─────────────────────┬──────────────────────┐
        ▼                     ▼                     ▼                      ▼
┌──────────────┐      ┌──────────────┐     ┌──────────────┐      ┌──────────────┐
│ PATH 1:      │      │ PATH 2:      │     │ PATH 3:      │      │ PATH 4:      │
│ Stays Simple │      │ Grows Slowly │     │ Grows Fast   │      │ Gets Killed  │
│              │      │              │     │              │      │              │
│ Keep YAML    │      │ Keep YAML    │     │ Migrate to   │      │ Discard with │
│ Production   │      │ Small edits  │     │ Full Service │      │ No Regret    │
│ Runtime      │      │ Easy changes │     │ Use YAML as  │      │ Minimal time │
│              │      │              │     │ AI spec      │      │ invested     │
└──────────────┘      └──────────────┘     └──────────────┘      └──────────────┘
   (70% cases)           (20% cases)          (10% cases)           (Variable)
```

### Key Proposition: Flexibility Without Lock-In

**SFlowG doesn't force you to choose upfront** whether this service will stay simple or grow complex.

✅ **Start fast**: Service running in hours, not weeks
✅ **Stay flexible**: YAML edits for simple changes
✅ **Migrate easily**: YAML = perfect spec for AI-assisted migration
✅ **No regret**: Minimal investment if service gets killed

**You're not locked in** - SFlowG is a **stepping stone, not a cage**.

---

## Architecture: Configuration as Code

### Traditional vs SFlowG Approach

**Traditional Service Development:**
```
HTTP Framework + Business Logic (Code) + Config Files = Service
├── Server setup (Express, Flask, Gin)
├── Request parsing and validation
├── Routing and middleware
├── Business logic implementation
├── Error handling
├── Logging and monitoring
└── Deployment configuration
```
**Time Investment**: 2-4 weeks for production-ready service

**SFlowG Approach:**
```
Declarative Flow (YAML) + Custom Tasks (Go) = Service
├── HTTP routing auto-generated from flow
├── Expression-driven business logic
├── Built-in retry, error handling, logging
└── Production runtime included
```
**Time Investment**: 1-4 hours for production-ready service

---

## Target Users: Technical Developers

### Primary Archetypes

#### 1. **Backend Developers**
**Pain**: "I need a simple service but don't want to setup Express/Flask/Go HTTP server"

**SFlowG Value**:
- No framework boilerplate
- Focus on business logic only
- Fast iteration on simple services

**Use Cases**:
- API proxies with caching/rate-limiting
- Webhook handlers (GitHub, Stripe, Twilio)
- Admin endpoints and internal tools
- Data fetchers and scheduled jobs

---

#### 2. **Platform Engineers**
**Pain**: "Teams keep asking for the same integration patterns, I'm building boilerplate repeatedly"

**SFlowG Value**:
- Create reusable flow templates
- Teams copy/customize for their needs
- Standardize internal service patterns

**Template Library Example**:
```
flows/templates/
├── rest-api-wrapper.yaml      # Standard REST facade
├── webhook-handler.yaml       # Generic webhook processor
├── scheduled-job.yaml         # Cron job template
├── health-aggregator.yaml     # Service health checks
└── notification-router.yaml   # Multi-channel notifier
```

**Impact**: "Stop writing the same integration boilerplate 50 times"

---

#### 3. **DevOps/SRE**
**Pain**: "Operational services (health checks, alerts, monitors) require full microservice setup"

**SFlowG Value**:
- Quick operational tooling without boilerplate
- Treat operational services as configuration
- Git-trackable, reviewable YAML

**Use Cases**:
- Deployment health checkers
- Alert aggregators (PagerDuty + DataDog + Custom)
- Chaos engineering triggers
- Backup validators
- Service discovery helpers

**Impact**: "Operational services in YAML, not another microservice repo"

---

#### 4. **API Integration Specialists**
**Pain**: "Each client integration becomes a new microservice with identical patterns"

**SFlowG Value**:
- One flow template, many client configurations
- Environment variables per client
- Standardized integration patterns

**Multi-Tenant Example**:
```yaml
# Flow template: crm_sync.yaml
id: crm_sync
steps:
  - id: fetchData
    type: http
    args: {url: properties.crmUrl}
  - id: transformData
    type: assign
  - id: postToInternal
    type: http

# Client A: .env
CRM_URL=https://salesforce.client-a.com
CRM_API_KEY=client-a-key

# Client B: .env
CRM_URL=https://hubspot.client-b.com
CRM_API_KEY=client-b-key
```

**Impact**: "One flow template, 50 client configurations"

---

#### 5. **Startup/Solo Developers**
**Pain**: "Need many small services, limited time and resources"

**SFlowG Value**:
- Build entire backend as collection of flows
- Each flow = single-purpose service
- Rapid iteration without framework overhead

**Startup Stack Example**:
```
flows/
├── user-registration.yaml
├── email-verification.yaml
├── password-reset.yaml
├── payment-checkout.yaml
├── webhook-stripe.yaml
├── webhook-sendgrid.yaml
├── admin-user-management.yaml
└── scheduled-analytics-email.yaml
```

**Impact**: "8 services in 8 hours instead of 8 weeks"

---

#### 6. **Prototypers/Researchers**
**Pain**: "Need to validate API design before investing in full implementation"

**SFlowG Value**:
- Functional prototype with real HTTP endpoints
- Test with actual frontend/client code
- Iterate on API contract rapidly

**Workflow**:
```
1. Define API contract in YAML (30 min)
2. Deploy to staging (5 min)
3. Test with frontend team (hours/days)
4. Iterate on YAML (minutes per change)
5. IF successful → migrate to optimized service
   ELSE → discard with minimal waste
```

**Impact**: "Fail fast on API design, not implementation"

---

## Use Cases: Services That Stay Simple

### Category 1: Third-Party Integration Facades

**Pattern**: Wrap external APIs to standardize internal usage

**Examples**:
```yaml
# Stripe payment facade
id: stripe_facade
steps:
  - id: createCustomer
    type: http
    args: {url: "https://api.stripe.com/v1/customers"}
  - id: createPaymentIntent
    type: http
```

**Why SFlowG**: API changes → update YAML. No migration needed.

**Other Examples**:
- Twilio SMS facade
- SendGrid email facade
- Google Maps API wrapper
- Slack notification service
- Auth0 authentication proxy

---

### Category 2: Webhook Receivers & Routers

**Pattern**: Receive webhooks, route to internal services

**Examples**:
```yaml
# GitHub webhook → Slack + Jira
id: github_webhook_router
steps:
  - id: validateWebhook
    type: github-signature
  - id: notifySlack
    type: http
  - id: createJiraTicket
    condition: request.body.action == "opened"
    type: http
```

**Why SFlowG**: Pure routing logic, stays stable.

**Other Examples**:
- Stripe webhook processor
- Shopify order webhook handler
- Calendar event webhook router
- Payment notification distributor

---

### Category 3: Internal Service Adapters

**Pattern**: Translate between different service versions or formats

**Examples**:
```yaml
# Legacy API → New API translator
id: legacy_adapter
steps:
  - id: transformRequest
    type: assign
    args:
      newFormat: {
        customerId: request.body.customer_id,
        amount: request.body.total_amount
      }
  - id: callNewService
    type: http
```

**Why SFlowG**: Single-purpose translation, doesn't evolve.

**Other Examples**:
- REST → GraphQL adapter
- SOAP → REST translator
- Old DB schema → New schema bridge
- Multi-version API router

---

### Category 4: Scheduled Job Coordinators

**Pattern**: Cron-triggered data processing and notifications

**Examples**:
```yaml
# Daily analytics report
id: daily_report_job
steps:
  - id: fetchData
    type: http
  - id: generatePDF
    type: pdf-generator
  - id: emailReport
    type: email
```

**Why SFlowG**: Simple ETL that never grows complex.

**Other Examples**:
- Database backup validators
- Daily data sync jobs
- Weekly report generators
- Cleanup/archival jobs
- Health check schedulers

---

### Category 5: Health Check Aggregators

**Pattern**: Monitor multiple services, return unified health status

**Examples**:
```yaml
# Aggregate system health
id: health_checker
steps:
  - id: checkDatabase
    type: http
  - id: checkCache
    type: http
  - id: checkQueue
    type: http
return:
  args:
    healthy: all([checkDatabase.ok, checkCache.ok, checkQueue.ok])
```

**Why SFlowG**: Fixed monitoring logic, just URLs change.

**Other Examples**:
- Kubernetes readiness probes
- Load balancer health endpoints
- Dependency health dashboards
- SLA monitoring services

---

### Category 6: Notification Routers

**Pattern**: Send notifications via user preferences

**Examples**:
```yaml
# Route notification by user preference
id: notify_user
steps:
  - id: getPreference
    type: database
  - type: switch
    args:
      email: getPreference.result.channel == "email"
      sms: getPreference.result.channel == "sms"
      push: getPreference.result.channel == "push"
```

**Why SFlowG**: Routing logic, not business logic.

**Other Examples**:
- Multi-channel alert dispatcher
- Priority-based notification router
- Timezone-aware message scheduler
- Communication preference manager

---

### Category 7: Data Transformation Pipelines

**Pattern**: Transform data formats between services

**Examples**:
```yaml
# CSV upload → JSON API
id: csv_importer
steps:
  - id: parseCSV
    type: csv-parser
  - id: validateRows
    type: validate
  - id: bulkInsert
    type: http
```

**Why SFlowG**: ETL pattern, stable once working.

**Other Examples**:
- JSON → XML converter
- Database export → API sync
- File upload processor
- Data format normalizer

---

### Category 8: API Rate Limit Proxies

**Pattern**: Queue and throttle requests to rate-limited APIs

**Examples**:
```yaml
# Rate-limited API proxy
id: rate_limiter_proxy
steps:
  - id: checkQuota
    type: redis
  - id: callAPI
    condition: checkQuota.result.remaining > 0
    retry: {max: 3, backoff: exponential}
  - id: queueForLater
    condition: checkQuota.result.remaining == 0
    type: queue
```

**Why SFlowG**: Infrastructure concern, not product logic.

**Other Examples**:
- Third-party API throttler
- Credit-based API gateway
- Fair-use quota enforcer
- Burst protection proxy

---

### Category 9: Internal Tool Backends

**Pattern**: Simple CRUD for admin panels and internal dashboards

**Examples**:
```yaml
# Admin panel backend
id: admin_tools
entrypoint:
  path: /admin/users/:action
steps:
  - id: authenticate
    type: auth-check
  - type: switch
    args:
      listUsers: request.params.action == "list"
      blockUser: request.params.action == "block"
      resetPassword: request.params.action == "reset"
```

**Why SFlowG**: Internal tools rarely grow complex.

**Other Examples**:
- Feature flag admin API
- Configuration management backend
- User impersonation service
- Debug/diagnostics endpoints

---

### Category 10: Feature Flag Services

**Pattern**: Dynamic feature toggle without deployment

**Examples**:
```yaml
# Feature flag service
id: feature_flags
steps:
  - id: getUserFlags
    type: database
  - id: evaluateRules
    type: assign
    args:
      enabled: getUserFlags.result.beta_user == true ||
               hash(request.user.id) % 100 < properties.rolloutPercentage
return:
  args:
    feature_enabled: evaluateRules.result.enabled
```

**Why SFlowG**: Configuration logic, perfect for YAML.

**Other Examples**:
- Percentage-based rollouts
- User segment toggles
- Geographic feature control
- A/B test router

---
## Migration Strategy: Easy Exit

### YAML as AI-Ready Specification

When services grow complex, SFlowG flows become **perfect specifications** for AI-assisted migration:

**Original SFlowG Flow:**
```yaml
id: payment_service
steps:
  - id: validatePayment
    condition: request.body.amount > 100
  - id: callStripe
    type: http
    retry: {max: 3}
  - id: saveTransaction
    type: database
```

**Migration Prompt to AI:**
```
Convert this SFlowG flow to Express.js microservice:
[paste YAML]

Requirements:
- Same HTTP endpoints
- Same validation logic
- Same retry behavior
- Add comprehensive tests
```

**Result**: Spec-driven development with zero ambiguity.

### Migration Paths

**Path 1: Gradual (Hybrid Approach)**
```
Keep simple flows in SFlowG
↓
Extract complex logic to separate services
↓
SFlowG becomes orchestrator
```

**Path 2: Full Migration**
```
Use YAML as specification
↓
AI-generate full microservice
↓
Test against same HTTP contracts
↓
Deploy replacement
```

**Path 3: Stay Forever**
```
Service stays simple
↓
YAML edits handle all changes
↓
No migration needed
```

**Key Principle**: SFlowG never locks you in. YAML is your spec, your documentation, and your migration guide.

---

## Competitive Landscape

### Current Market Gap

```
VISUAL WORKFLOW TOOLS          TRADITIONAL FRAMEWORKS
(n8n, Zapier, Make)            (Express, Flask, Go HTTP)
        │                              │
        │         MISSING              │
        │          MIDDLE              │
        │            ▼                 │
        │        ┌────────┐            │
        └───────►│ SFlowG │◄───────────┘
                 └────────┘
```

### vs. Visual Workflow Tools (n8n, Zapier, Make)

**Why NOT n8n for developers:**
- ❌ Visual UI overhead (developers prefer code/YAML)
- ❌ Limited expression power (JavaScript snippets, not unified expressions)
- ❌ Not Git-friendly (JSON exports, not clean YAML)
- ❌ Node-based mental model (developers think in steps/functions)
- ❌ Deployment complexity (self-hosted or vendor lock-in)

**SFlowG advantages:**
- ✅ Developer YAML (Git-trackable, reviewable, diffable)
- ✅ Powerful expression engine (expr-lang, full context access)
- ✅ Code-first approach (no visual editor required)
- ✅ Standard deployment (Docker, K8s, any Go runtime)
- ✅ Custom task extensibility (write Go tasks)

---

### vs. Traditional Microservice Frameworks

**Why NOT Express/Flask/Go HTTP for simple services:**
- ❌ Weeks of boilerplate (routing, middleware, error handling)
- ❌ Framework-specific patterns (learning curve, lock-in)
- ❌ Overkill for simple integrations
- ❌ Testing overhead (mocking, fixtures, integration tests)

**SFlowG advantages:**
- ✅ Hours not weeks (30min to working service)
- ✅ Framework-agnostic (just YAML + tasks)
- ✅ Right-sized for simple services
- ✅ Built-in testing (request/response YAML definitions)
- ✅ Easy migration (YAML = specification for full service)

---

### Inspiration Sources (Not Direct Competitors)

**Windmill**: Code-first workflow platform (Python/TypeScript scripts)
**Ballerina Language**: Network-aware programming language
**Apache Camel**: Enterprise integration patterns
**Kestra**: Infrastructure orchestration
**Temporal**: Workflow orchestration engine


---

## SFlowG Features: Complete Reference

### Core Flow Features

#### **1. HTTP Entrypoints**
Define RESTful endpoints with path variables, query parameters, headers, and body parsing.

```yaml
entrypoint:
  type: http
  config:
    method: post
    path: /orders/:orderId/process
    headers: ["X-API-Key", "Authorization"]
    queryParameters: ["status", "limit"]
    body:
      type: json
```

**Capabilities:**
- RESTful routing (GET, POST, PUT, DELETE, PATCH)
- Path variables (`:id`, `:name`)
- Query parameter parsing
- Header extraction
- JSON/form-data body parsing

---

#### **2. Expression Engine**
Powerful runtime expressions using `expr-lang` with full context access.

```yaml
steps:
  - id: calculateTotal
    type: assign
    args:
      subtotal: request.body.items.map(i => i.price * i.quantity).sum()
      tax: subtotal * properties.taxRate
      total: subtotal + tax
      discount: total > 100 ? total * 0.1 : 0
      finalAmount: total - discount
```

**Expression Features:**
- Dot notation access: `request.body.amount.total`
- Arithmetic: `+`, `-`, `*`, `/`, `%`
- Comparison: `>`, `<`, `>=`, `<=`, `==`, `!=`
- Logical: `&&`, `||`, `!`
- Ternary: `condition ? true : false`
- Array operations: `map()`, `filter()`, `sum()`, `len()`
- String operations: concatenation, interpolation
- Function calls: `hash()`, `now()`, `format()`

---

#### **3. Step Types**

##### **assign** - Variable Assignment
```yaml
- id: computeDiscount
  type: assign
  args:
    discountRate: request.body.isVIP ? 0.15 : 0.05
    discountAmount: request.body.total * discountRate
```

##### **http** - HTTP Requests
```yaml
- id: callPaymentAPI
  type: http
  args:
    url: properties.paymentServiceUrl + "/charge"
    method: post
    headers:
      Authorization: "Bearer " + properties.apiKey
    body:
      amount: request.body.total
      currency: "USD"
  retry:
    max: 3
    backoff: exponential
    condition: callPaymentAPI.result.status >= 500
  timeout: 5s
```

##### **switch** - Conditional Branching
```yaml
- type: switch
  args:
    highValue: request.body.amount > 1000
    mediumValue: request.body.amount > 100
    lowValue: true  # Default case
```

##### **foreach** - Array Iteration
```yaml
- id: processOrders
  type: foreach
  items: request.body.orders
  continueOnError: true
  step:
    id: processOrder
    type: http
    args:
      url: properties.orderServiceUrl
      body: ${item}
```

##### **flow.call** - Flow Composition
```yaml
- id: calculateTax
  type: flow.call
  flow: tax-calculator
  entrypoint: calculate
  args:
    amount: request.body.total
    country: request.body.country
```

---

#### **4. Conditional Execution**
Run steps based on runtime conditions.

```yaml
steps:
  - id: validateAmount
    type: http
    condition: request.body.amount > properties.minAmount
    args:
      url: properties.validationServiceUrl
```

---

#### **5. Retry Logic**
Automatic retry with exponential backoff and conditional retries.

```yaml
retry:
  max: 3
  backoff: exponential  # linear, exponential, fixed
  initialDelay: 1s
  maxDelay: 30s
  condition: ${step.result.status >= 500}  # Only retry server errors
```

---

#### **6. Error Handling**

##### **Step-level Error Handling**
```yaml
- id: optionalCache
  type: redis.get
  continueOnError: true  # Sets result to null, continues flow

- id: fetchWithFallback
  type: http
  default:  # Use if step fails
    status: "offline"
    cached: false
```

##### **Flow-level Error Response**
```yaml
errorResponse:
  status: 500
  body:
    success: false
    error: ${error.message}
    requestId: ${execution.id}
    timestamp: ${now()}
```

---

#### **7. Plugin System**

##### **Connectors** - Infrastructure with Lifecycle
```yaml
# flow-config.yaml
connectors:
  - name: postgres
    type: postgresql
    config:
      url: ${DATABASE_URL}
      maxOpenConns: 25

  - name: redis
    type: redis
    config:
      url: ${REDIS_URL}
      poolSize: 10

  - name: logger
    type: logging
    config:
      level: info
      format: json
```

##### **Tasks** - Custom Business Logic
Write custom Go tasks for domain-specific operations.

```go
type CustomValidator struct{}

func (t *CustomValidator) ValidateOrder(ctx *Execution, args map[string]any) (map[string]any, error) {
    // Custom validation logic
    return map[string]any{"valid": true}, nil
}
```

---

#### **8. Built-in Plugins**

##### **Logging Plugin**
```yaml
- id: logEvent
  type: logger.info
  args:
    message: "Order processed"
    fields:
      orderId: ${request.params.orderId}
      userId: ${request.body.userId}
      amount: ${request.body.amount}
```

##### **Redis Plugin**
```yaml
- id: cacheResult
  type: redis.set
  args:
    key: "user:${request.params.userId}"
    value: ${fetchUser.result}
    ttl: 3600  # 1 hour

- id: checkCache
  type: redis.get
  args:
    key: "user:${request.params.userId}"
```

##### **PostgreSQL Plugin**
```yaml
- id: fetchUser
  type: postgres.queryRow
  args:
    query: "SELECT id, name, email FROM users WHERE id = $1"
    params: [${request.params.userId}]

- id: updateUser
  type: postgres.execute
  args:
    query: "UPDATE users SET last_login = NOW() WHERE id = $1"
    params: [${request.params.userId}]
```

---

#### **9. Environment Variables**
12-factor app configuration with defaults.

```yaml
properties:
  apiUrl: "${API_URL:http://localhost:8080}"
  maxRetries: "${MAX_RETRIES:3}"
  dbConnection: "${DATABASE_URL}"  # Required, no default
  apiKey: "${API_KEY}"
```

---

#### **10. Flow Composition**
Reuse flows as functions without HTTP overhead.

```yaml
# tax-calculator.yaml
id: tax_calculator
entrypoint:
  type: function
  id: calculate

steps:
  - id: getTaxRate
    type: postgres.queryRow
    args:
      query: "SELECT rate FROM tax_rates WHERE country = $1"
      params: [${input.country}]

  - id: compute
    type: assign
    args:
      taxAmount: ${input.amount} * ${getTaxRate.rate}

return:
  type: function.return
  args:
    taxAmount: ${compute.taxAmount}
    rate: ${getTaxRate.rate}
```

```yaml
# main-flow.yaml
steps:
  - id: calculateTax
    type: flow.call
    flow: tax_calculator
    entrypoint: calculate
    args:
      amount: ${request.body.total}
      country: ${request.body.country}
```

---

#### **11. Graceful Shutdown**
Clean resource cleanup on termination.

- HTTP server graceful shutdown (30s timeout)
- Connector cleanup (close DB/Redis connections)
- In-flight request completion
- Signal handling (SIGTERM, SIGINT)

---

#### **12. Production-Ready Runtime**

✅ **HTTP Server**: Gin-based, battle-tested
✅ **Request Parsing**: JSON, form data, path variables, query params
✅ **Response Formatting**: Template-based, type-safe
✅ **Error Handling**: Structured error responses
✅ **Logging**: Request tracing with slog
✅ **Execution Context**: UUID-tracked request lifecycle
✅ **Health Checks**: Built-in connector health monitoring
✅ **Timeouts**: Per-step timeout enforcement

---

#### **13. Hot Reload (YAML Flows)**
Zero-downtime flow updates for development and production.

**File System Watcher:**
```go
// Watches flows/ directory for changes
// Automatically reloads flows on save
// Atomic swap with validation
// Rollback on parse errors
```

**Development Workflow (Phase 1: Manual):**
```bash
# Build binary first
sflowg build . -o my-service

# Run binary with hot reload enabled
./my-service --hot-reload

# Edit flow.yaml in your editor
vim flows/order-processing.yaml

# Save file
# → SFlowG detects change
# → Validates YAML
# → Reloads flow atomically
# → Next request uses new flow
# ✅ Zero downtime, instant feedback (< 1s)
```

**Development Workflow (Phase 2: Dev Mode - Roadmap):**
```bash
# Convenience command: build + run + watch
sflowg dev

# Behind the scenes:
# 1. Builds binary with plugins
# 2. Runs with --hot-reload
# 3. Watches for changes:
#    - YAML → hot reload (< 1s)
#    - Plugins → rebuild + restart (~5s)

# Edit YAML flows
vim flows/order-processing.yaml
# → Instant reload, no rebuild needed ✅

# Edit plugins (if needed)
vim plugins/custom-validator.go
# → Auto-rebuild + restart (~5s) ✅
```

**Production Deployment:**
```bash
# Build production binary
sflowg build . -o my-service

# Run in production with hot reload
./my-service --hot-reload --env production

# Deploy new flow version
kubectl apply -f configmap.yaml  # or git pull

# → Flows reload automatically
# → In-flight requests: old flow
# → New requests: new flow
# ✅ Zero downtime deployment
```

**What Gets Hot Reloaded:**
- ✅ YAML flow definitions (instant, no rebuild)
- ✅ Flow logic changes (steps, conditions, expressions)
- ✅ HTTP routing changes (paths, methods)
- ❌ Plugin code changes (requires rebuild)
- ❌ Connector configuration (requires restart)

**Features:**
- Automatic YAML validation before reload
- Atomic flow swap (mutex-protected)
- Error handling (keeps old flow on failure)
- Reload event logging
- Health endpoint shows flow versions
- No restart required (YAML only)
- No orchestration needed

**Benefits:**
- ⏱️ Development feedback: < 1 second (YAML changes)
- 🚀 Production deployment: < 5 seconds
- ✅ Zero downtime for YAML changes
- 🔒 Safe: validates before swapping
- 📊 Observable: logs all reload events
- 🔧 Plugin changes: ~5s rebuild (Phase 2 dev mode)

---

#### **14. CLI Build System**
Generate standalone binaries from flows.

```bash
# Project structure
my-service/
├── flow.yaml              # Flow definition
├── flow-config.yaml       # Plugin configuration
└── plugins/
    └── custom-validator.go

# Build command
sflowg build my-service

# Output: my-service binary (standalone executable)
```

**Plugin Type Support:**
- **Remote modules**: `github.com/user/plugin@v1.2.3`
- **Local modules**: `../shared-plugins/validator`
- **Vendor files**: Copy `.go` files directly into build

---

### Feature Summary Table

| Feature | Status | Description |
|---------|--------|-------------|
| **HTTP Entrypoints** | ✅ Production | RESTful routing with path/query/headers/body |
| **Expression Engine** | ✅ Production | expr-lang with full context access |
| **Step Types** | ✅ Production | assign, http, switch, foreach, flow.call |
| **Conditional Execution** | ✅ Production | condition field on any step |
| **Retry Logic** | ✅ Production | Exponential backoff, conditional retries |
| **Error Handling** | ✅ Production | continueOnError, default, errorResponse |
| **Plugin System** | ✅ Production | Connectors (lifecycle) + Tasks (stateless) |
| **Logging Plugin** | ✅ Production | Structured logging with slog |
| **Redis Plugin** | ✅ Production | Connection pooling, Get/Set/Del/Incr |
| **PostgreSQL Plugin** | ✅ Production | Connection pooling, Query/Execute |
| **Environment Variables** | ✅ Production | ${VAR:default} syntax |
| **Flow Composition** | ✅ Production | Local flow calls, function.return |
| **Graceful Shutdown** | ✅ Production | Signal handling, connector cleanup |
| **Hot Reload** | 🔄 Roadmap | Zero-downtime YAML flow updates |
| **CLI Build System** | ✅ Production | 3 plugin types, standalone binaries |
| **Foreach Loops** | ✅ Production | Array iteration, batch optimization |
| **Timeouts** | ✅ Production | Per-step timeout enforcement |
| **Health Checks** | ✅ Production | Connector health monitoring |
| **Kafka Plugin** | 🔄 Roadmap | Producer/Consumer operations |
| **Data Masking Plugin** | 🔄 Roadmap | PII masking for logs |
| **Observability Plugin** | 🔄 Roadmap | OpenTelemetry integration |
| **Multi-Trigger Support** | 🔄 Roadmap | HTTP, Kafka, Cron triggers |

---

## Developer Experience

### Runtime Model: Single Compiled Binary

**SFlowG has ONE runtime model** - a compiled Go binary with plugins linked at build time.

```
❌ NO interpreted mode (cannot "run" YAML directly)
❌ NO scripting runtime (plugins must be compiled)
✅ YES compiled binary (production-ready from start)
```

**Why compiled-only?**
- Plugins (remote/local/vendor) require Go compilation
- Single runtime = simpler architecture
- Production-ready performance from development
- Hot reload handles YAML changes without rebuild

---

### CLI Workflows

**Production Workflow (Explicit Build):**
```bash
# Build standalone binary
sflowg build my-service -o my-service-binary

# Run binary
./my-service-binary

# Deploy to production
docker build -t my-service .
kubectl apply -f deployment.yaml
```

**Development Workflow (Convenience Mode - Phase 2/Roadmap):**
```bash
# Convenience command: build + run + watch
sflowg dev

# Behind the scenes:
# 1. Builds binary with plugins
# 2. Runs binary with --hot-reload
# 3. Watches for changes:
#    - YAML changes → hot reload (< 1s)
#    - Plugin changes → rebuild + restart (~5s)
```

**Development Experience:**

| Change Type | Detection | Action | Time |
|-------------|-----------|--------|------|
| Edit YAML flow | File watcher | Hot reload | < 1s |
| Edit plugin code | File watcher | Rebuild + restart | ~5s |
| Add new plugin | Manual | Rebuild required | ~5s |

---

### Quick Start (5 Minutes)

```bash
# 1. Install
go install github.com/yourusername/sflowg@latest

# 2. Create flow
cat > flows/hello.yaml <<EOF
id: hello_service
entrypoint:
  type: http
  config:
    method: get
    path: /hello/:name
return:
  type: http.response
  args:
    status: 200
    body:
      message: "Hello, " + request.params.name
EOF

# 3. Build and run (Phase 1: Manual)
sflowg build . -o hello-service
./hello-service

# OR (Phase 2: Dev mode convenience)
sflowg dev

# 4. Test
curl http://localhost:8080/hello/world
# {"message": "Hello, world"}
```

### Development Workflow

```
1. Write YAML flow           (30 min)
2. Build binary              (5 sec)
3. Start server              (instant)
4. Test with real HTTP       (minutes)
5. Edit YAML                 (minutes per change)
6. Hot reload                (< 1s)  ← No restart needed!
7. Edit plugin (optional)    (5 min)
8. Rebuild                   (~5s)   ← Only for plugin changes
9. Deploy to staging         (standard Docker/K8s)
```

**Developer Feedback Loop**:
- YAML changes: < 1 second (hot reload)
- Plugin changes: ~5 seconds (rebuild)
- No framework overhead, no restart delays

---

## Real-World Examples: Production Flows

### Example 1: E-Commerce Order Processing
**Showcases**: Redis caching, PostgreSQL, logging, error handling, conditional execution

```yaml
id: order_processing
entrypoint:
  type: http
  config:
    method: post
    path: /orders/:orderId/process

properties:
  cachePrefix: "order:"
  cacheTTL: 3600
  minOrderAmount: 10

steps:
  # 1. Log incoming request
  - id: logStart
    type: logger.info
    args:
      message: "Processing order"
      fields:
        orderId: ${request.params.orderId}
        userId: ${request.body.userId}
        amount: ${request.body.amount}

  # 2. Validate minimum order amount
  - id: validateAmount
    type: assign
    condition: request.body.amount < properties.minOrderAmount
    args:
      error: "Order amount below minimum"

  # 3. Check cache for existing order (with error handling)
  - id: checkCache
    type: redis.get
    args:
      key: "${properties.cachePrefix}${request.params.orderId}"
    continueOnError: true  # Continue if Redis is down

  # 4. Return cached result if available
  - id: returnCached
    condition: checkCache != null
    type: assign
    args:
      result:
        status: "cached"
        order: ${checkCache}
        cached: true

  # 5. Fetch from database if not cached
  - id: fetchOrder
    condition: checkCache == null
    type: postgres.queryRow
    args:
      query: |
        SELECT id, user_id, total, status, created_at
        FROM orders
        WHERE id = $1
      params: [${request.params.orderId}]

  # 6. Update order status
  - id: updateOrder
    condition: checkCache == null
    type: postgres.execute
    args:
      query: |
        UPDATE orders
        SET status = $1, processed_at = NOW()
        WHERE id = $2
      params: ["processed", ${request.params.orderId}]

  # 7. Cache the result
  - id: cacheOrder
    condition: checkCache == null
    type: redis.set
    args:
      key: "${properties.cachePrefix}${request.params.orderId}"
      value: ${fetchOrder}
      ttl: ${properties.cacheTTL}

  # 8. Log success
  - id: logSuccess
    type: logger.info
    args:
      message: "Order processed successfully"
      fields:
        orderId: ${request.params.orderId}
        source: ${checkCache != null ? "cache" : "database"}
        status: "processed"

return:
  type: http.response
  args:
    status: 200
    body:
      success: true
      orderId: ${request.params.orderId}
      order: ${checkCache != null ? checkCache : fetchOrder}
      cached: ${checkCache != null}

# Flow-level error handling
errorResponse:
  status: 500
  body:
    success: false
    error: ${error.message}
    requestId: ${execution.id}
```

---

### Example 2: Batch Notification Service
**Showcases**: Foreach loops, flow composition, retry logic, switch branching

```yaml
id: batch_notifications
entrypoint:
  type: http
  config:
    method: post
    path: /notifications/send-batch

properties:
  smsServiceUrl: "${SMS_SERVICE_URL}"
  emailServiceUrl: "${EMAIL_SERVICE_URL}"
  pushServiceUrl: "${PUSH_SERVICE_URL}"

steps:
  # 1. Validate batch size
  - id: validateBatch
    type: assign
    args:
      batchSize: ${len(request.body.notifications)}
      valid: ${batchSize > 0 && batchSize <= 100}

  # 2. Log batch start
  - id: logBatchStart
    type: logger.info
    args:
      message: "Processing notification batch"
      fields:
        batchSize: ${validateBatch.batchSize}
        campaignId: ${request.body.campaignId}

  # 3. Process each notification (with error resilience)
  - id: processNotifications
    type: foreach
    items: ${request.body.notifications}
    continueOnError: true  # Don't fail entire batch if one fails
    step:
      id: sendNotification
      type: flow.call
      flow: notification_sender
      entrypoint: send
      args:
        userId: ${item.userId}
        channel: ${item.channel}
        message: ${item.message}

  # 4. Aggregate results
  - id: aggregateResults
    type: assign
    args:
      totalSent: ${len(processNotifications)}
      successCount: ${processNotifications.filter(r => r.success).len()}
      failureCount: ${processNotifications.filter(r => !r.success).len()}

  # 5. Log completion
  - id: logBatchComplete
    type: logger.info
    args:
      message: "Batch processing complete"
      fields:
        campaignId: ${request.body.campaignId}
        totalSent: ${aggregateResults.totalSent}
        successCount: ${aggregateResults.successCount}
        failureCount: ${aggregateResults.failureCount}

return:
  type: http.response
  args:
    status: 200
    body:
      success: true
      campaignId: ${request.body.campaignId}
      results:
        totalSent: ${aggregateResults.totalSent}
        successCount: ${aggregateResults.successCount}
        failureCount: ${aggregateResults.failureCount}
      details: ${processNotifications}
```

```yaml
# notification_sender.yaml - Reusable flow
id: notification_sender
entrypoint:
  type: function
  id: send

steps:
  # 1. Fetch user preferences
  - id: getPreference
    type: postgres.queryRow
    args:
      query: "SELECT channel, contact FROM user_preferences WHERE user_id = $1"
      params: [${input.userId}]
    default:  # Fallback if user not found
      channel: "email"
      contact: "support@example.com"

  # 2. Route based on channel
  - type: switch
    args:
      sendEmail: ${getPreference.channel == "email"}
      sendSMS: ${getPreference.channel == "sms"}
      sendPush: ${getPreference.channel == "push"}

  # Email channel
  - id: sendEmail
    condition: ${getPreference.channel == "email"}
    type: http
    args:
      url: ${properties.emailServiceUrl}
      method: post
      body:
        to: ${getPreference.contact}
        subject: "Notification"
        body: ${input.message}
    retry:
      max: 3
      backoff: exponential

  # SMS channel
  - id: sendSMS
    condition: ${getPreference.channel == "sms"}
    type: http
    args:
      url: ${properties.smsServiceUrl}
      method: post
      body:
        to: ${getPreference.contact}
        message: ${input.message}
    retry:
      max: 3
      backoff: exponential

  # Push notification channel
  - id: sendPush
    condition: ${getPreference.channel == "push"}
    type: http
    args:
      url: ${properties.pushServiceUrl}
      method: post
      body:
        userId: ${input.userId}
        message: ${input.message}
    retry:
      max: 2
      backoff: linear

return:
  type: function.return
  args:
    success: true
    channel: ${getPreference.channel}
    userId: ${input.userId}
```

---

### Example 3: API Rate Limiter with Quota Management
**Showcases**: Redis operations, expressions, retry logic, timeouts

```yaml
id: rate_limited_api_proxy
entrypoint:
  type: http
  config:
    method: post
    path: /api/proxy/:endpoint

properties:
  targetApiUrl: "${TARGET_API_URL}"
  quotaLimit: 100
  quotaWindow: 3600  # 1 hour in seconds
  quotaKey: "quota:"

steps:
  # 1. Get current quota usage
  - id: getQuota
    type: redis.get
    args:
      key: "${properties.quotaKey}${request.headers.API-Key}"
    default:  # Initialize if not exists
      remaining: ${properties.quotaLimit}
      resetAt: ${now() + properties.quotaWindow}

  # 2. Check if quota available
  - id: checkQuota
    type: assign
    args:
      remaining: ${getQuota.remaining - 1}
      allowed: ${getQuota.remaining > 0}
      resetAt: ${getQuota.resetAt}

  # 3. Reject if quota exceeded
  - id: rejectQuotaExceeded
    condition: ${!checkQuota.allowed}
    type: assign
    args:
      error: "Rate limit exceeded"
      retryAfter: ${checkQuota.resetAt - now()}

  # 4. Call target API (only if quota available)
  - id: callTargetAPI
    condition: ${checkQuota.allowed}
    type: http
    args:
      url: "${properties.targetApiUrl}/${request.params.endpoint}"
      method: ${request.method}
      headers:
        Authorization: ${request.headers.Authorization}
      body: ${request.body}
    retry:
      max: 3
      backoff: exponential
      condition: ${callTargetAPI.result.status >= 500}
    timeout: 5s

  # 5. Update quota (decrement)
  - id: updateQuota
    condition: ${checkQuota.allowed}
    type: redis.set
    args:
      key: "${properties.quotaKey}${request.headers.API-Key}"
      value:
        remaining: ${checkQuota.remaining}
        resetAt: ${checkQuota.resetAt}
      ttl: ${properties.quotaWindow}

  # 6. Log usage
  - id: logUsage
    type: logger.info
    args:
      message: "API proxy request"
      fields:
        endpoint: ${request.params.endpoint}
        quotaRemaining: ${checkQuota.remaining}
        allowed: ${checkQuota.allowed}

return:
  type: http.response
  args:
    status: ${checkQuota.allowed ? callTargetAPI.result.status : 429}
    headers:
      X-Rate-Limit-Limit: ${properties.quotaLimit}
      X-Rate-Limit-Remaining: ${checkQuota.remaining}
      X-Rate-Limit-Reset: ${checkQuota.resetAt}
    body: ${checkQuota.allowed ? callTargetAPI.result.body : {error: "Rate limit exceeded", retryAfter: rejectQuotaExceeded.retryAfter}}
```

---

### Example 4: Multi-Stage Order Fulfillment
**Showcases**: Flow composition, complex expressions, switch logic, PostgreSQL transactions

```yaml
id: order_fulfillment
entrypoint:
  type: http
  config:
    method: post
    path: /orders/:orderId/fulfill

properties:
  inventoryServiceUrl: "${INVENTORY_SERVICE_URL}"
  shippingServiceUrl: "${SHIPPING_SERVICE_URL}"
  notificationServiceUrl: "${NOTIFICATION_SERVICE_URL}"

steps:
  # 1. Fetch order details
  - id: fetchOrder
    type: postgres.queryRow
    args:
      query: |
        SELECT id, user_id, items, total, status
        FROM orders
        WHERE id = $1 AND status = 'pending'
      params: [${request.params.orderId}]

  # 2. Validate order exists and is pending
  - id: validateOrder
    type: assign
    args:
      exists: ${fetchOrder != null}
      items: ${fetchOrder.items}
      userId: ${fetchOrder.user_id}

  # 3. Check inventory for all items
  - id: checkInventory
    type: flow.call
    flow: inventory_checker
    entrypoint: checkBatch
    args:
      items: ${validateOrder.items}

  # 4. Determine fulfillment strategy
  - type: switch
    args:
      fullFulfillment: ${checkInventory.allAvailable}
      partialFulfillment: ${checkInventory.someAvailable}
      noFulfillment: ${!checkInventory.anyAvailable}

  # Full fulfillment path
  - id: reserveInventory
    condition: ${checkInventory.allAvailable}
    type: http
    args:
      url: "${properties.inventoryServiceUrl}/reserve"
      method: post
      body:
        orderId: ${request.params.orderId}
        items: ${validateOrder.items}
    retry:
      max: 3
      backoff: exponential

  - id: createShipment
    condition: ${checkInventory.allAvailable && reserveInventory.result.success}
    type: http
    args:
      url: "${properties.shippingServiceUrl}/shipments"
      method: post
      body:
        orderId: ${request.params.orderId}
        items: ${validateOrder.items}
        address: ${request.body.shippingAddress}

  - id: updateOrderFulfilled
    condition: ${checkInventory.allAvailable && createShipment.result.success}
    type: postgres.execute
    args:
      query: |
        UPDATE orders
        SET status = 'fulfilled', fulfilled_at = NOW()
        WHERE id = $1
      params: [${request.params.orderId}]

  # Partial fulfillment path
  - id: handlePartialFulfillment
    condition: ${checkInventory.someAvailable}
    type: flow.call
    flow: partial_fulfillment_handler
    entrypoint: handle
    args:
      orderId: ${request.params.orderId}
      availableItems: ${checkInventory.availableItems}
      unavailableItems: ${checkInventory.unavailableItems}

  # No fulfillment path
  - id: cancelOrder
    condition: ${!checkInventory.anyAvailable}
    type: postgres.execute
    args:
      query: |
        UPDATE orders
        SET status = 'cancelled', cancelled_at = NOW(), cancel_reason = 'out_of_stock'
        WHERE id = $1
      params: [${request.params.orderId}]

  # 5. Send notification
  - id: notifyCustomer
    type: http
    args:
      url: "${properties.notificationServiceUrl}/send"
      method: post
      body:
        userId: ${validateOrder.userId}
        channel: "email"
        template: ${checkInventory.allAvailable ? "order_fulfilled" : (checkInventory.someAvailable ? "order_partial" : "order_cancelled")}
        data:
          orderId: ${request.params.orderId}
          shipmentId: ${createShipment.result.shipmentId}

  # 6. Log fulfillment outcome
  - id: logFulfillment
    type: logger.info
    args:
      message: "Order fulfillment processed"
      fields:
        orderId: ${request.params.orderId}
        status: ${checkInventory.allAvailable ? "fulfilled" : (checkInventory.someAvailable ? "partial" : "cancelled")}
        allAvailable: ${checkInventory.allAvailable}

return:
  type: http.response
  args:
    status: 200
    body:
      success: true
      orderId: ${request.params.orderId}
      status: ${checkInventory.allAvailable ? "fulfilled" : (checkInventory.someAvailable ? "partial" : "cancelled")}
      shipmentId: ${createShipment.result.shipmentId}
      availableItems: ${checkInventory.availableItems}
      unavailableItems: ${checkInventory.unavailableItems}
```

---

### Features Demonstrated Across Examples

| Feature | Example 1 | Example 2 | Example 3 | Example 4 |
|---------|-----------|-----------|-----------|-----------|
| **Redis Plugin** | ✅ Caching | - | ✅ Quota | - |
| **PostgreSQL Plugin** | ✅ CRUD | ✅ Preferences | - | ✅ Transactions |
| **Logging Plugin** | ✅ Tracing | ✅ Batch logs | ✅ Usage logs | ✅ Outcomes |
| **HTTP Requests** | - | ✅ 3rd party APIs | ✅ Proxy | ✅ Microservices |
| **Foreach Loops** | - | ✅ Batch processing | - | - |
| **Flow Composition** | - | ✅ Reusable sender | - | ✅ Inventory checker |
| **Conditional Execution** | ✅ Cache check | ✅ Channel routing | ✅ Quota check | ✅ Fulfillment paths |
| **Error Handling** | ✅ continueOnError | ✅ Partial failures | - | - |
| **Default Fallbacks** | - | ✅ User preferences | ✅ Quota init | - |
| **Retry Logic** | - | ✅ Network calls | ✅ Exponential backoff | ✅ Inventory reserve |
| **Timeouts** | - | - | ✅ 5s timeout | - |
| **Switch Branching** | - | ✅ Channel routing | - | ✅ Fulfillment strategy |
| **Expression Engine** | ✅ Simple | ✅ Array operations | ✅ Complex | ✅ Ternary |
| **Environment Variables** | ✅ Config | ✅ Service URLs | ✅ API URL | ✅ Service URLs |

---

## Future Vision

### Phase 1: Current (Core Runtime) ✅
- YAML flow definitions
- Expression engine
- HTTP entrypoints
- Retry logic
- Basic task library (http, assign, switch)

### Phase 2: Enhanced Developer Experience
- **SFlowG CLI Tool**
  ```bash
  sflowg init my-service
  sflowg add task custom-validator --lang go
  sflowg test flows/payment.yaml
  sflowg deploy --env staging
  ```

- **Flow Templates Library**
  ```bash
  sflowg template list
  sflowg template use webhook-handler
  sflowg template use scheduled-job
  ```

- **Hot Reloading**
  - Watch YAML files for changes
  - Reload flows without downtime
  - Zero-downtime deployment

### Phase 3: Advanced Task Library
- **Validation Tasks**: validate, sanitize, transform
- **Data Tasks**: database, cache, queue
- **Integration Tasks**: auth, template, notify
- **Business Logic Tasks**: calculate, aggregate, rate-limit

### Phase 4: Multi-Trigger Support
```yaml
# Same flow, different triggers
id: payment_flow

# Trigger 1: HTTP
entrypoint:
  type: http
  config: {method: post, path: /purchase}

# Trigger 2: Kafka (future)
# entrypoint:
#   type: kafka
#   config: {topic: payment-events}

# Trigger 3: Scheduled (future)
# entrypoint:
#   type: cron
#   config: {schedule: "0 0 * * *"}
```

### Phase 5: Orchestration Layer (Distant Future)

**Current**: SFlowG builds individual microservices
**Future**: SFlowG orchestrates services built with SFlowG

```yaml
# Service orchestration flow (future idea)
id: order_saga
steps:
  - id: createOrder
    type: sflowg-service
    args: {service: order-service, endpoint: /create}

  - id: reserveInventory
    type: sflowg-service
    args: {service: inventory-service, endpoint: /reserve}
    condition: createOrder.result.status == 200

  - id: compensateOrder
    type: sflowg-service
    args: {service: order-service, endpoint: /cancel}
    condition: reserveInventory.result.status != 200
```

**Inspiration**: Temporal, Kestra (but for SFlowG services specifically)

### Phase 6: Ecosystem & Community
- **Flow Marketplace**
  - Community-contributed flows
  - Verified integration templates
  - Best practice patterns

- **Task Plugin Registry**
  - Third-party task implementations
  - Framework integrations (Stripe, Twilio, Auth0)
  - Domain-specific task libraries

- **Migration Tools**
  - AI-assisted flow → microservice conversion
  - Testing frameworks for contract validation
  - Performance profiling for optimization decisions

---

## Success Metrics

A successful SFlowG implementation enables:

**Speed Metrics:**
- ⏱️ Idea to working service: < 1 hour
- ⏱️ YAML change to deployed: < 5 minutes
- ⏱️ Prototype to production decision: hours/days not weeks

**Quality Metrics:**
- 🎯 Production-ready from start (error handling, logging, retry)
- 🎯 Git-trackable flows (code review, version control)
- 🎯 Testable with real HTTP (no mocking required)

**Flexibility Metrics:**
- 🔄 Services staying simple: remain in SFlowG
- 🔄 Services growing complex: easy migration
- 🔄 Failed prototypes: minimal time waste

**Developer Experience:**
- 😊 Minimal learning curve (YAML + expressions)
- 😊 Framework-agnostic (no lock-in)
- 😊 AI-migration friendly (YAML as spec)

---

## The Paradigm Shift

### What Changes with SFlowG

**Before SFlowG:**
```
Simple Service Idea
  ↓ (2-4 weeks)
Framework Setup → Boilerplate → Testing → Deployment
  ↓
Production Service
```

**With SFlowG:**
```
Simple Service Idea
  ↓ (1-4 hours)
YAML Definition → Deploy
  ↓
Production Service ───┬─→ Stays Simple (70%)
                      └─→ Migrates When Complex (30%)
```

### What Becomes Possible

✨ **Services as Configuration**: Business logic as data, not code
✨ **Zero-Regret Prototyping**: Hours invested, not weeks wasted
✨ **No Premature Optimization**: Start simple, migrate when proven
✨ **Developer Velocity**: 10x faster for simple services
✨ **AI-Ready Migration**: YAML = perfect specification

---

## The Core Promise

> **SFlowG gets you from idea to working service in hours.**
>
> **For simple services**: It's your permanent runtime that evolves with YAML.
> **For complex services**: It's your prototype that proves the concept.
> **For failed experiments**: It's minimal time wasted.
>
> **You're not locked in. YAML is your spec, your docs, and your migration guide.**

---

## Positioning Statement

**For technical developers** who need to build services quickly without framework boilerplate,

**SFlowG** is a declarative service construction framework

**That** enables idea-to-production in hours through developer-friendly YAML and expression-driven logic,

**Unlike** n8n (visual workflows for non-developers) or traditional microservice frameworks (weeks of setup),

**SFlowG** provides the missing middle: developer power with configuration simplicity, production-ready runtime with easy migration when services grow complex.

---

*From idea to production in hours. From simple to complex without regret. From YAML to microservice with AI assistance.*

**SFlowG: The Missing Middle for Service Development**
