package dsl

import (
	"fmt"
	"strings"

	"github.com/BDNK1/sflowg/runtime"
)

// Parse parses a .flow DSL source into a runtime.Flow.
//
// DSL syntax supports four top-level block types:
//
//	entrypoint.http { method: POST, path: /api/payments, ... }
//	properties { key: value, ... }
//	step step_name(condition: expr, retry: { ... }) { risor code }
//	return response.json({ ... })
func Parse(source string) (runtime.Flow, error) {
	p := &parser{source: source, pos: 0}
	return p.parse()
}

type parser struct {
	source string
	pos    int
}

func (p *parser) parse() (runtime.Flow, error) {
	var flow runtime.Flow

	p.skipWhitespaceAndComments()
	for p.pos < len(p.source) {
		keyword := p.peekKeyword()

		switch {
		case strings.HasPrefix(keyword, "entrypoint"):
			ep, err := p.parseEntrypoint()
			if err != nil {
				return flow, fmt.Errorf("parsing entrypoint: %w", err)
			}
			flow.Entrypoint = ep

		case keyword == "properties":
			props, err := p.parseProperties()
			if err != nil {
				return flow, fmt.Errorf("parsing properties: %w", err)
			}
			flow.Properties = props

		case keyword == "step":
			step, err := p.parseStep()
			if err != nil {
				return flow, fmt.Errorf("parsing step: %w", err)
			}
			flow.Steps = append(flow.Steps, step)

		case keyword == "return":
			ret, err := p.parseReturn()
			if err != nil {
				return flow, fmt.Errorf("parsing return: %w", err)
			}
			flow.Return = ret

		default:
			if p.pos < len(p.source) {
				return flow, fmt.Errorf("unexpected token at position %d: %q", p.pos, p.source[p.pos:min(p.pos+20, len(p.source))])
			}
		}

		p.skipWhitespaceAndComments()
	}

	return flow, nil
}

// peekKeyword reads the next word without advancing position.
func (p *parser) peekKeyword() string {
	start := p.pos
	for start < len(p.source) && !isWordChar(p.source[start]) {
		start++
	}
	end := start
	for end < len(p.source) && (isWordChar(p.source[end]) || p.source[end] == '.') {
		end++
	}
	return p.source[start:end]
}

// readWord reads the next word and advances position.
func (p *parser) readWord() string {
	p.skipWhitespace()
	start := p.pos
	for p.pos < len(p.source) && (isWordChar(p.source[p.pos]) || p.source[p.pos] == '.') {
		p.pos++
	}
	return p.source[start:p.pos]
}

// parseEntrypoint parses: entrypoint.http { ... }
func (p *parser) parseEntrypoint() (runtime.Entrypoint, error) {
	word := p.readWord() // "entrypoint.http"
	parts := strings.SplitN(word, ".", 2)
	if len(parts) != 2 {
		return runtime.Entrypoint{}, fmt.Errorf("expected entrypoint.TYPE, got %q", word)
	}
	epType := parts[1]

	p.skipWhitespace()
	body, err := p.readBracedBlock()
	if err != nil {
		return runtime.Entrypoint{}, err
	}

	config, err := parseSimpleMap(body)
	if err != nil {
		return runtime.Entrypoint{}, fmt.Errorf("parsing entrypoint config: %w", err)
	}

	return runtime.Entrypoint{
		Type:   epType,
		Config: config,
	}, nil
}

// parseProperties parses: properties { key: value, ... }
func (p *parser) parseProperties() (map[string]any, error) {
	p.readWord() // consume "properties"
	p.skipWhitespace()

	body, err := p.readBracedBlock()
	if err != nil {
		return nil, err
	}

	props, err := parseSimpleMap(body)
	if err != nil {
		return nil, fmt.Errorf("parsing properties: %w", err)
	}

	// Resolve env() calls in property values
	for k, v := range props {
		if s, ok := v.(string); ok {
			props[k] = resolveEnvCall(s)
		}
	}

	return props, nil
}

