package kafkatransport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/BDNK1/sflowg/core"
	"github.com/BDNK1/sflowg/core/validation/kafkainput"
	validationschema "github.com/BDNK1/sflowg/core/validation/schema"
	"github.com/IBM/sarama"
	"github.com/ThreeDotsLabs/watermill"
	wmkafka "github.com/ThreeDotsLabs/watermill-kafka/v3/pkg/kafka"
	"github.com/ThreeDotsLabs/watermill/message"
)

const defaultKafkaFlowTimeout = 5 * time.Minute

type Config struct {
	Brokers map[string]BrokerConfig
}

type BrokerConfig struct {
	Brokers               []string
	ClientID              string
	NackRedeliveryDelayMS int
}

type Transport struct {
	cfg           Config
	mu            sync.Mutex
	subscribers   []subscriber
	consumers     sync.WaitGroup
	newSubscriber func(BrokerConfig, kafkaFlowConfig) (subscriber, error)
}

type subscriber interface {
	Subscribe(context.Context, string) (<-chan *message.Message, error)
	Close() error
}

type kafkaFlowConfig struct {
	Broker          string
	Topic           string
	GroupID         string
	AutoOffsetReset string
}

func New(cfg Config) *Transport {
	t := &Transport{cfg: cfg}
	t.newSubscriber = t.createSubscriber
	return t
}

func (t *Transport) Type() string { return "kafka" }

func (t *Transport) ResponseContract() runtime.ResponseContract {
	return runtime.KafkaResponseContract()
}

func (t *Transport) ValidateFlow(flow runtime.Flow) error {
	cfg, err := readKafkaFlowConfig(flow)
	if err != nil {
		return err
	}
	broker, ok := t.cfg.Brokers[cfg.Broker]
	if !ok {
		return fmt.Errorf("broker %q is not configured", cfg.Broker)
	}
	if len(broker.Brokers) == 0 {
		return fmt.Errorf("broker %q has no Kafka bootstrap servers", cfg.Broker)
	}
	if cfg.Topic == "" {
		return fmt.Errorf("topic must be a non-empty string")
	}
	if cfg.GroupID == "" {
		return fmt.Errorf("group_id must be a non-empty string")
	}
	switch cfg.AutoOffsetReset {
	case "earliest", "latest":
	default:
		return fmt.Errorf("auto_offset_reset must be earliest or latest")
	}
	return nil
}

func (t *Transport) ValidateTransportFlows(flows []runtime.Flow) error {
	seen := map[string]string{}
	for _, flow := range flows {
		cfg, err := readKafkaFlowConfig(flow)
		if err != nil {
			return fmt.Errorf("flow %q: %w", flow.ID, err)
		}
		key := cfg.Broker + "\x00" + cfg.Topic + "\x00" + cfg.GroupID
		if existing, ok := seen[key]; ok {
			return fmt.Errorf("duplicate broker/topic/group_id for flows %q and %q", existing, flow.ID)
		}
		seen[key] = flow.ID
	}
	return nil
}

func (t *Transport) Start(ctx context.Context, rt runtime.TransportRuntime) error {
	errCh := make(chan error, len(rt.Flows))
	for i := range rt.Flows {
		flow := rt.Flows[i]
		cfg, err := readKafkaFlowConfig(flow)
		if err != nil {
			return fmt.Errorf("flow %q: %w", flow.ID, err)
		}
		broker := t.cfg.Brokers[cfg.Broker]
		sub, err := t.newSubscriber(broker, cfg)
		if err != nil {
			return fmt.Errorf("flow %q subscriber: %w", flow.ID, err)
		}
		t.addSubscriber(sub)

		messages, err := sub.Subscribe(ctx, cfg.Topic)
		if err != nil {
			return fmt.Errorf("flow %q subscribe topic %q: %w", flow.ID, cfg.Topic, err)
		}
		rt.Container.Logger().Info("Kafka consumer subscribed", "flow_id", flow.ID, "topic", cfg.Topic, "group_id", cfg.GroupID)
		t.consumers.Add(1)
		go func() {
			defer t.consumers.Done()
			errCh <- t.consume(ctx, rt, &flow, messages)
		}()
	}

	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		remaining := len(rt.Flows) - 1
		drained := drainErrors(errCh, remaining)
		return errors.Join(append([]error{err}, drained...)...)
	}
}

