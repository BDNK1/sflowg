package crontransport

import (
	"context"
	"testing"
	"time"

	"github.com/BDNK1/sflowg/core"
)

type captureRunner struct {
	values map[string]any
}

func (r *captureRunner) RunStep(_ context.Context, _ *runtime.Execution, input runtime.StepInput) (runtime.StepOutput, error) {
	r.values = input.Input
	return runtime.StepOutput{}, nil
}

type noopEvaluator struct{}

func (noopEvaluator) Eval(*runtime.Execution, string) (any, error) {
	return nil, nil
}

func (noopEvaluator) EvalWithEnv(*runtime.Execution, string, map[string]any) (any, error) {
	return nil, nil
}

func TestValidateFlowScheduleAndTimezone(t *testing.T) {
	transport := New()
	valid := runtime.Flow{Entrypoint: runtime.Entrypoint{Type: "cron", Config: map[string]any{
		"schedule": "*/5 * * * *",
		"timezone": "Europe/Madrid",
	}}}
	if err := transport.ValidateFlow(valid); err != nil {
		t.Fatalf("ValidateFlow(valid) error = %v", err)
	}

	missing := valid
	missing.Entrypoint.Config = map[string]any{}
	if err := transport.ValidateFlow(missing); err == nil {
		t.Fatal("expected missing schedule to fail")
	}

	sixFields := valid
	sixFields.Entrypoint.Config = map[string]any{"schedule": "0 */5 * * * *"}
	if err := transport.ValidateFlow(sixFields); err == nil {
		t.Fatal("expected six-field schedule to fail")
	}

	badTimezone := valid
	badTimezone.Entrypoint.Config = map[string]any{"schedule": "*/5 * * * *", "timezone": "Mars/Base"}
	if err := transport.ValidateFlow(badTimezone); err == nil {
		t.Fatal("expected invalid timezone to fail")
	}
}

func TestRunFlowSeedsTriggerScheduledAtAndFireCount(t *testing.T) {
	location, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		t.Fatal(err)
	}
	runner := &captureRunner{}
	executor := runtime.NewExecutor(noopEvaluator{}, nil, runner)
	container := runtime.NewContainer(runtime.NewLogger(nil))
	flow := &runtime.Flow{
		ID:               "reconcile",
		Entrypoint:       runtime.Entrypoint{Type: "cron"},
		ResponseContract: runtime.CronResponseContract(),
		Steps:            []runtime.Step{{ID: "work"}},
	}
	rt := runtime.TransportRuntime{
		Container:     container,
		Executor:      executor,
		NewValueStore: func() runtime.ValueStore { return runtime.NewValueStore() },
	}
	scheduledAt := time.Date(2026, 5, 1, 10, 30, 0, 123, location)

	New().runFlow(context.Background(), rt, flow, location, scheduledAt, 7)

	trigger, ok := runner.values["trigger"].(map[string]any)
	if !ok {
		t.Fatalf("trigger = %#v, want map", runner.values["trigger"])
	}
	if got := trigger["scheduled_at"]; got != scheduledAt.Format(time.RFC3339Nano) {
		t.Fatalf("trigger.scheduled_at = %#v, want %q", got, scheduledAt.Format(time.RFC3339Nano))
	}
	if got := trigger["fire_count"]; got != 7 {
		t.Fatalf("trigger.fire_count = %#v, want 7", got)
	}
}

func TestNextScheduledAfterSkipsMissedTicks(t *testing.T) {
	cfg := runtime.Flow{Entrypoint: runtime.Entrypoint{Type: "cron", Config: map[string]any{"schedule": "* * * * *"}}}
	flowCfg, err := readFlowConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cursor := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	now := time.Date(2026, 5, 1, 10, 5, 30, 0, time.UTC)

	next, skipped := nextScheduledAfter(flowCfg.Schedule, cursor, now)

	want := time.Date(2026, 5, 1, 10, 6, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("next = %s, want %s", next, want)
	}
	if skipped != 5 {
		t.Fatalf("skipped = %d, want 5", skipped)
	}
}

func TestHasMissedFollowingTickOnlySkipsLateWakeups(t *testing.T) {
	cfg := runtime.Flow{Entrypoint: runtime.Entrypoint{Type: "cron", Config: map[string]any{"schedule": "* * * * *"}}}
	flowCfg, err := readFlowConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	scheduledAt := time.Date(2026, 5, 1, 10, 5, 0, 0, time.UTC)

	if hasMissedFollowingTick(flowCfg.Schedule, scheduledAt, scheduledAt.Add(500*time.Millisecond)) {
		t.Fatal("should not skip a slightly late scheduled fire")
	}
	if !hasMissedFollowingTick(flowCfg.Schedule, scheduledAt, scheduledAt.Add(2*time.Minute)) {
		t.Fatal("expected late wakeup after following ticks to be skipped")
	}
}