// parseStep parses: step NAME(condition: ..., retry: {...}) { body }
func (p *parser) parseStep() (runtime.Step, error) {
	p.readWord() // consume "step"
	p.skipWhitespace()

	// Read step name
	name := p.readStepName()
	if name == "" {
		return runtime.Step{}, fmt.Errorf("expected step name")
	}

	var step runtime.Step
	step.ID = name

	p.skipWhitespace()

	// Optional parenthesized options: (condition: ..., retry: {...})
	if p.pos < len(p.source) && p.source[p.pos] == '(' {
		opts, err := p.readParenBlock()
		if err != nil {
			return step, fmt.Errorf("parsing step options: %w", err)
		}
		if err := applyStepOptions(&step, opts); err != nil {
			return step, err
		}
	}

	p.skipWhitespace()

	// Read the step body
	body, err := p.readBracedBlock()
	if err != nil {
		return step, fmt.Errorf("parsing step body: %w", err)
	}
	step.Body = body

	return step, nil
}

// parseReturn parses: return <expression>
// Everything after "return " until end of meaningful content is the return body.
func (p *parser) parseReturn() (runtime.Return, error) {
	p.readWord() // consume "return"
	p.skipWhitespace()

	// Read the rest as the return expression (until next top-level keyword or EOF)
	start := p.pos
	// The return body could be a function call like response.json({...})
	// We need to handle nested braces in the expression
	depth := 0
	inString := false
	stringChar := byte(0)

	for p.pos < len(p.source) {
		ch := p.source[p.pos]

		if inString {
			if ch == '\\' {
				p.pos++ // skip escape
			} else if ch == stringChar {
				inString = false
			}
			p.pos++
			continue
		}

		if ch == '"' || ch == '\'' || ch == '`' {
			inString = true
			stringChar = ch
			p.pos++
			continue
		}

		if ch == '(' || ch == '{' || ch == '[' {
			depth++
		} else if ch == ')' || ch == '}' || ch == ']' {
			depth--
			if depth < 0 {
				break
			}
		} else if depth == 0 && ch == '\n' {
			// Check if next non-whitespace is a top-level keyword
			saved := p.pos
			p.pos++
			p.skipWhitespace()
			if p.pos >= len(p.source) {
				break
			}
			next := p.peekKeyword()
			if next == "step" || next == "return" || next == "properties" || strings.HasPrefix(next, "entrypoint") {
				p.pos = saved
				break
			}
			continue
		}

		p.pos++
	}

	body := strings.TrimSpace(p.source[start:p.pos])
	return runtime.Return{Body: body}, nil
}

// readBracedBlock reads content between { and }, handling nested braces and strings.
func (p *parser) readBracedBlock() (string, error) {
	if p.pos >= len(p.source) || p.source[p.pos] != '{' {
		return "", fmt.Errorf("expected '{' at position %d", p.pos)
	}
	p.pos++ // skip opening {

	start := p.pos
	depth := 1
	inString := false
	stringChar := byte(0)

	for p.pos < len(p.source) && depth > 0 {
		ch := p.source[p.pos]

		if inString {
			if ch == '\\' {
				p.pos++ // skip escape
			} else if ch == stringChar {
				inString = false
			}
			p.pos++
			continue
		}

		switch ch {
		case '"', '\'', '`':
			inString = true
			stringChar = ch
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				body := p.source[start:p.pos]
				p.pos++ // skip closing }
				return strings.TrimSpace(body), nil
			}
		case '/':
			// Handle // line comments
			if p.pos+1 < len(p.source) && p.source[p.pos+1] == '/' {
				for p.pos < len(p.source) && p.source[p.pos] != '\n' {
					p.pos++
				}
				continue
			}
		}

		p.pos++
	}

	return "", fmt.Errorf("unclosed brace block starting at position %d", start)
}