func (t *Transport) Shutdown(ctx context.Context) error {
	t.mu.Lock()
	subs := t.subscribers
	t.subscribers = nil
	t.mu.Unlock()

	var errs []error
	for _, sub := range subs {
		if err := sub.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	done := make(chan struct{})
	go func() {
		t.consumers.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		errs = append(errs, fmt.Errorf("kafka shutdown timeout: %w", ctx.Err()))
	}

	return errors.Join(errs...)
}

func drainErrors(ch <-chan error, remaining int) []error {
	if remaining <= 0 {
		return nil
	}
	out := make([]error, 0, remaining)
	for i := 0; i < remaining; i++ {
		select {
		case err := <-ch:
			if err != nil {
				out = append(out, err)
			}
		default:
			return out
		}
	}
	return out
}

func (t *Transport) createSubscriber(broker BrokerConfig, flow kafkaFlowConfig) (subscriber, error) {
	saramaCfg := wmkafka.DefaultSaramaSubscriberConfig()
	if broker.ClientID != "" {
		saramaCfg.ClientID = broker.ClientID
	}
	switch flow.AutoOffsetReset {
	case "earliest":
		saramaCfg.Consumer.Offsets.Initial = sarama.OffsetOldest
	case "latest":
		saramaCfg.Consumer.Offsets.Initial = sarama.OffsetNewest
	}
	return wmkafka.NewSubscriber(wmkafka.SubscriberConfig{
		Brokers:               broker.Brokers,
		Unmarshaler:           wmkafka.DefaultMarshaler{},
		OverwriteSaramaConfig: saramaCfg,
		ConsumerGroup:         flow.GroupID,
		NackResendSleep:       time.Duration(broker.NackRedeliveryDelayMS) * time.Millisecond,
	}, watermill.NewStdLogger(false, false))
}

func (t *Transport) addSubscriber(sub subscriber) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.subscribers = append(t.subscribers, sub)
}

func (t *Transport) consume(ctx context.Context, rt runtime.TransportRuntime, flow *runtime.Flow, messages <-chan *message.Message) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-messages:
			if !ok {
				return nil
			}
			t.handleMessage(ctx, rt, flow, msg)
		}
	}
}

func (t *Transport) handleMessage(ctx context.Context, rt runtime.TransportRuntime, flow *runtime.Flow, msg *message.Message) {
	execution := runtime.NewExecution(flow, rt.Container, rt.GlobalProperties, rt.NewValueStore())
	timeout := time.Duration(flow.Timeout) * time.Millisecond
	if timeout <= 0 {
		timeout = defaultKafkaFlowTimeout
	}
	msgCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	execution = execution.WithContext(msgCtx)

	defer func() {
		if r := recover(); r != nil {
			execution.Logger().Error("Kafka message handler panicked", "error", r, "flow_id", flow.ID)
			msg.Nack()
		}
	}()

	boundaryErr := populateMessage(execution, flow, msg)
	if boundaryErr != nil {
		handled, handlerErr := rt.Executor.HandleBoundaryError(execution, boundaryErr)
		if handlerErr != nil {
			execution.Logger().Error("Kafka on_error handler failed", "error", handlerErr)
			msg.Nack()
			return
		}
		if handled && execution.State().Response() != nil {
			dispatchKafkaResponse(msg, execution.State().Response())
			return
		}
		msg.Nack()
		return
	}

	if err := rt.Executor.ExecuteSteps(execution); err != nil {
		execution.Logger().Error("Kafka flow execution failed", "error", err)
		if execution.State().Response() != nil {
			dispatchKafkaResponse(msg, execution.State().Response())
			return
		}
		msg.Nack()
		return
	}
	dispatchKafkaResponse(msg, execution.State().Response())
}

