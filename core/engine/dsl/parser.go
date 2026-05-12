package dsl

import (
	"fmt"
	"strings"

	"github.com/BDNK1/sflowg/core"
	"github.com/BDNK1/sflowg/core/validation/flowinput"
	"github.com/BDNK1/sflowg/core/validation/httpinput"
	"github.com/BDNK1/sflowg/core/validation/kafkainput"
)

// Parse parses a .flow DSL source into a core.Flow.
//
// DSL syntax supports these top-level block types:
//
//	entrypoint.http { method: POST, path: /api/payments, timeout: 5000, ... }
//	properties { key: value, ... }
//	step step_name(condition: expr, timeout: 2000, retry: { ... }) { risor code }
//	async step step_name(condition: expr, timeout: 2000, retry: { ... }) { risor code }
//	step step_name(condition: expr) as plugin.method { risor map body }
//	step step_name as subflow.flow_id { risor map body }
//	  fallback { risor code }          // optional suffix block
//	  compensate { risor code }        // optional suffix block
//	on_error { risor code }            // flow-level error handler
//	return response.json({ ... })
func Parse(source string) (runtime.Flow, error) {
	p := &parser{source: source, pos: 0}
	return p.parse()
}

type parser struct {
	source        string
	pos           int
	parallelCount int
	foreachCount  int
}

