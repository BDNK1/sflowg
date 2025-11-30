# SFlowG Payment System Integration - Test Features

This document outlines all features and subfeatures that need to be tested in the payment-system-integration example.

## 1. Plugin System

### 1.1 Plugin Types
- [x] **Core Plugin** - Using HTTP plugin from `github.com/sflowg/sflowg/plugins/http`
- [x] **Local Module Plugin** - Custom payment plugin in `./plugins/payment`
- [ ] **Remote Module Plugin** - (Optional: could add example from github.com/org/plugin)

### 1.2 Plugin Structure
- [ ] **Multiple Tasks per Plugin** - Payment plugin with multiple public methods (ValidateCard, ProcessPayment, RefundPayment)
- [ ] **Plugin Config struct** - Config with defaults and validation tags
- [ ] **Shared State** - Plugin instance fields accessible across all tasks

### 1.3 Typed Tasks
- [ ] **Typed Input Structs** - Strongly typed task inputs with validation
- [ ] **Typed Output Structs** - Strongly typed task outputs
- [ ] **Map-based Tasks** - Support for untyped map[string]any tasks
- [ ] **Mixed Signatures** - Both typed and untyped tasks in same plugin

### 1.4 Plugin Lifecycle
- [ ] **Initialize** - Plugin initialization with context and config
- [ ] **Shutdown** - Graceful shutdown and resource cleanup
- [ ] **Initialization Order** - Plugins initialized in dependency order

### 1.5 Plugin Configuration
- [ ] **Config Defaults** - `default:"value"` tags on config fields
- [ ] **Config Validation** - `validate:"required"` and other validation rules
- [ ] **Environment Variable Overrides** - Config values from env vars
- [ ] **Literal Values** - Config values from flow-config.yaml
- [ ] **Complex Types** - time.Duration, int, bool, string fields

### 1.6 Plugin Dependencies
- [ ] **Field Injection** - HTTP plugin injected into payment plugin fields
- [ ] **Dependency Graph** - Correct initialization order based on dependencies
- [ ] **Using Injected Plugins** - Payment plugin calling HTTP plugin tasks

### 1.7 Task Discovery
- [ ] **Auto-discovery** - CLI finds all public methods on plugin
- [ ] **Task Naming** - `plugin_name.method_name` format (e.g., `payment.validateCard`)
- [ ] **Multiple Tasks** - Single plugin with 3+ distinct tasks

### 1.8 Plugin Registration
- [ ] **Container Registration** - Plugins registered in generated main.go
- [ ] **Task Executor Creation** - Each task gets executor wrapper
- [ ] **Optional Interface Detection** - Lifecycle interfaces detected at runtime

## 2. CLI Build System

### 2.1 Configuration Parsing
- [ ] **flow-config.yaml** - Parse plugin sources and configurations
- [ ] **Auto-detection** - Detect plugin types from source format
- [ ] **Version Handling** - Support version specifications (Phase 2)

### 2.2 Code Generation
- [ ] **main.go Generation** - Complete working main.go with all imports
- [ ] **Import Statements** - Correct imports for core and local plugins
- [ ] **Plugin Initialization Code** - Config preparation and validation
- [ ] **Dependency Injection Code** - Field assignments for dependencies
- [ ] **go.mod Generation** - Temp workspace with correct dependencies
- [ ] **Replace Directives** - Local modules use replace directives

### 2.3 Build Process
- [ ] **Temporary Workspace** - Build in temporary directory
- [ ] **Dependency Download** - `go mod download` execution
- [ ] **Binary Compilation** - `go build` successful
- [ ] **Binary Placement** - Output binary in project directory
- [ ] **Cleanup** - Temp workspace removal after build

### 2.4 Validation
- [ ] **Task Existence** - Verify all flow tasks exist in plugins
- [ ] **Dependency Cycles** - Detect circular dependencies
- [ ] **Config Validation** - Validate plugin configs at build time

