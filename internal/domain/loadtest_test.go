package domain

import (
	"errors"
	"math"
	"testing"
	"time"
)

func TestLoadPlanApprove(t *testing.T) {
	now := time.Date(2026, 10, 10, 1, 0, 0, 0, time.UTC)
	plan := LoadTestPlan{ID: "plan-1", Status: LoadPlanDraft, Version: 1}
	if err := plan.Approve("supervisor-1", 4, 16, now); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	if plan.Status != LoadPlanApproved {
		t.Fatalf("status = %s, want approved", plan.Status)
	}
	if plan.ApprovedBy != "supervisor-1" {
		t.Fatalf("approved_by = %q", plan.ApprovedBy)
	}
	if !plan.ApprovedAt.Equal(now) || plan.ApprovedAt.Location() != time.UTC {
		t.Fatalf("approved_at = %s, want UTC %s", plan.ApprovedAt, now)
	}
	if plan.Version != 2 {
		t.Fatalf("version = %d, want 2", plan.Version)
	}
}

func TestLoadPlanApprovalRequiresCasesAndChannels(t *testing.T) {
	tests := []struct {
		name     string
		cases    int
		channels int
	}{
		{name: "no cases or channels", cases: 0, channels: 0},
		{name: "no cases", cases: 0, channels: 8},
		{name: "no channels", cases: 3, channels: 0},
		{name: "negative cases", cases: -1, channels: 2},
		{name: "negative channels", cases: 2, channels: -1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := LoadTestPlan{Status: LoadPlanDraft, Version: 4}
			err := plan.Approve("supervisor", test.cases, test.channels, time.Now())
			if !errors.Is(err, ErrConflict) {
				t.Fatalf("Approve() error = %v, want conflict", err)
			}
			if plan.Status != LoadPlanDraft || plan.Version != 4 || plan.ApprovedBy != "" || !plan.ApprovedAt.IsZero() {
				t.Fatalf("failed approval mutated plan: %+v", plan)
			}
		})
	}
}

func TestLoadPlanCannotBeApprovedTwiceOrAfterArchive(t *testing.T) {
	for _, status := range []LoadPlanStatus{LoadPlanApproved, LoadPlanArchived, "unknown"} {
		plan := LoadTestPlan{Status: status, Version: 6}
		err := plan.Approve("supervisor", 1, 1, time.Now())
		if !errors.Is(err, ErrConflict) {
			t.Fatalf("status %s error = %v, want conflict", status, err)
		}
		if plan.Status != status || plan.Version != 6 {
			t.Fatalf("failed approval mutated plan: %+v", plan)
		}
	}
}

func TestLoadRunSuccessfulLifecycle(t *testing.T) {
	now := time.Date(2026, 10, 12, 3, 0, 0, 0, time.UTC)
	run := LoadTestRun{ID: "run-1", Status: LoadRunPending, Version: 1}
	if err := run.Transition(LoadRunRunning, now); err != nil {
		t.Fatalf("start error = %v", err)
	}
	if run.Status != LoadRunRunning || !run.StartedAt.Equal(now) || run.Version != 2 {
		t.Fatalf("unexpected running state: %+v", run)
	}
	if !run.CompletedAt.IsZero() {
		t.Fatalf("running run has completed_at = %s", run.CompletedAt)
	}
	if err := run.Transition(LoadRunEvaluating, now.Add(time.Hour)); err != nil {
		t.Fatalf("evaluate transition error = %v", err)
	}
	if run.Status != LoadRunEvaluating || run.Version != 3 {
		t.Fatalf("unexpected evaluating state: %+v", run)
	}
	if err := run.Transition(LoadRunPassed, now.Add(2*time.Hour)); err != nil {
		t.Fatalf("pass transition error = %v", err)
	}
	if run.Status != LoadRunPassed || run.Version != 4 {
		t.Fatalf("unexpected passed state: %+v", run)
	}
	if !run.CompletedAt.Equal(now.Add(2 * time.Hour)) {
		t.Fatalf("completed_at = %s", run.CompletedAt)
	}
}

func TestLoadRunFailureAndCancellationLifecycles(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		name  string
		steps []LoadRunStatus
		final LoadRunStatus
	}{
		{name: "threshold failure", steps: []LoadRunStatus{LoadRunRunning, LoadRunEvaluating, LoadRunFailed}, final: LoadRunFailed},
		{name: "operator cancellation", steps: []LoadRunStatus{LoadRunRunning, LoadRunCanceled}, final: LoadRunCanceled},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			run := LoadTestRun{Status: LoadRunPending, Version: 1}
			for index, status := range test.steps {
				if err := run.Transition(status, now.Add(time.Duration(index)*time.Minute)); err != nil {
					t.Fatalf("transition %d to %s error = %v", index, status, err)
				}
			}
			if run.Status != test.final {
				t.Fatalf("final status = %s, want %s", run.Status, test.final)
			}
			if run.CompletedAt.IsZero() {
				t.Fatal("terminal run has zero completed_at")
			}
		})
	}
}

