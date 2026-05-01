package crontransport

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/BDNK1/sflowg/core"
	"github.com/robfig/cron/v3"
)

const defaultTimezone = "UTC"

type Transport struct {
	clock clock
}

type flowConfig struct {
	Schedule cron.Schedule
	Timezone string
	Location *time.Location
}

type clock interface {
	Now() time.Time
	NewTimer(time.Duration) timer
}

type timer interface {
	C() <-chan time.Time
	Stop() bool
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func (realClock) NewTimer(d time.Duration) timer {
	if d < 0 {
		d = 0
	}
	return realTimer{Timer: time.NewTimer(d)}
}

type realTimer struct {
	*time.Timer
}

func (t realTimer) C() <-chan time.Time { return t.Timer.C }

func New() *Transport {
	return &Transport{clock: realClock{}}
}

func (t *Transport) Type() string { return "cron" }

func (t *Transport) ResponseContract() runtime.ResponseContract {
	return runtime.CronResponseContract()
}

func (t *Transport) ValidateFlow(flow runtime.Flow) error {
	_, err := readFlowConfig(flow)
	return err
}

func (t *Transport) Start(ctx context.Context, rt runtime.TransportRuntime) error {
	if t.clock == nil {
		t.clock = realClock{}
	}
	errCh := make(chan error, len(rt.Flows))
	for i := range rt.Flows {
		flow := rt.Flows[i]
		cfg, err := readFlowConfig(flow)
		if err != nil {
			return fmt.Errorf("flow %q: %w", flow.ID, err)
		}
		rt.Container.Logger().Info("Cron scheduler registered", "flow_id", flow.ID, "schedule", flow.Entrypoint.Config["schedule"], "timezone", cfg.Timezone)
		go func() {
			errCh <- t.runFlowLoop(ctx, rt, &flow, cfg)
		}()
	}

	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		return err
	}
}

func (t *Transport) Shutdown(context.Context) error { return nil }

func (t *Transport) runFlowLoop(ctx context.Context, rt runtime.TransportRuntime, flow *runtime.Flow, cfg flowConfig) error {
	cursor := t.clock.Now().In(cfg.Location)
	var mu sync.Mutex
	running := false
	fireCount := 0

	for {
		now := t.clock.Now().In(cfg.Location)
		next, skipped := nextScheduledAfter(cfg.Schedule, cursor, now)
		if skipped > 0 {
			rt.Container.Logger().Warn("Skipping missed cron fires", "flow_id", flow.ID, "skipped", skipped, "now", now.Format(time.RFC3339Nano))
		}
		timer := t.clock.NewTimer(next.Sub(now))
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C():
		}
		now = t.clock.Now().In(cfg.Location)
		if hasMissedFollowingTick(cfg.Schedule, next, now) {
			rt.Container.Logger().Warn("Skipping cron fire because scheduler woke after later ticks", "flow_id", flow.ID, "scheduled_at", next.Format(time.RFC3339Nano), "now", now.Format(time.RFC3339Nano))
			cursor = next
			continue
		}

		mu.Lock()
		if running {
			mu.Unlock()
			rt.Container.Logger().Warn("Skipping cron fire because previous run is still active", "flow_id", flow.ID, "scheduled_at", next.Format(time.RFC3339Nano))
			rt.Container.Metrics().RecordCronSkipped(ctx, flow.ID, "overlap")
			cursor = next
			continue
		}
		running = true
		fireCount++
		count := fireCount
		mu.Unlock()

		go func(scheduledAt time.Time, count int) {
			defer func() {
				mu.Lock()
				running = false
				mu.Unlock()
			}()
			t.runFlow(ctx, rt, flow, cfg.Location, scheduledAt, count)
		}(next, count)

		cursor = next
	}
}

func nextScheduledAfter(schedule cron.Schedule, cursor time.Time, now time.Time) (time.Time, int) {
	next := schedule.Next(cursor)
	skipped := 0
	for !next.After(now) {
		skipped++
		cursor = next
		next = schedule.Next(cursor)
	}
	return next, skipped
}

func hasMissedFollowingTick(schedule cron.Schedule, scheduledAt time.Time, now time.Time) bool {
	return !schedule.Next(scheduledAt).After(now)
}

func (t *Transport) runFlow(parent context.Context, rt runtime.TransportRuntime, flow *runtime.Flow, loc *time.Location, scheduledAt time.Time, fireCount int) {
	start := time.Now()
	execution := runtime.NewExecution(flow, rt.Container, rt.GlobalProperties, rt.NewValueStore())
	runCtx := parent
	cancel := func() {}
	if flow.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(parent, time.Duration(flow.Timeout)*time.Millisecond)
	}
	defer cancel()
	execution = execution.WithContext(runCtx)
	execution.AddValue("trigger.scheduled_at", scheduledAt.In(loc).Format(time.RFC3339Nano))
	execution.AddValue("trigger.fire_count", fireCount)

	err := rt.Executor.ExecuteSteps(execution)
	if err != nil {
		execution.Logger().Error("Cron flow execution failed", "error", err, "scheduled_at", scheduledAt.In(loc).Format(time.RFC3339Nano), "fire_count", fireCount)
	}
	execution.Metrics().RecordFlow(runCtx, flow.ID, classifyMetricOutcome(err), time.Since(start))
}

func readFlowConfig(flow runtime.Flow) (flowConfig, error) {
	config := flow.Entrypoint.Config
	scheduleExpr, _ := config["schedule"].(string)
	scheduleExpr = strings.TrimSpace(scheduleExpr)
	if scheduleExpr == "" {
		return flowConfig{}, fmt.Errorf("schedule must be a non-empty string")
	}
	if len(strings.Fields(scheduleExpr)) != 5 {
		return flowConfig{}, fmt.Errorf("schedule must use exactly 5 fields")
	}
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	parsed, err := parser.Parse(scheduleExpr)
	if err != nil {
		return flowConfig{}, fmt.Errorf("invalid schedule: %w", err)
	}

	timezone, _ := config["timezone"].(string)
	timezone = strings.TrimSpace(timezone)
	if timezone == "" {
		timezone = defaultTimezone
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return flowConfig{}, fmt.Errorf("invalid timezone %q: %w", timezone, err)
	}

	return flowConfig{Schedule: parsed, Timezone: timezone, Location: loc}, nil
}

func classifyMetricOutcome(err error) string {
	if err == nil {
		return "success"
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "timeout"
	}

	var flowErr *runtime.FlowError
	if errors.As(err, &flowErr) {
		if flowErr.Type == runtime.ErrorTypeTimeout ||
			flowErr.Code == string(runtime.ErrorCodeDeadlineExceeded) ||
			flowErr.Code == string(runtime.ErrorCodeContextCancelled) {
			return "timeout"
		}
	}

	return "error"
}