## 3. HTTP Plugin

### 3.1 Request Configuration
- [ ] **URL** - Full URL support with schemes
- [ ] **HTTP Methods** - GET, POST, PUT, PATCH, DELETE
- [ ] **Headers** - Custom request headers
- [ ] **Query Parameters** - URL query parameters
- [ ] **Request Body** - JSON body support

### 3.2 Response Handling
- [ ] **Status Code** - HTTP status code in response
- [ ] **Response Status** - Status text in response
- [ ] **Response Body** - Parsed JSON response body
- [ ] **Error Responses** - Error body on non-2xx responses
- [ ] **IsError Flag** - Boolean indicating error status

### 3.3 Retry Logic
- [ ] **Max Retries** - Configurable retry count
- [ ] **Retry Delay** - Delay between retries
- [ ] **Exponential Backoff** - Backoff strategy
- [ ] **Retry Condition** - Conditional retry based on response

### 3.4 Timeout
- [ ] **Request Timeout** - Timeout for individual requests
- [ ] **Global Timeout** - Plugin-level timeout configuration

### 3.5 Plugin Config
- [ ] **Timeout Config** - `timeout` field with default
- [ ] **Max Retries Config** - `max_retries` with validation
- [ ] **Debug Mode** - `debug` boolean flag
- [ ] **Retry Wait** - `retry_wait_ms` configuration

## 4. Flow Execution

### 4.1 Variable Resolution
- [ ] **request.body** - Access request body fields
- [ ] **request.headers** - Access request headers
- [ ] **request.pathVariables** - Access path parameters
- [ ] **request.queryParameters** - Access query parameters
- [ ] **properties** - Access static properties from flow
- [ ] **Previous Step Results** - Access results from previous steps
- [ ] **Nested Access** - Dot notation for nested fields (e.g., `request.body.card.number`)

### 4.2 Expressions (expr-lang)
- [ ] **Arithmetic** - Math operations in expressions
- [ ] **String Operations** - String manipulation
- [ ] **Comparisons** - `==`, `!=`, `>`, `<`, `>=`, `<=`
- [ ] **Logical Operators** - `&&`, `||`, `!`
- [ ] **Type Conversions** - Automatic type coercion
- [ ] **Function Calls** - Built-in expression functions
- [ ] **Complex Expressions** - Multi-level nested expressions

### 4.3 Step Types

#### 4.3.1 Assign Step
- [ ] **Simple Assignment** - Assign literal values
- [ ] **Expression Assignment** - Assign computed expressions
- [ ] **Multiple Assignments** - Multiple variables in one step
- [ ] **Reference Resolution** - Assignments using other variables

#### 4.3.2 HTTP Request Step
- [ ] **Basic Request** - Simple HTTP call
- [ ] **Dynamic URL** - URL from variables
- [ ] **Dynamic Headers** - Headers from variables
- [ ] **Dynamic Body** - Body fields from variables
- [ ] **Conditional Execution** - `condition` field on step

#### 4.3.3 Switch Step
- [ ] **Multiple Branches** - Multiple named conditions
- [ ] **First Match Execution** - Execute first true condition
- [ ] **Default Branch** - Fallback branch (true == true)
- [ ] **Boolean Expressions** - Complex condition expressions

#### 4.3.4 Plugin Task Step
- [ ] **Task Invocation** - Call plugin tasks
- [ ] **Input Mapping** - Map flow variables to task inputs
- [ ] **Output Access** - Access task results in subsequent steps
- [ ] **Conditional Execution** - Conditional plugin task calls

### 4.4 Flow Structure
- [ ] **Entrypoint Config** - HTTP entrypoint configuration
- [ ] **Method** - HTTP method (GET, POST, etc.)
- [ ] **Path** - URL path with parameters
- [ ] **Headers** - Required/optional headers
- [ ] **Path Variables** - Dynamic path segments
- [ ] **Query Parameters** - Query parameter definitions
- [ ] **Body Type** - JSON body type

