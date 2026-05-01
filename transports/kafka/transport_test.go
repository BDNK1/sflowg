package kafkatransport

import (
	"testing"

	"github.com/BDNK1/sflowg/core"
	"github.com/BDNK1/sflowg/core/validation/kafkainput"
	"github.com/BDNK1/sflowg/core/validation/schema"
	"github.com/ThreeDotsLabs/watermill/message"
)

func TestValidateFlowRequiresConfiguredBrokerAndConsumerFields(t *testing.T) {
	transport := New(Config{Brokers: map[string]BrokerConfig{
		"default": {Brokers: []string{"localhost:9092"}, NackRedeliveryDelayMS: 100},
	}})
	flow := runtime.Flow{Entrypoint: runtime.Entrypoint{Type: "kafka", Config: map[string]any{
		"broker":            "missing",
		"topic":             "orders",
		"group_id":          "orders-service",
		"auto_offset_reset": "earliest",
	}}}
	if err := transport.ValidateFlow(flow); err == nil {
		t.Fatal("expected missing broker error")
	}

	flow.Entrypoint.Config["broker"] = "default"
	if err := transport.ValidateFlow(flow); err != nil {
		t.Fatalf("valid kafka flow failed: %v", err)
	}
}

func TestValidateTransportFlowsRejectsDuplicateConsumer(t *testing.T) {
	transport := New(Config{})
	flows := []runtime.Flow{
		{ID: "a", Entrypoint: runtime.Entrypoint{Config: kafkaConfig("orders", "orders-service")}},
		{ID: "b", Entrypoint: runtime.Entrypoint{Config: kafkaConfig("orders", "orders-service")}},
	}
	if err := transport.ValidateTransportFlows(flows); err == nil {
		t.Fatal("expected duplicate consumer error")
	}
}

func TestPopulateMessageSetsRawValueAndAppliesSchemaDefaults(t *testing.T) {
	input := runtime.NewInputContract()
	input.SetRoot(kafkainput.ValueNamespace, &schema.Schema{
		Type: schema.TypeObject,
		Properties: map[string]*schema.Schema{
			"order_id": {Type: schema.TypeString, Required: true},
			"status":   {Type: schema.TypeString, Default: "new"},
		},
	})
	flow := &runtime.Flow{
		Entrypoint: runtime.Entrypoint{
			Type:   "kafka",
			Config: kafkaConfig("orders", "orders-service"),
			Input:  input,
		},
	}
	execution := runtime.NewExecution(flow, nil, nil, runtime.NewValueStore())
	msg := message.NewMessage("id", []byte(`{"order_id":"ord_1"}`))
	msg.Metadata.Set("X-Tenant", "tenant-a")

	if err := populateMessage(execution, flow, msg); err != nil {
		t.Fatalf("populateMessage failed: %v", err)
	}
	if raw, _ := execution.State().Store().Get("message.rawValue"); raw != `{"order_id":"ord_1"}` {
		t.Fatalf("message.rawValue = %#v", raw)
	}
	if topic, _ := execution.State().Store().Get("message.topic"); topic != "orders" {
		t.Fatalf("message.topic = %#v", topic)
	}
	value, _ := execution.State().Store().Get(kafkainput.ValueNamespace)
	valueMap := value.(map[string]any)
	if valueMap["status"] != "new" {
		t.Fatalf("schema default not applied: %#v", valueMap)
	}
}

func TestPopulateMessageRejectsInvalidJSON(t *testing.T) {
	flow := &runtime.Flow{Entrypoint: runtime.Entrypoint{Type: "kafka"}}
	execution := runtime.NewExecution(flow, nil, nil, runtime.NewValueStore())
	err := populateMessage(execution, flow, message.NewMessage("id", []byte(`{bad`)))
	if err == nil || err.Code != string(runtime.ErrorCodeSchemaViolation) {
		t.Fatalf("expected schema violation, got %#v", err)
	}
}

func kafkaConfig(topic, groupID string) map[string]any {
	return map[string]any{
		"broker":            "default",
		"topic":             topic,
		"group_id":          groupID,
		"auto_offset_reset": "earliest",
	}
}
