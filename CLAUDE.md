# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

SFlowG is a workflow execution engine written in Go that processes YAML-defined flows. It's designed to handle HTTP-triggered workflows with steps including assignments, HTTP requests, conditional branching, and retries.

## Architecture

### Core Components

- **App** (`sflowg/app.go`): Main application that loads flows from YAML files and manages the container
- **Flow** (`sflowg/components.go`): YAML-defined workflow structure with entrypoint, steps, properties, and return configuration
- **Executor** (`sflowg/executor.go`): Executes flow steps sequentially with support for conditions, retries, and branching
- **Execution** (`sflowg/execution.go`): Runtime context that implements `context.Context` and stores execution state
- **Container** (`sflowg/container.go`): Dependency injection container for tasks and HTTP client
- **HttpHandler** (`sflowg/http_handler.go`): Gin-based HTTP server that triggers flow execution

### Flow Structure

Flows are defined in YAML files in the `flows/` directory with this structure:
- **entrypoint**: HTTP configuration (method, path, headers, parameters, body)
- **properties**: Static values available during execution
- **steps**: Sequence of operations (assign, http, switch types)
- **return**: Response configuration

### Step Types

- **assign**: Variable assignment with expression evaluation
- **http**: HTTP request execution with retry support
- **switch**: Conditional branching based on boolean expressions

### Expression System

The engine uses the `expr-lang/expr` library for dynamic expression evaluation in conditions, assignments, and retry logic. Variables are accessible via dot notation (e.g., `request.body.amount`, `properties.paymentServiceUrl`).

## Development Commands

### Build and Run
```bash
go build -o sflowg .
./sflowg
```

### Start Development Server
```bash
go run main.go
```
Server runs on port 8080 by default.

### Testing
```bash
go test ./...
go test ./sflowg -v
```

### Dependencies
```bash
go mod download
go mod tidy
```

## Key Dependencies

- **gin-gonic/gin**: HTTP web framework
- **expr-lang/expr**: Expression evaluation engine
- **go-resty/resty**: HTTP client for outbound requests  
- **Jeffail/gabs**: JSON manipulation and flattening
- **google/uuid**: UUID generation for execution tracking
- **gopkg.in/yaml.v3**: YAML parsing for flow definitions

## Flow Development

1. Create YAML flow files in the `flows/` directory
2. Define HTTP entrypoint configuration
3. Add properties for static values
4. Create sequential steps with appropriate types
5. Configure return response structure
6. Test flows by making HTTP requests to the defined endpoints

The engine automatically registers all `*.yaml` files from the flows directory on startup.