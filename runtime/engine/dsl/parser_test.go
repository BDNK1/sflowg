package dsl

import (
	"testing"
)

func TestParse_Entrypoint(t *testing.T) {
	source := `entrypoint.http {
	method: POST
	path: /api/payments
	headers: [Authorization, Content-Type]
}`

	flow, err := Parse(source)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if flow.Entrypoint.Type != "http" {
		t.Errorf("entrypoint type = %q, want http", flow.Entrypoint.Type)
	}
	if flow.Entrypoint.Config["method"] != "POST" {
		t.Errorf("method = %v, want POST", flow.Entrypoint.Config["method"])
	}
	if flow.Entrypoint.Config["path"] != "/api/payments" {
		t.Errorf("path = %v, want /api/payments", flow.Entrypoint.Config["path"])
	}

	headers, ok := flow.Entrypoint.Config["headers"].([]any)
	if !ok {
		t.Fatalf("headers is not []any, got %T", flow.Entrypoint.Config["headers"])
	}
	if len(headers) != 2 {
		t.Errorf("headers len = %d, want 2", len(headers))
	}
}

func TestParse_Properties(t *testing.T) {
	source := `properties {
	base_url: "https://api.stripe.com"
	timeout: 30
}`

	flow, err := Parse(source)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if flow.Properties["base_url"] != "https://api.stripe.com" {
		t.Errorf("base_url = %v, want https://api.stripe.com", flow.Properties["base_url"])
	}
	if flow.Properties["timeout"] != "30" {
		t.Errorf("timeout = %v, want 30", flow.Properties["timeout"])
	}
}

func TestParse_Step(t *testing.T) {
	source := `step create_customer {
	result := http.request({
		url: base_url + "/customers",
		method: "POST",
		body: { email: request.body.email }
	})
}`

	flow, err := Parse(source)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(flow.Steps) != 1 {
		t.Fatalf("steps len = %d, want 1", len(flow.Steps))
	}
	step := flow.Steps[0]
	if step.ID != "create_customer" {
		t.Errorf("step ID = %q, want create_customer", step.ID)
	}
	if step.Body == "" {
		t.Error("step body is empty")
	}
}

func TestParse_StepWithCondition(t *testing.T) {
	source := `step handle_error(condition: create_customer.status_code != 200) {
	response.json({status: 400, body: {error: "failed"}})
}`

	flow, err := Parse(source)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(flow.Steps) != 1 {
		t.Fatalf("steps len = %d, want 1", len(flow.Steps))
	}
	step := flow.Steps[0]
	if step.ID != "handle_error" {
		t.Errorf("step ID = %q, want handle_error", step.ID)
	}
	if step.Condition != "create_customer.status_code != 200" {
		t.Errorf("condition = %q, want create_customer.status_code != 200", step.Condition)
	}
}

func TestParse_StepWithRetry(t *testing.T) {
	source := `step call_api(retry: { maxRetries: 3, delay: 1000, condition: call_api.status_code != 200 }) {
	http.request({url: "https://api.example.com"})
}`

	flow, err := Parse(source)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	step := flow.Steps[0]
	if step.Retry == nil {
		t.Fatal("retry config is nil")
	}
	if step.Retry.MaxRetries != 3 {
		t.Errorf("maxRetries = %d, want 3", step.Retry.MaxRetries)
	}
	if step.Retry.Delay != 1000 {
		t.Errorf("delay = %d, want 1000", step.Retry.Delay)
	}
	if step.Retry.Condition != "call_api.status_code != 200" {
		t.Errorf("retry condition = %q, want call_api.status_code != 200", step.Retry.Condition)
	}
}

func TestParse_Return(t *testing.T) {
	source := `return response.json({status: 201, body: {payment_id: create_payment.body.id}})`

	flow, err := Parse(source)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if flow.Return.Body == "" {
		t.Error("return body is empty")
	}
	expected := `response.json({status: 201, body: {payment_id: create_payment.body.id}})`
	if flow.Return.Body != expected {
		t.Errorf("return body = %q, want %q", flow.Return.Body, expected)
	}
}

