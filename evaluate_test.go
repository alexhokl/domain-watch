package main

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	now    = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	expiry = time.Date(2027, 5, 10, 13, 56, 7, 0, time.UTC)
	warn   = []int{7, 30, 60}
)

func registered(exp time.Time, statuses ...string) Result {
	return Result{Registered: true, Expiry: exp, Statuses: statuses}
}

func previous(exp time.Time, checked time.Time, statuses ...string) *State {
	return &State{Registered: true, Expiry: exp, Statuses: statuses, LastChecked: checked}
}

func names(events []Event) []string {
	out := make([]string, 0, len(events))
	for _, e := range events {
		out = append(out, e.Name)
	}
	return out
}

func TestEvaluate(t *testing.T) {
	yesterday := now.Add(-24 * time.Hour)
	transferLock := "clienttransferprohibited"

	tests := []struct {
		name  string
		prev  *State
		cur   Result
		now   time.Time
		want  []string
		level slog.Level
	}{
		{
			name:  "first check of a healthy domain only records it",
			cur:   registered(expiry, transferLock),
			now:   now,
			want:  []string{"first_check"},
			level: levelNotice,
		},
		{
			name:  "unchanged domain reports no change",
			prev:  previous(expiry, yesterday, transferLock),
			cur:   registered(expiry, transferLock),
			now:   now,
			want:  []string{"no_change"},
			level: levelNotice,
		},
		{
			name:  "an unregistered domain is reported as available",
			prev:  previous(expiry, yesterday, transferLock),
			cur:   Result{Registered: false},
			now:   now,
			want:  []string{"domain_available"},
			level: slog.LevelError,
		},
		{
			name:  "an unregistered domain is reported as available on the first check too",
			cur:   Result{Registered: false},
			now:   now,
			want:  []string{"domain_available"},
			level: slog.LevelError,
		},
		{
			name:  "a renewed domain reports the new expiry",
			prev:  previous(expiry, yesterday, transferLock),
			cur:   registered(expiry.AddDate(1, 0, 0), transferLock),
			now:   now,
			want:  []string{"expiry_changed"},
			level: slog.LevelWarn,
		},
		{
			name:  "a changed status set is reported",
			prev:  previous(expiry, yesterday, transferLock),
			cur:   registered(expiry, transferLock, "serverhold"),
			now:   now,
			want:  []string{"status_changed"},
			level: slog.LevelWarn,
		},
		{
			name:  "entering redemption reports it once without a generic status change",
			prev:  previous(expiry, yesterday, transferLock),
			cur:   registered(expiry, transferLock, "redemptionperiod"),
			now:   now,
			want:  []string{"redemption_period"},
			level: slog.LevelWarn,
		},
		{
			name:  "staying in redemption is not reported again",
			prev:  previous(expiry, yesterday, "redemptionperiod"),
			cur:   registered(expiry, "redemptionperiod"),
			now:   now,
			want:  []string{"no_change"},
			level: levelNotice,
		},
		{
			name:  "pending delete is reported on every run",
			prev:  previous(expiry, yesterday, "pendingdelete"),
			cur:   registered(expiry, "pendingdelete"),
			now:   now,
			want:  []string{"pending_delete"},
			level: slog.LevelError,
		},
		{
			name:  "a domain registered by someone after being available is reported",
			prev:  &State{Registered: false, LastChecked: yesterday},
			cur:   registered(expiry, transferLock),
			now:   now,
			want:  []string{"domain_registered"},
			level: slog.LevelWarn,
		},
		{
			name:  "crossing the 60 day threshold warns",
			prev:  previous(now.AddDate(0, 0, 59), now.Add(-48*time.Hour), transferLock),
			cur:   registered(now.AddDate(0, 0, 59), transferLock),
			now:   now,
			want:  []string{"expiring_soon"},
			level: slog.LevelWarn,
		},
		{
			name:  "staying under a threshold does not warn again",
			prev:  previous(now.AddDate(0, 0, 50), yesterday, transferLock),
			cur:   registered(now.AddDate(0, 0, 50), transferLock),
			now:   now,
			want:  []string{"no_change"},
			level: levelNotice,
		},
		{
			name:  "the first check of a domain already under a threshold warns",
			cur:   registered(now.AddDate(0, 0, 20), transferLock),
			now:   now,
			want:  []string{"expiring_soon"},
			level: slog.LevelWarn,
		},
		{
			name:  "an expired but still registered domain stays quiet after the first warning",
			prev:  previous(now.Add(-2*time.Hour), now.Add(-26*time.Hour), transferLock),
			cur:   registered(now.Add(-2*time.Hour), transferLock),
			now:   now,
			want:  []string{"no_change"},
			level: levelNotice,
		},
		{
			name:  "a registry that hides the expiry never warns about it",
			prev:  previous(time.Time{}, yesterday, transferLock),
			cur:   registered(time.Time{}, transferLock),
			now:   now,
			want:  []string{"no_change"},
			level: levelNotice,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evaluate(tt.prev, tt.cur, tt.now, warn)
			assert.Equal(t, tt.want, names(got))
			require.NotEmpty(t, got)
			assert.Equal(t, tt.level, got[0].Level)
		})
	}
}

func TestEvaluateReportsTheSmallestCrossedThreshold(t *testing.T) {
	// A missed run can skip several thresholds at once; one event for the
	// closest one is enough.
	exp := now.AddDate(0, 0, 5)
	prev := previous(exp, now.AddDate(0, 0, -70), "clienttransferprohibited")
	cur := registered(exp, "clienttransferprohibited")

	got := evaluate(prev, cur, now, warn)

	require.Len(t, got, 1)
	var threshold int64
	for _, e := range got {
		if e.Name != "expiring_soon" {
			continue
		}
		for _, a := range e.Attrs {
			if a.Key == "threshold_days" {
				threshold = a.Value.Int64()
			}
		}
	}
	assert.EqualValues(t, 7, threshold)
}

func TestDaysLeft(t *testing.T) {
	tests := []struct {
		name string
		exp  time.Time
		want int
	}{
		{"exactly three days", now.AddDate(0, 0, 3), 3},
		{"just under three days rounds down", now.AddDate(0, 0, 3).Add(-time.Minute), 2},
		{"expired earlier today is negative", now.Add(-2 * time.Hour), -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, daysLeft(tt.exp, now))
		})
	}
}