func dispatchKafkaResponse(msg *message.Message, response *runtime.ResponseDescriptor) {
	if response == nil {
		msg.Nack()
		return
	}
	switch response.Subtype {
	case "ack":
		msg.Ack()
	case "nack":
		msg.Nack()
	default:
		msg.Nack()
	}
}

func populateMessage(execution *runtime.Execution, flow *runtime.Flow, msg *message.Message) *runtime.FlowError {
	rawValue := string(msg.Payload)
	if !utf8.Valid(msg.Payload) {
		return kafkaBoundaryError("message.value", "utf8", rawValue, "message value must be valid UTF-8")
	}
	execution.AddValue("message.rawValue", rawValue)

	headers := map[string]any{}
	for key, value := range msg.Metadata {
		if !utf8.ValidString(key) || !utf8.ValidString(value) {
			return kafkaBoundaryError("message.headers", "utf8", key, "message headers must be valid UTF-8")
		}
		headers[key] = value
	}
	execution.State().Store().SetNested("message.headers", headers)

	if key, ok := wmkafka.MessageKeyFromCtx(msg.Context()); ok {
		if !utf8.Valid(key) {
			return kafkaBoundaryError("message.key", "utf8", string(key), "message key must be valid UTF-8")
		}
		execution.AddValue("message.key", string(key))
	}
	if ts, ok := wmkafka.MessageTimestampFromCtx(msg.Context()); ok && !ts.IsZero() {
		execution.AddValue("message.timestamp", ts.Format(time.RFC3339Nano))
	}
	if offset, ok := wmkafka.MessagePartitionOffsetFromCtx(msg.Context()); ok {
		execution.AddValue("message.offset", offset)
	}
	if partition, ok := wmkafka.MessagePartitionFromCtx(msg.Context()); ok {
		execution.AddValue("message.partition", partition)
	}
	if cfg, err := readKafkaFlowConfig(*flow); err == nil && cfg.Topic != "" {
		execution.AddValue("message.topic", cfg.Topic)
	}

	var parsed any
	if err := json.Unmarshal(msg.Payload, &parsed); err != nil {
		return kafkaBoundaryError("message.value", "json", rawValue, "message value must be valid JSON")
	}
	execution.State().Store().SetNested("message.value", parsed)

	if flow.Entrypoint.Input != nil {
		if valueSchema, ok := flow.Entrypoint.Input.RootSchema(kafkainput.ValueNamespace); ok {
			raw, present := execution.State().Store().Get(kafkainput.ValueNamespace)
			normalized, errs := validationschema.ValidateField(raw, present, valueSchema, "value")
			if len(errs) > 0 {
				return schemaViolation(errs)
			}
			execution.State().Store().SetNested(kafkainput.ValueNamespace, normalized)
		}
	}
	return nil
}

func readKafkaFlowConfig(flow runtime.Flow) (kafkaFlowConfig, error) {
	config := flow.Entrypoint.Config
	broker, _ := config["broker"].(string)
	if broker == "" {
		broker = "default"
	}
	topic, _ := config["topic"].(string)
	groupID, _ := config["group_id"].(string)
	autoOffsetReset, _ := config["auto_offset_reset"].(string)
	return kafkaFlowConfig{
		Broker:          broker,
		Topic:           topic,
		GroupID:         groupID,
		AutoOffsetReset: autoOffsetReset,
	}, nil
}

func kafkaBoundaryError(path, constraint string, got any, message string) *runtime.FlowError {
	return schemaViolation([]validationschema.FieldError{{
		Path:       path,
		Pointer:    "#/" + strings.ReplaceAll(path, ".", "/"),
		Constraint: constraint,
		Got:        got,
		Message:    message,
	}})
}

func schemaViolation(fields []validationschema.FieldError) *runtime.FlowError {
	return &runtime.FlowError{
		Type:    runtime.ErrorTypePermanent,
		Code:    string(runtime.ErrorCodeSchemaViolation),
		Message: "Message validation failed",
		Meta: map[string]any{
			"fields": validationschema.FieldsToMaps(fields),
		},
	}
}