func (p *parser) parse() (runtime.Flow, error) {
	var flow runtime.Flow

	p.skipWhitespaceAndComments()
	for p.pos < len(p.source) {
		keyword := p.peekKeyword()

		switch {
		case strings.HasPrefix(keyword, "entrypoint"):
			ep, timeout, err := p.parseEntrypoint()
			if err != nil {
				return flow, fmt.Errorf("parsing entrypoint: %w", err)
			}
			flow.Entrypoint = ep
			flow.Timeout = timeout

		case keyword == "properties":
			props, err := p.parseProperties()
			if err != nil {
				return flow, fmt.Errorf("parsing properties: %w", err)
			}
			flow.Properties = props

		case keyword == "step":
			step, err := p.parseStep(false)
			if err != nil {
				return flow, fmt.Errorf("parsing step: %w", err)
			}
			flow.Steps = append(flow.Steps, step)
			flow.Nodes = append(flow.Nodes, runtime.FlowNode{ID: step.ID, Kind: runtime.FlowNodeStep, Step: &step})

		case keyword == "async":
			p.readWord()
			p.skipWhitespace()
			if p.peekKeyword() != "step" {
				return flow, fmt.Errorf("parsing async: expected step after async")
			}
			step, err := p.parseStep(true)
			if err != nil {
				return flow, fmt.Errorf("parsing async step: %w", err)
			}
			flow.Steps = append(flow.Steps, step)
			flow.Nodes = append(flow.Nodes, runtime.FlowNode{ID: step.ID, Kind: runtime.FlowNodeStep, Step: &step})

		case keyword == "parallel":
			node, err := p.parseParallel()
			if err != nil {
				return flow, fmt.Errorf("parsing parallel: %w", err)
			}
			flow.Nodes = append(flow.Nodes, node)

		case keyword == "foreach":
			node, err := p.parseForeach(false, runtime.ParallelOptions{})
			if err != nil {
				return flow, fmt.Errorf("parsing foreach: %w", err)
			}
			flow.Nodes = append(flow.Nodes, node)

		case keyword == "on_error":
			body, err := p.parseOnError()
			if err != nil {
				return flow, fmt.Errorf("parsing on_error: %w", err)
			}
			flow.OnErrorBody = body

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
// Returns the Entrypoint, the flow-level timeout (extracted from "timeout" key), and any error.
func (p *parser) parseEntrypoint() (runtime.Entrypoint, int, error) {
	word := p.readWord() // "entrypoint.http"
	parts := strings.SplitN(word, ".", 2)
	if len(parts) != 2 {
		return runtime.Entrypoint{}, 0, fmt.Errorf("expected entrypoint.TYPE, got %q", word)
	}
	epType := parts[1]

	p.skipWhitespace()
	body, err := p.readBracedBlock()
	if err != nil {
		return runtime.Entrypoint{}, 0, err
	}

	config, err := parseSimpleMap(body)
	if err != nil {
		return runtime.Entrypoint{}, 0, fmt.Errorf("parsing entrypoint config: %w", err)
	}
	input, err := parseEntrypointInput(epType, config)
	if err != nil {
		return runtime.Entrypoint{}, 0, fmt.Errorf("parsing entrypoint input contract: %w", err)
	}

	// Extract flow-level timeout from entrypoint config.
	timeout := 0
	if v, ok := config["timeout"]; ok {
		timeout = toInt(v)
		delete(config, "timeout")
	}

	return runtime.Entrypoint{
		Type:   epType,
		Config: config,
		Input:  input,
	}, timeout, nil
}

func parseEntrypointInput(epType string, config map[string]any) (*runtime.InputContract, error) {
	switch epType {
	case "http", "":
		return httpinput.ParseBinding(config)
	case "flow":
		return flowinput.ParseBinding(config)
	case "kafka":
		return kafkainput.ParseBinding(config)
	case "cron":
		return nil, nil
	default:
		return nil, nil
	}
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

// parseStep parses:
//
//	step NAME(condition: ..., timeout: N, retry: {...}) { body }
//	step NAME(condition: ..., timeout: N, retry: {...}) as plugin.method { map_body }
//	step NAME as subflow.flow_id { map_body }
//	  fallback { body }    // optional
//	  compensate { body }  // optional
func (p *parser) parseStep(async bool, inParallel ...bool) (runtime.Step, error) {
	branch := len(inParallel) > 0 && inParallel[0]
	p.readWord() // consume "step"
	p.skipWhitespace()

	// Read step name
	name := p.readStepName()
	if name == "" {
		return runtime.Step{}, fmt.Errorf("expected step name")
	}

	var step runtime.Step
	step.ID = name
	step.Async = async

	p.skipWhitespace()

	// Optional parenthesized options: (condition: ..., timeout: N, retry: {...})
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

	headerCallTarget := ""
	if p.peekKeyword() == "as" {
		p.readWord() // consume "as"
		p.skipWhitespace()
		headerCallTarget = p.readWord()
		if headerCallTarget == "" {
			return step, fmt.Errorf("missing step call target after as")
		}
		if err := validateStepHeaderCallTarget(headerCallTarget); err != nil {
			return step, err
		}
		p.skipWhitespace()
	}

	// Read the primary step body
	body, err := p.readBracedBlock()
	if err != nil {
		return step, fmt.Errorf("parsing step body: %w", err)
	}
	if headerCallTarget != "" {
		body = lowerStepHeaderCall(headerCallTarget, body)
	}
	step.Body = body

	// Optionally read suffix blocks: fallback {} and compensate {} in any order.
	for i := 0; i < 2; i++ {
		p.skipWhitespaceAndComments()
		kw := p.peekKeyword()
		if kw == "fallback" {
			p.readWord() // consume "fallback"
			p.skipWhitespace()
			fb, err := p.readBracedBlock()
			if err != nil {
				return step, fmt.Errorf("parsing fallback body for step %s: %w", name, err)
			}
			step.FallbackBody = fb
		} else if kw == "compensate" {
			if async || branch {
				return step, fmt.Errorf("step %s cannot have compensate block here", name)
			}
			p.readWord() // consume "compensate"
			p.skipWhitespace()
			cb, err := p.readBracedBlock()
			if err != nil {
				return step, fmt.Errorf("parsing compensate body for step %s: %w", name, err)
			}
			step.CompensateBody = cb
		} else {
			break
		}
	}

	return step, nil
}

func (p *parser) parseParallel() (runtime.FlowNode, error) {
	p.readWord() // consume "parallel"
	options := runtime.ParallelOptions{}
	hadOptions := false
	p.skipWhitespace()
	if p.pos < len(p.source) && p.source[p.pos] == '(' {
		hadOptions = true
		opts, err := p.readParenBlock()
		if err != nil {
			return runtime.FlowNode{}, fmt.Errorf("parsing parallel options: %w", err)
		}
		parsed, err := parseParallelOptions(opts)
		if err != nil {
			return runtime.FlowNode{}, err
		}
		options = parsed
	}

	p.skipWhitespace()
	if p.peekKeyword() == "foreach" {
		if !hadOptions {
			return runtime.FlowNode{}, fmt.Errorf("parallel foreach requires parentheses; use parallel() foreach")
		}
		return p.parseForeach(true, options)
	}

	body, err := p.readBracedBlock()
	if err != nil {
		return runtime.FlowNode{}, fmt.Errorf("parsing parallel body: %w", err)
	}

	p.parallelCount++
	id := fmt.Sprintf("%s%d", runtime.InternalParallelNodePrefix, p.parallelCount)
	branches, err := parseParallelBranches(body)
	if err != nil {
		return runtime.FlowNode{}, err
	}

	block := &runtime.ParallelBlock{Options: options, Branches: branches}
	return runtime.FlowNode{
		ID:       id,
		Kind:     runtime.FlowNodeParallel,
		Parallel: block,
	}, nil
}

func (p *parser) parseForeach(parallel bool, options runtime.ParallelOptions) (runtime.FlowNode, error) {
	p.readWord() // consume "foreach"
	p.skipWhitespace()

	expr, delimiter, err := p.readExpressionUntilTopLevelKeywords("as", "batch")
	if err != nil {
		return runtime.FlowNode{}, err
	}
	if strings.TrimSpace(expr) == "" {
		return runtime.FlowNode{}, fmt.Errorf("foreach source expression is required")
	}
	if delimiter == "" {
		return runtime.FlowNode{}, fmt.Errorf("foreach must include as <item>")
	}

	batchSize := 0
	if delimiter == "batch" {
		p.readWord() // consume "batch"
		p.skipWhitespace()
		literal := p.readWord()
		if !isPositiveIntegerLiteral(literal) {
			return runtime.FlowNode{}, fmt.Errorf("foreach batch size must be a positive integer literal")
		}
		batchSize = toInt(literal)
		p.skipWhitespace()
		if p.peekKeyword() != "as" {
			return runtime.FlowNode{}, fmt.Errorf("foreach batch must include as <item>")
		}
	}

	if p.peekKeyword() != "as" {
		return runtime.FlowNode{}, fmt.Errorf("foreach must include as <item>")
	}
	p.readWord() // consume "as"
	p.skipWhitespace()
	itemVar := p.readWord()
	if err := validateSimpleIdentifier("foreach loop variable", itemVar); err != nil {
		return runtime.FlowNode{}, err
	}

	p.skipWhitespace()
	body, err := p.readBracedBlock()
	if err != nil {
		return runtime.FlowNode{}, fmt.Errorf("parsing foreach body: %w", err)
	}
	steps, collects, err := parseForeachBody(body)
	if err != nil {
		return runtime.FlowNode{}, err
	}

	p.foreachCount++
	id := fmt.Sprintf("%s%d", runtime.InternalForeachNodePrefix, p.foreachCount)
	block := &runtime.ForeachBlock{
		Expr:      strings.TrimSpace(expr),
		ItemVar:   itemVar,
		BatchSize: batchSize,
		Parallel:  parallel,
		Options:   options,
		Steps:     steps,
		Collects:  collects,
	}
	return runtime.FlowNode{ID: id, Kind: runtime.FlowNodeForeach, Foreach: block}, nil
}

func parseForeachBody(body string) ([]runtime.Step, []runtime.ForeachCollect, error) {
	bodyParser := &parser{source: body}
	steps := []runtime.Step{}
	collects := []runtime.ForeachCollect{}
	seenSteps := map[string]struct{}{}
	seenCollects := map[string]struct{}{}

	bodyParser.skipWhitespaceAndComments()
	for bodyParser.pos < len(bodyParser.source) {
		keyword := bodyParser.peekKeyword()
		switch keyword {
		case "step":
			step, err := bodyParser.parseStep(false, true)
			if err != nil {
				return nil, nil, err
			}
			if _, exists := seenSteps[step.ID]; exists {
				return nil, nil, fmt.Errorf("duplicate foreach body step %q", step.ID)
			}
			seenSteps[step.ID] = struct{}{}
			steps = append(steps, step)
		case "async":
			bodyParser.readWord()
			bodyParser.skipWhitespace()
			if bodyParser.peekKeyword() != "step" {
				return nil, nil, fmt.Errorf("expected step after async")
			}
			step, err := bodyParser.parseStep(true, true)
			if err != nil {
				return nil, nil, err
			}
			if _, exists := seenSteps[step.ID]; exists {
				return nil, nil, fmt.Errorf("duplicate foreach body step %q", step.ID)
			}
			seenSteps[step.ID] = struct{}{}
			steps = append(steps, step)
		case "collect":
			collect, err := bodyParser.parseForeachCollect()
			if err != nil {
				return nil, nil, err
			}
			if _, exists := seenCollects[collect.Alias]; exists {
				return nil, nil, fmt.Errorf("duplicate foreach collect alias %q", collect.Alias)
			}
			seenCollects[collect.Alias] = struct{}{}
			collects = append(collects, collect)
		case "parallel":
			return nil, nil, fmt.Errorf("nested parallel blocks are not supported in foreach")
		case "foreach":
			return nil, nil, fmt.Errorf("nested foreach blocks are not supported")
		case "return":
			return nil, nil, fmt.Errorf("return is not allowed inside foreach")
		default:
			if bodyParser.pos < len(bodyParser.source) {
				return nil, nil, fmt.Errorf("unexpected foreach body token at position %d: %q", bodyParser.pos, bodyParser.source[bodyParser.pos:min(bodyParser.pos+20, len(bodyParser.source))])
			}
		}
		bodyParser.skipWhitespaceAndComments()
	}

	return steps, collects, nil
}

func (p *parser) parseForeachCollect() (runtime.ForeachCollect, error) {
	p.readWord() // consume "collect"
	p.skipWhitespace()
	expr, delimiter, err := p.readExpressionUntilTopLevelKeywords("as")
	if err != nil {
		return runtime.ForeachCollect{}, err
	}
	if strings.TrimSpace(expr) == "" {
		return runtime.ForeachCollect{}, fmt.Errorf("collect expression is required")
	}
	if delimiter != "as" {
		return runtime.ForeachCollect{}, fmt.Errorf("collect must include as <alias>")
	}
	p.readWord() // consume "as"
	p.skipWhitespace()
	alias := p.readWord()
	if err := validateSimpleIdentifier("collect alias", alias); err != nil {
		return runtime.ForeachCollect{}, err
	}
	return runtime.ForeachCollect{Expr: strings.TrimSpace(expr), Alias: alias}, nil
}

func parseParallelOptions(opts string) (runtime.ParallelOptions, error) {
	m, err := parseSimpleMap(opts)
	if err != nil {
		return runtime.ParallelOptions{}, fmt.Errorf("parsing parallel options: %w", err)
	}
	options := runtime.ParallelOptions{}
	for key, value := range m {
		switch key {
		case "max_in_flight":
			options.MaxInFlight = toInt(value)
			if options.MaxInFlight <= 0 {
				return options, fmt.Errorf("parallel max_in_flight must be greater than 0")
			}
		case "on_failure":
			mode := runtime.OnFailureMode(fmt.Sprintf("%v", value))
			if mode != runtime.OnFailureWaitAll && mode != runtime.OnFailureFailFast {
				return options, fmt.Errorf("invalid parallel on_failure value %q", value)
			}
			options.OnFailure = mode
		default:
			return options, fmt.Errorf("unknown parallel option %q", key)
		}
	}
	return options, nil
}

func parseParallelBranches(body string) ([]runtime.Step, error) {
	branchParser := &parser{source: body}
	branches := []runtime.Step{}
	seen := map[string]struct{}{}
	branchParser.skipWhitespaceAndComments()
	for branchParser.pos < len(branchParser.source) {
		keyword := branchParser.peekKeyword()
		switch keyword {
		case "step":
			step, err := branchParser.parseStep(false, true)
			if err != nil {
				return nil, err
			}
			if _, exists := seen[step.ID]; exists {
				return nil, fmt.Errorf("duplicate parallel branch %q", step.ID)
			}
			seen[step.ID] = struct{}{}
			branches = append(branches, step)
		case "async":
			branchParser.readWord()
			branchParser.skipWhitespace()
			if branchParser.peekKeyword() != "step" {
				return nil, fmt.Errorf("expected step after async")
			}
			step, err := branchParser.parseStep(true, true)
			if err != nil {
				return nil, err
			}
			if _, exists := seen[step.ID]; exists {
				return nil, fmt.Errorf("duplicate parallel branch %q", step.ID)
			}
			seen[step.ID] = struct{}{}
			branches = append(branches, step)
		case "parallel":
			return nil, fmt.Errorf("nested parallel blocks are not supported")
		default:
			if branchParser.pos < len(branchParser.source) {
				return nil, fmt.Errorf("unexpected parallel body token at position %d: %q", branchParser.pos, branchParser.source[branchParser.pos:min(branchParser.pos+20, len(branchParser.source))])
			}
		}
		branchParser.skipWhitespaceAndComments()
	}
	return branches, nil
}

func validateStepHeaderCallTarget(target string) error {
	if target == "flow.call" {
		return fmt.Errorf("as flow.call is not supported; use subflow.<flow_id> or manual flow.call(...)")
	}

	parts := strings.Split(target, ".")
	if len(parts) != 2 {
		return fmt.Errorf("step call target %q must be plugin.method or subflow.<flow_id>", target)
	}
	if parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("step call target %q must not have empty segments", target)
	}
	if parts[0] == "subflow" && parts[1] == "" {
		return fmt.Errorf("subflow step call target must include a flow ID")
	}
	return nil
}

func lowerStepHeaderCall(target, rawMapBody string) string {
	mapBody := normalizeHeaderCallMapBody(rawMapBody)
	parts := strings.SplitN(target, ".", 2)
	if parts[0] == "subflow" {
		return fmt.Sprintf("flow.call(%q, {%s})", parts[1], mapBody)
	}
	return fmt.Sprintf("%s({%s})", target, mapBody)
}

func normalizeHeaderCallMapBody(raw string) string {
	var b strings.Builder
	b.Grow(len(raw) + 8)

	depth := 0
	inString := false
	stringChar := byte(0)
	pendingEntry := false

	for i := 0; i < len(raw); i++ {
		ch := raw[i]

		if inString {
			b.WriteByte(ch)
			if ch == '\\' && i+1 < len(raw) {
				i++
				b.WriteByte(raw[i])
				continue
			}
			if ch == stringChar {
				inString = false
			}
			continue
		}

		if ch == '/' && i+1 < len(raw) && raw[i+1] == '/' {
			commentEnd := i + 2
			for commentEnd < len(raw) && raw[commentEnd] != '\n' {
				commentEnd++
			}
			if depth == 0 && pendingEntry && hasTopLevelMapKeyAfter(raw, commentEnd+1) {
				b.WriteByte(',')
				pendingEntry = false
			}
			b.WriteString(raw[i:commentEnd])
			i = commentEnd - 1
			continue
		}

		switch ch {
		case '"', '\'', '`':
			inString = true
			stringChar = ch
		case '{', '[', '(':
			depth++
		case '}', ']', ')':
			if depth > 0 {
				depth--
			}
		case ':':
			if depth == 0 {
				pendingEntry = true
			}
		case ',':
			if depth == 0 {
				pendingEntry = false
			}
		case '\n':
			if pendingEntry && hasTopLevelMapKeyAfter(raw, i+1) {
				b.WriteByte(',')
				pendingEntry = false
			}
		}

		b.WriteByte(ch)
	}

	return b.String()
}

func hasTopLevelMapKeyAfter(source string, pos int) bool {
	for pos < len(source) {
		for pos < len(source) && (source[pos] == ' ' || source[pos] == '\t' || source[pos] == '\r' || source[pos] == '\n') {
			pos++
		}
		if pos+1 < len(source) && source[pos] == '/' && source[pos+1] == '/' {
			for pos < len(source) && source[pos] != '\n' {
				pos++
			}
			continue
		}
		break
	}
	if pos >= len(source) {
		return false
	}

	if source[pos] == '"' || source[pos] == '\'' || source[pos] == '`' {
		quote := source[pos]
		pos++
		for pos < len(source) {
			if source[pos] == '\\' {
				pos += 2
				continue
			}
			if source[pos] == quote {
				pos++
				for pos < len(source) && (source[pos] == ' ' || source[pos] == '\t') {
					pos++
				}
				return pos < len(source) && source[pos] == ':'
			}
			if source[pos] == '\n' {
				return false
			}
			pos++
		}
		return false
	}

	if !isWordChar(source[pos]) {
		return false
	}
	for pos < len(source) && isWordChar(source[pos]) {
		pos++
	}
	for pos < len(source) && (source[pos] == ' ' || source[pos] == '\t') {
		pos++
	}
	return pos < len(source) && source[pos] == ':'
}

// parseOnError parses: on_error { risor code }
func (p *parser) parseOnError() (string, error) {
	p.readWord() // consume "on_error"
	p.skipWhitespace()
	body, err := p.readBracedBlock()
	if err != nil {
		return "", err
	}
	return body, nil
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
			if next == "step" || next == "async" || next == "parallel" || next == "foreach" || next == "return" || next == "properties" || next == "on_error" || strings.HasPrefix(next, "entrypoint") {
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

func (p *parser) readExpressionUntilTopLevelKeywords(keywords ...string) (string, string, error) {
	start := p.pos
	depth := 0
	inString := false
	stringChar := byte(0)

	for p.pos < len(p.source) {
		ch := p.source[p.pos]

		if inString {
			if ch == '\\' {
				p.pos += 2
				continue
			}
			if ch == stringChar {
				inString = false
			}
			p.pos++
			continue
		}

		switch ch {
		case '"', '\'', '`':
			inString = true
			stringChar = ch
			p.pos++
			continue
		case '(', '{', '[':
			depth++
		case ')', '}', ']':
			if depth == 0 {
				return strings.TrimSpace(p.source[start:p.pos]), "", nil
			}
			depth--
		}

		if depth == 0 {
			for _, keyword := range keywords {
				if p.matchesKeywordAt(keyword, p.pos) {
					return strings.TrimSpace(p.source[start:p.pos]), keyword, nil
				}
			}
		}
		p.pos++
	}

	return strings.TrimSpace(p.source[start:p.pos]), "", nil
}

func (p *parser) matchesKeywordAt(keyword string, pos int) bool {
	if pos < 0 || pos+len(keyword) > len(p.source) || p.source[pos:pos+len(keyword)] != keyword {
		return false
	}
	if pos > 0 && (isWordChar(p.source[pos-1]) || p.source[pos-1] == '.') {
		return false
	}
	after := pos + len(keyword)
	if after < len(p.source) && (isWordChar(p.source[after]) || p.source[after] == '.') {
		return false
	}
	return true
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

func validateSimpleIdentifier(label string, id string) error {
	if id == "" {
		return fmt.Errorf("%s is required", label)
	}
	for i := 0; i < len(id); i++ {
		if !isWordChar(id[i]) {
			return fmt.Errorf("%s %q must be a simple identifier", label, id)
		}
	}
	return nil
}

func isPositiveIntegerLiteral(value string) bool {
	if value == "" {
		return false
	}
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return toInt(value) > 0
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
	squareDepth := 0
	parenDepth := 0
	curlyDepth := 0
	inString := false
	stringChar := byte(0)

	for i < len(block) {
		c := block[i]

		if inString {
			if c == '\\' {
				i++
			} else if c == stringChar {
				inString = false
			}
			i++
			continue
		}

		switch c {
		case '"', '\'', '`':
			inString = true
			stringChar = c
		case '[':
			squareDepth++
		case ']':
			if squareDepth > 0 {
				squareDepth--
			} else if parenDepth == 0 && curlyDepth == 0 {
				// End of parent container (array) at top-level.
				return strings.TrimSpace(block[start:i]), i, nil
			}
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case '{':
			curlyDepth++
		case '}':
			if curlyDepth > 0 {
				curlyDepth--
			} else if squareDepth == 0 && parenDepth == 0 {
				// End of parent container (map) at top-level.
				return strings.TrimSpace(block[start:i]), i, nil
			}
		case ',', '\n':
			if squareDepth == 0 && parenDepth == 0 && curlyDepth == 0 {
				return strings.TrimSpace(block[start:i]), i, nil
			}
		}

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
// Supports fields: max_attempts, delay, backoff, max_delay, jitter, when, non_retryable, timeout.
func applyStepOptions(step *runtime.Step, opts string) error {
	m, err := parseSimpleMap(opts)
	if err != nil {
		return fmt.Errorf("parsing step options: %w", err)
	}

	if cond, ok := m["condition"]; ok {
		step.Condition = fmt.Sprintf("%v", cond)
	}

	if t, ok := m["timeout"]; ok {
		step.Timeout = toInt(t)
	}

	if retryRaw, ok := m["retry"]; ok {
		retryMap, ok := retryRaw.(map[string]any)
		if !ok {
			return fmt.Errorf("retry must be a map")
		}
		// Legacy aliases removed intentionally.
		if _, ok := retryMap["maxRetries"]; ok {
			return fmt.Errorf("unsupported retry field: maxRetries (use max_attempts)")
		}
		if _, ok := retryMap["condition"]; ok {
			return fmt.Errorf("unsupported retry field: condition (use when)")
		}
		step.Retry = &runtime.RetryConfig{}

		if v, ok := retryMap["max_attempts"]; ok {
			step.Retry.MaxAttempts = toInt(v)
		}
		if v, ok := retryMap["max_delay"]; ok {
			step.Retry.MaxDelay = toInt(v)
		}
		if v, ok := retryMap["jitter"]; ok {
			step.Retry.Jitter = toBool(v)
		}
		if v, ok := retryMap["when"]; ok {
			step.Retry.When = fmt.Sprintf("%v", v)
		}
		if v, ok := retryMap["non_retryable"]; ok {
			if arr, ok := v.([]any); ok {
				for _, item := range arr {
					step.Retry.NonRetryable = append(step.Retry.NonRetryable, fmt.Sprintf("%v", item))
				}
			}
		}
		if v, ok := retryMap["backoff"]; ok {
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("retry.backoff must be a string: none | linear | exponential")
			}
			switch s {
			case "none", "linear", "exponential":
				step.Retry.Backoff = s
			default:
				return fmt.Errorf("invalid retry.backoff value: %q (allowed: none, linear, exponential)", s)
			}
		}

		if v, ok := retryMap["delay"]; ok {
			step.Retry.Delay = toInt(v)
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