### 4.5 Properties
- [ ] **Static Values** - Hardcoded values in flow
- [ ] **Reference in Steps** - Access via `properties.name`
- [ ] **Multiple Properties** - Several properties defined
- [ ] **Different Types** - String, number, boolean properties

### 4.6 Return Configuration
- [ ] **Response Type** - http.response type
- [ ] **Status Code** - Configurable response status
- [ ] **Response Body** - Body with variable substitution
- [ ] **Nested Response** - Complex nested response structure

### 4.7 Step Execution
- [ ] **Sequential Execution** - Steps run in order
- [ ] **Conditional Skip** - Steps skipped when condition false
- [ ] **Error Handling** - Errors propagate correctly
- [ ] **Result Accumulation** - Results available to later steps

## 5. Retry and Timeout

### 5.1 Step-level Retry
- [ ] **maxRetries** - Maximum retry attempts configuration
- [ ] **delay** - Delay between retries in milliseconds
- [ ] **backoff** - Exponential backoff enabled/disabled
- [ ] **condition** - Expression to determine if retry needed

### 5.2 Timeout
- [ ] **Step Timeout** - Individual step timeout (via HTTP plugin config)
- [ ] **Global Timeout** - Overall execution timeout (context)

### 5.3 Error Scenarios
- [ ] **Retry on Error** - Automatic retry on failures
- [ ] **Max Retries Exhausted** - Fail after max retries
- [ ] **Successful Retry** - Succeed after initial failure
- [ ] **Conditional Retry** - Only retry when condition met

## 6. Conditional Logic (If/Switch)

### 6.1 Switch Step
- [ ] **Named Branches** - Multiple labeled branches
- [ ] **Expression Evaluation** - Boolean expressions per branch
- [ ] **Branch Execution** - Correct branch executed
- [ ] **Default Fallback** - Default branch when no match
- [ ] **Access to Variables** - Conditions use flow state

### 6.2 Step Conditions
- [ ] **condition Field** - Condition on any step
- [ ] **Boolean Expression** - Expression evaluates to bool
- [ ] **Conditional Skip** - Step skipped if false
- [ ] **Multiple Conditional Steps** - Several conditional steps in flow

### 6.3 Complex Conditions
- [ ] **Comparisons** - Numeric and string comparisons
- [ ] **Logical AND/OR** - Combined conditions
- [ ] **Nested Conditions** - Complex nested logic
- [ ] **Previous Step Results** - Conditions based on prior results

## 7. Assignment Operations

### 7.1 Variable Creation
- [ ] **New Variables** - Create new flow variables
- [ ] **Multiple Variables** - Several variables in one assign step
- [ ] **Variable Scope** - Variables accessible in later steps

### 7.2 Value Types
- [ ] **Literals** - Direct literal values
- [ ] **Expressions** - Computed values from expressions
- [ ] **References** - Values from other variables
- [ ] **Mixed Types** - String, number, boolean, object values

### 7.3 Expression Assignment
- [ ] **Arithmetic** - Math expressions assigned to variables
- [ ] **String Concatenation** - String manipulation assignments
- [ ] **Conditional Expressions** - Ternary-like assignments
- [ ] **Function Calls** - Built-in functions in assignments

## 8. Lifecycle Management

### 8.1 Initialization
- [ ] **Plugin Initialize** - Initialize method called
- [ ] **Context Access** - Execution context available
- [ ] **Config Available** - Validated config accessible
- [ ] **Setup Resources** - Initialize connections, clients, etc.
- [ ] **Error Handling** - Initialization errors fail startup

### 8.2 Shutdown
- [ ] **Plugin Shutdown** - Shutdown method called
- [ ] **Context Access** - Execution context available
- [ ] **Cleanup Resources** - Close connections, cleanup, etc.
- [ ] **Error Handling** - Shutdown errors logged
- [ ] **Graceful Shutdown** - Clean shutdown on SIGTERM/SIGINT