func TestLoadRunRejectsIllegalTransitions(t *testing.T) {
	tests := []struct {
		from LoadRunStatus
		to   LoadRunStatus
	}{
		{from: LoadRunPending, to: LoadRunPassed},
		{from: LoadRunPending, to: LoadRunEvaluating},
		{from: LoadRunRunning, to: LoadRunPassed},
		{from: LoadRunRunning, to: LoadRunFailed},
		{from: LoadRunEvaluating, to: LoadRunCanceled},
		{from: LoadRunPassed, to: LoadRunRunning},
		{from: LoadRunFailed, to: LoadRunRunning},
		{from: LoadRunCanceled, to: LoadRunRunning},
	}
	for _, test := range tests {
		name := string(test.from) + "_to_" + string(test.to)
		t.Run(name, func(t *testing.T) {
			run := LoadTestRun{Status: test.from, Version: 11}
			err := run.Transition(test.to, time.Now())
			if !errors.Is(err, ErrConflict) {
				t.Fatalf("Transition() error = %v, want conflict", err)
			}
			if run.Status != test.from || run.Version != 11 || !run.StartedAt.IsZero() || !run.CompletedAt.IsZero() {
				t.Fatalf("failed transition mutated run: %+v", run)
			}
		})
	}
}

func TestSensorReadingValidate(t *testing.T) {
	now := time.Now().UTC()
	channel := SensorChannel{ID: "channel-1", MinValue: -5, MaxValue: 5}
	tests := []struct {
		name    string
		reading SensorReading
		valid   bool
	}{
		{name: "valid zero", reading: SensorReading{RunID: "run-1", ChannelID: "channel-1", Sequence: 1, Value: 0, ObservedAt: now}, valid: true},
		{name: "valid lower boundary", reading: SensorReading{RunID: "run-1", ChannelID: "channel-1", Sequence: 2, Value: -5, ObservedAt: now}, valid: true},
		{name: "valid upper boundary", reading: SensorReading{RunID: "run-1", ChannelID: "channel-1", Sequence: 3, Value: 5, ObservedAt: now}, valid: true},
		{name: "missing run", reading: SensorReading{ChannelID: "channel-1", Sequence: 1, ObservedAt: now}},
		{name: "wrong channel", reading: SensorReading{RunID: "run-1", ChannelID: "channel-2", Sequence: 1, ObservedAt: now}},
		{name: "zero sequence", reading: SensorReading{RunID: "run-1", ChannelID: "channel-1", Sequence: 0, ObservedAt: now}},
		{name: "negative sequence", reading: SensorReading{RunID: "run-1", ChannelID: "channel-1", Sequence: -1, ObservedAt: now}},
		{name: "missing timestamp", reading: SensorReading{RunID: "run-1", ChannelID: "channel-1", Sequence: 1}},
		{name: "not a number", reading: SensorReading{RunID: "run-1", ChannelID: "channel-1", Sequence: 1, Value: math.NaN(), ObservedAt: now}},
		{name: "positive infinity", reading: SensorReading{RunID: "run-1", ChannelID: "channel-1", Sequence: 1, Value: math.Inf(1), ObservedAt: now}},
		{name: "negative infinity", reading: SensorReading{RunID: "run-1", ChannelID: "channel-1", Sequence: 1, Value: math.Inf(-1), ObservedAt: now}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.reading.Validate(channel)
			if test.valid && err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if !test.valid && !errors.Is(err, ErrInvalid) {
				t.Fatalf("Validate() error = %v, want invalid", err)
			}
		})
	}
}

func TestSensorReadingWithinInclusiveThresholds(t *testing.T) {
	channel := SensorChannel{MinValue: -2.5, MaxValue: 8.25}
	tests := []struct {
		value float64
		want  bool
	}{
		{value: -3, want: false},
		{value: -2.5001, want: false},
		{value: -2.5, want: true},
		{value: 0, want: true},
		{value: 8.25, want: true},
		{value: 8.2501, want: false},
		{value: 9, want: false},
	}
	for _, test := range tests {
		reading := SensorReading{Value: test.value}
		if got := reading.Within(channel); got != test.want {
			t.Fatalf("Within(%v) = %v, want %v", test.value, got, test.want)
		}
	}
}