// readParenBlock reads content between ( and ).
func (p *parser) readParenBlock() (string, error) {
	if p.pos >= len(p.source) || p.source[p.pos] != '(' {
		return "", fmt.Errorf("expected '(' at position %d", p.pos)
	}
	p.pos++ // skip opening (

	start := p.pos
	depth := 1
	inString := false
	stringChar := byte(0)

	for p.pos < len(p.source) && depth > 0 {
		ch := p.source[p.pos]

		if inString {
			if ch == '\\' {
				p.pos++
			} else if ch == stringChar {
				inString = false
			}
			p.pos++
			continue
		}

		switch ch {
		case '"', '\'', '`':
			inString = true
			stringChar = ch
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				body := p.source[start:p.pos]
				p.pos++ // skip closing )
				return strings.TrimSpace(body), nil
			}
		}

		p.pos++
	}

	return "", fmt.Errorf("unclosed paren block starting at position %d", start)
}

func (p *parser) readStepName() string {
	start := p.pos
	for p.pos < len(p.source) && (isWordChar(p.source[p.pos]) || p.source[p.pos] == '_') {
		p.pos++
	}
	return p.source[start:p.pos]
}

func (p *parser) skipWhitespace() {
	for p.pos < len(p.source) && (p.source[p.pos] == ' ' || p.source[p.pos] == '\t' || p.source[p.pos] == '\n' || p.source[p.pos] == '\r') {
		p.pos++
	}
}

func (p *parser) skipWhitespaceAndComments() {
	for {
		p.skipWhitespace()
		if p.pos+1 < len(p.source) && p.source[p.pos] == '/' && p.source[p.pos+1] == '/' {
			// Skip line comment
			for p.pos < len(p.source) && p.source[p.pos] != '\n' {
				p.pos++
			}
			continue
		}
		break
	}
}

func isWordChar(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_'
}

// parseSimpleMap parses a simple key: value map from block contents.
// Supports string values (quoted and unquoted), arrays [...], and nested maps {...}.
func parseSimpleMap(block string) (map[string]any, error) {
	result := make(map[string]any)
	block = strings.TrimSpace(block)
	if block == "" {
		return result, nil
	}

	i := 0
	for i < len(block) {
		// Skip whitespace and commas
		for i < len(block) && (block[i] == ' ' || block[i] == '\t' || block[i] == '\n' || block[i] == '\r' || block[i] == ',') {
			i++
		}
		if i >= len(block) {
			break
		}

		// Skip comments
		if i+1 < len(block) && block[i] == '/' && block[i+1] == '/' {
			for i < len(block) && block[i] != '\n' {
				i++
			}
			continue
		}

		// Read key
		keyStart := i
		for i < len(block) && block[i] != ':' && block[i] != ' ' && block[i] != '\n' {
			i++
		}
		key := strings.TrimSpace(block[keyStart:i])
		if key == "" {
			break
		}

		// Skip to colon
		for i < len(block) && block[i] != ':' {
			i++
		}
		if i >= len(block) {
			return nil, fmt.Errorf("expected ':' after key %q", key)
		}
		i++ // skip colon

		// Skip whitespace
		for i < len(block) && (block[i] == ' ' || block[i] == '\t') {
			i++
		}

		// Read value
		value, newPos, err := readValue(block, i)
		if err != nil {
			return nil, fmt.Errorf("reading value for key %q: %w", key, err)
		}
		i = newPos

		result[key] = value
	}

	return result, nil
}

// readValue reads a value starting at position i in the block string.
// Returns the parsed value and the new position after the value.
func readValue(block string, i int) (any, int, error) {
	if i >= len(block) {
		return "", i, nil
	}

	ch := block[i]

	// Quoted string
	if ch == '"' || ch == '\'' {
		return readQuotedString(block, i)
	}

	// Array
	if ch == '[' {
		return readArray(block, i)
	}

	// Nested map
	if ch == '{' {
		return readNestedMap(block, i)
	}

	// Unquoted value — read until comma, newline, or closing delimiter
	start := i
	for i < len(block) && block[i] != ',' && block[i] != '\n' && block[i] != '}' && block[i] != ']' {
		i++
	}
	return strings.TrimSpace(block[start:i]), i, nil
}