### 8.3 Lifecycle Order
- [ ] **Dependency Order** - Initialize respects dependencies
- [ ] **Reverse Shutdown** - Shutdown in reverse order
- [ ] **Single Call** - Initialize/Shutdown called once per plugin

## 9. Container Management

### 9.1 Plugin Registration
- [ ] **RegisterPlugin** - Plugins registered correctly
- [ ] **Task Discovery** - All public methods discovered
- [ ] **Task Executors** - Executors created for each task
- [ ] **Interface Detection** - Optional interfaces detected

### 9.2 Task Execution
- [ ] **GetTask** - Retrieve task executor by name
- [ ] **Execute Task** - Execute task with input
- [ ] **Error Propagation** - Task errors handled correctly
- [ ] **Result Return** - Task results available to flow

### 9.3 Lifecycle Management
- [ ] **Initialize All** - All plugins initialized
- [ ] **Shutdown All** - All plugins shut down
- [ ] **Fail Fast** - Startup fails on init error

## 10. Integration Tests

### 10.1 End-to-End Flow
- [ ] **Complete Flow** - Full payment flow executes
- [ ] **All Features Used** - Every feature tested in flow
- [ ] **Success Scenario** - Happy path works
- [ ] **Error Scenarios** - Error handling works

### 10.2 Build and Run
- [ ] **CLI Build** - `sflowg build` succeeds
- [ ] **Binary Execution** - Generated binary runs
- [ ] **HTTP Requests** - Flow responds to HTTP requests
- [ ] **Flow Registration** - Flow loaded and registered

### 10.3 Manual Testing
- [ ] **HTTP File** - Test requests in .http file
- [ ] **Multiple Scenarios** - Different test cases
- [ ] **Success Cases** - Valid requests work
- [ ] **Error Cases** - Invalid requests handled

## Testing Coverage Summary

| Category | Subfeatures | Status |
|----------|-------------|--------|
| Plugin System | 8 subcategories, ~35 features | 🟡 Partial |
| CLI Build | 4 subcategories, ~15 features | 🟡 Partial |
| HTTP Plugin | 5 subcategories, ~15 features | 🟡 Partial |
| Flow Execution | 7 subcategories, ~30 features | 🟡 Partial |
| Retry/Timeout | 3 subcategories, ~10 features | 🟡 Partial |
| Conditional Logic | 3 subcategories, ~10 features | 🟡 Partial |
| Assignment | 3 subcategories, ~10 features | 🟡 Partial |
| Lifecycle | 3 subcategories, ~10 features | 🟡 Partial |
| Container | 3 subcategories, ~10 features | 🟡 Partial |
| Integration | 3 subcategories, ~10 features | ❌ Not Started |

**Total Features to Test: ~155 features across 10 major categories**

## Priority Features (Must Test)

### P0 - Critical (Must Work)
1. Plugin system basics (local plugin, config, lifecycle)
2. HTTP plugin (request, response, headers)
3. Variable resolution (request, properties, previous steps)
4. Expressions in conditions and assignments
5. Assign step
6. Switch step
7. CLI build and execution

### P1 - Important (Should Work)
1. Retry logic with condition
2. Timeout configuration
3. Multiple tasks per plugin
4. Typed task inputs/outputs
5. Plugin dependencies (field injection)
6. Conditional step execution
7. Complex expressions

### P2 - Nice to Have (Can Defer)
1. Remote module plugins
2. Advanced error scenarios
3. Performance optimization
4. Edge cases and corner cases

## Test Execution Plan

1. **Create Payment Plugin** - Local module with all features
2. **Create Comprehensive Flow** - Tests all P0 and P1 features
3. **Update flow-config.yaml** - Proper plugin configuration
4. **Build with CLI** - Verify generation works
5. **Run and Test** - Manual HTTP testing
6. **Document Results** - Mark features as tested