func TestParse_FullFlow(t *testing.T) {
	source := `// Payment processing flow
entrypoint.http {
	method: POST
	path: /api/payments
	headers: [Authorization]
	body: { type: json }
}

properties {
	stripe_url: "https://api.stripe.com"
}

step create_customer {
	result := http.request({
		url: stripe_url + "/v1/customers",
		method: "POST"
	})
}

step create_payment(condition: create_customer.status_code == 200) {
	result := http.request({
		url: stripe_url + "/v1/charges",
		method: "POST"
	})
}

return response.json({status: 201, body: {id: create_payment.body.id}})`

	flow, err := Parse(source)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if flow.Entrypoint.Type != "http" {
		t.Errorf("entrypoint type = %q, want http", flow.Entrypoint.Type)
	}
	if flow.Properties["stripe_url"] != "https://api.stripe.com" {
		t.Errorf("stripe_url = %v", flow.Properties["stripe_url"])
	}
	if len(flow.Steps) != 2 {
		t.Errorf("steps = %d, want 2", len(flow.Steps))
	}
	if flow.Steps[0].ID != "create_customer" {
		t.Errorf("step[0] ID = %q, want create_customer", flow.Steps[0].ID)
	}
	if flow.Steps[1].ID != "create_payment" {
		t.Errorf("step[1] ID = %q, want create_payment", flow.Steps[1].ID)
	}
	if flow.Return.Body == "" {
		t.Error("return body is empty")
	}
}

func TestParse_CommentsSkipped(t *testing.T) {
	source := `// This is a comment
properties {
	// Comment inside block
	key: value
}`

	flow, err := Parse(source)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if flow.Properties["key"] != "value" {
		t.Errorf("key = %v, want value", flow.Properties["key"])
	}
}

func TestParse_StringsWithBraces(t *testing.T) {
	source := `step test {
	x := "hello { world }"
}`

	flow, err := Parse(source)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(flow.Steps) != 1 {
		t.Fatalf("steps len = %d, want 1", len(flow.Steps))
	}
	// Body should contain the full string including braces
	if flow.Steps[0].Body == "" {
		t.Error("step body is empty")
	}
}

func TestParse_CreateNoteFlow(t *testing.T) {
	source := `// Create a new note: validate input, save to DB, return the created note

entrypoint.http {
    method: POST
    path: /api/notes
    body: { type: json }
}

step validate_input(condition: request.body.title == "") {
    response.json({
        status: 400,
        body: { error: "title is required" }
    })
    early_return()
}

step save_note {
    postgres.get({
        query: "INSERT INTO notes (title, content, author) VALUES ($1, $2, $3) RETURNING id, title, content, author, created_at",
        params: [request.body.title, request.body.content, request.body.author]
    })
}

return response.json({
    status: 201,
    body: {
        note: save_note.row
    }
})`

	flow, err := Parse(source)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if flow.Entrypoint.Type != "http" {
		t.Errorf("entrypoint type = %q, want http", flow.Entrypoint.Type)
	}
	if flow.Entrypoint.Config["method"] != "POST" {
		t.Errorf("method = %v, want POST", flow.Entrypoint.Config["method"])
	}
	if flow.Entrypoint.Config["path"] != "/api/notes" {
		t.Errorf("path = %v, want /api/notes", flow.Entrypoint.Config["path"])
	}
	if len(flow.Steps) != 2 {
		t.Fatalf("steps = %d, want 2", len(flow.Steps))
	}
	if flow.Steps[0].ID != "validate_input" {
		t.Errorf("step[0] ID = %q, want validate_input", flow.Steps[0].ID)
	}
	if flow.Steps[0].Condition == "" {
		t.Error("validate_input should have a condition")
	}
	if flow.Steps[1].ID != "save_note" {
		t.Errorf("step[1] ID = %q, want save_note", flow.Steps[1].ID)
	}
	if flow.Return.Body == "" {
		t.Error("return body is empty")
	}
}

func TestResolveEnvCall(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`env("STRIPE_KEY")`, `${STRIPE_KEY}`},
		{`env("DB_URL", "localhost:5432")`, `${DB_URL:localhost:5432}`},
		{`plain_value`, `plain_value`},
		{`"quoted_value"`, `"quoted_value"`},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := resolveEnvCall(tt.input)
			if got != tt.want {
				t.Errorf("resolveEnvCall(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
