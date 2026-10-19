package main

import (
	"log/slog"
	"slices"
	"time"
)

// levelNotice sits between Info and Warn and maps to Cloud Logging's NOTICE.
const levelNotice = slog.Level(2)

const (
	statusPendingDelete    = "pendingdelete"
	statusRedemptionPeriod = "redemptionperiod"
)

// Event is something worth logging after comparing the previous and current
// state. Events at Warn and above are what the alert policy emails.
type Event struct {
	Name  string
	Level slog.Level
	Attrs []slog.Attr
}

// evaluate is pure: all inputs are passed in so every transition can be tested
// without a network, a clock or storage. warnDays must be sorted ascending.
func evaluate(prev *State, cur Result, now time.Time, warnDays []int) []Event {
	if !cur.Registered {
		return []Event{{Name: "domain_available", Level: slog.LevelError}}
	}

	var events []Event
	add := func(name string, level slog.Level, attrs ...slog.Attr) {
		events = append(events, Event{Name: name, Level: level, Attrs: attrs})
	}

	prevRegistered := prev != nil && prev.Registered

	if prev != nil && !prev.Registered {
		add("domain_registered", slog.LevelWarn)
	}

	if slices.Contains(cur.Statuses, statusPendingDelete) {
		// Reported on every run while pending: release is at most days away and
		// the alert policy rate-limits the email.
		add("pending_delete", slog.LevelError)
	}
	if slices.Contains(cur.Statuses, statusRedemptionPeriod) &&
		(prev == nil || !slices.Contains(prev.Statuses, statusRedemptionPeriod)) {
		add("redemption_period", slog.LevelWarn)
	}

	if prevRegistered {
		if !prev.Expiry.Equal(cur.Expiry) {
			add("expiry_changed", slog.LevelWarn,
				slog.String("previous_expiry", formatTime(prev.Expiry)))
		}
		if !slices.Equal(withoutLifecycle(prev.Statuses), withoutLifecycle(cur.Statuses)) {
			add("status_changed", slog.LevelWarn,
				slog.Any("previous_statuses", prev.Statuses))
		}
	}

	if t, ok := crossedThreshold(prev, cur, now, warnDays); ok {
		add("expiring_soon", slog.LevelWarn, slog.Int("threshold_days", t))
	}

	if len(events) == 0 {
		if prev == nil {
			add("first_check", levelNotice)
		} else {
			add("no_change", levelNotice)
		}
	}
	return events
}

// withoutLifecycle drops the statuses that have their own events so a move into
// redemption does not also report a generic status change.
func withoutLifecycle(statuses []string) []string {
	return slices.DeleteFunc(slices.Clone(statuses), func(s string) bool {
		return s == statusPendingDelete || s == statusRedemptionPeriod
	})
}

// crossedThreshold returns the smallest warning threshold (in days) that the
// expiry has fallen under since the previous run, so each threshold alerts once
// rather than daily. The previous days-left is computed from the current expiry
// at the previous check time, which resets the thresholds after a renewal.
func crossedThreshold(prev *State, cur Result, now time.Time, warnDays []int) (int, bool) {
	if cur.Expiry.IsZero() {
		return 0, false
	}
	left := daysLeft(cur.Expiry, now)
	for _, t := range warnDays {
		if left > t {
			continue
		}
		if prev == nil || !prev.Registered || daysLeft(cur.Expiry, prev.LastChecked) > t {
			return t, true
		}
	}
	return 0, false
}

func daysLeft(expiry, at time.Time) int {
	d := expiry.Sub(at)
	days := int(d / (24 * time.Hour))
	if d < 0 && d%(24*time.Hour) != 0 {
		days-- // floor, so "expired 2 hours ago" is -1 rather than 0
	}
	return days
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