func readQuotedString(block string, i int) (string, int, error) {
	quote := block[i]
	i++ // skip opening quote
	start := i
	for i < len(block) {
		if block[i] == '\\' {
			i += 2
			continue
		}
		if block[i] == quote {
			s := block[start:i]
			i++ // skip closing quote
			return s, i, nil
		}
		i++
	}
	return "", i, fmt.Errorf("unclosed string starting at position %d", start-1)
}

func readArray(block string, i int) ([]any, int, error) {
	i++ // skip [
	var items []any
	for i < len(block) {
		// Skip whitespace and commas
		for i < len(block) && (block[i] == ' ' || block[i] == '\t' || block[i] == '\n' || block[i] == '\r' || block[i] == ',') {
			i++
		}
		if i >= len(block) || block[i] == ']' {
			i++ // skip ]
			return items, i, nil
		}

		val, newPos, err := readValue(block, i)
		if err != nil {
			return nil, newPos, err
		}
		items = append(items, val)
		i = newPos
	}
	return nil, i, fmt.Errorf("unclosed array")
}

func readNestedMap(block string, i int) (map[string]any, int, error) {
	// Find matching closing brace
	start := i
	i++ // skip {
	depth := 1
	inStr := false
	strChar := byte(0)

	for i < len(block) && depth > 0 {
		ch := block[i]
		if inStr {
			if ch == '\\' {
				i++
			} else if ch == strChar {
				inStr = false
			}
			i++
			continue
		}
		switch ch {
		case '"', '\'':
			inStr = true
			strChar = ch
		case '{':
			depth++
		case '}':
			depth--
		}
		i++
	}

	inner := block[start+1 : i-1]
	m, err := parseSimpleMap(inner)
	if err != nil {
		return nil, i, err
	}
	return m, i, nil
}

// resolveEnvCall resolves env("VAR") or env("VAR", "default") patterns in property values.
func resolveEnvCall(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "env(") || !strings.HasSuffix(s, ")") {
		return s
	}
	// Convert env("VAR") to ${VAR} and env("VAR", "default") to ${VAR:default}
	inner := strings.TrimPrefix(s, "env(")
	inner = strings.TrimSuffix(inner, ")")
	inner = strings.TrimSpace(inner)

	parts := strings.SplitN(inner, ",", 2)
	varName := strings.Trim(strings.TrimSpace(parts[0]), `"'`)

	if len(parts) == 2 {
		defaultVal := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		return fmt.Sprintf("${%s:%s}", varName, defaultVal)
	}

	return fmt.Sprintf("${%s}", varName)
}

// applyStepOptions parses the parenthesized options string and applies to step.
func applyStepOptions(step *runtime.Step, opts string) error {
	m, err := parseSimpleMap(opts)
	if err != nil {
		return fmt.Errorf("parsing step options: %w", err)
	}

	if cond, ok := m["condition"]; ok {
		step.Condition = fmt.Sprintf("%v", cond)
	}

	if retryRaw, ok := m["retry"]; ok {
		retryMap, ok := retryRaw.(map[string]any)
		if !ok {
			return fmt.Errorf("retry must be a map")
		}
		step.Retry = &runtime.RetryConfig{}
		if v, ok := retryMap["maxRetries"]; ok {
			step.Retry.MaxRetries = toInt(v)
		}
		if v, ok := retryMap["delay"]; ok {
			step.Retry.Delay = toInt(v)
		}
		if v, ok := retryMap["backoff"]; ok {
			step.Retry.Backoff = toBool(v)
		}
		if v, ok := retryMap["condition"]; ok {
			step.Retry.Condition = fmt.Sprintf("%v", v)
		}
	}

	return nil
}

func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case string:
		var i int
		fmt.Sscanf(n, "%d", &i)
		return i
	default:
		return 0
	}
}

func toBool(v any) bool {
	switch b := v.(type) {
	case bool:
		return b
	case string:
		return b == "true"
	default:
		return false
	}
}
