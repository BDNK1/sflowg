package runtime

import "testing"

func TestBuiltInResponseContractsExposeExpectedSubtypes(t *testing.T) {
	tests := []struct {
		name     string
		contract ResponseContract
		want     []string
	}{
		{name: "http", contract: HTTPResponseContract(), want: []string{"json", "text", "redirect"}},
		{name: "flow", contract: FlowResponseContract(), want: []string{"value", "error"}},
		{name: "kafka", contract: KafkaResponseContract(), want: []string{"ack", "nack"}},
	}

	for _, tt := range tests {
		if !tt.contract.EqualSubtypes(tt.want) {
			t.Fatalf("%s subtypes = %#v, want %#v", tt.name, tt.contract.Subtypes(), tt.want)
		}
	}
}

func TestKafkaResponseContractRejectsArguments(t *testing.T) {
	contract := KafkaResponseContract()
	if _, err := contract.ValidateArgs("ack", nil); err != nil {
		t.Fatalf("ack with no args failed: %v", err)
	}
	if _, err := contract.ValidateArgs("nack", []any{map[string]any{}}); err == nil {
		t.Fatal("expected nack with args to fail")
	}
	if err := contract.ValidateStaticArgCount("ack", 1); err == nil {
		t.Fatal("expected static ack arg validation to fail")
	}
}

func TestHTTPResponseContractRequiresSingleMapArg(t *testing.T) {
	contract := HTTPResponseContract()
	if _, err := contract.ValidateArgs("json", []any{map[string]any{"status": 200}}); err != nil {
		t.Fatalf("json map arg failed: %v", err)
	}
	if _, err := contract.ValidateArgs("json", nil); err == nil {
		t.Fatal("expected json with no args to fail")
	}
	if _, err := contract.ValidateArgs("json", []any{200, map[string]any{"ok": true}}); err == nil {
		t.Fatal("expected legacy two-arg json form to fail")
	}
}
