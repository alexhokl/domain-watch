package main

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const (
	defaultStatePrefix = "domain-watch"
	defaultWarnDays    = "60,30,7"
)

// Config is the run configuration, read from environment variables so the same
// image works for Cloud Run (bucket state) and local runs (directory state).
type Config struct {
	Domains     []string
	StateBucket string
	StateDir    string
	StatePrefix string
	// WarnDays is sorted ascending, e.g. [7 30 60].
	WarnDays []int
}

func loadConfig(getenv func(string) string) (Config, error) {
	cfg := Config{
		StateBucket: strings.TrimSpace(getenv("STATE_BUCKET")),
		StateDir:    strings.TrimSpace(getenv("STATE_DIR")),
		StatePrefix: strings.Trim(strings.TrimSpace(getenv("STATE_PREFIX")), "/"),
	}
	if cfg.StatePrefix == "" {
		cfg.StatePrefix = defaultStatePrefix
	}

	for _, d := range strings.Split(getenv("DOMAINS"), ",") {
		d = strings.ToLower(strings.TrimSpace(d))
		if d != "" {
			cfg.Domains = append(cfg.Domains, d)
		}
	}
	if len(cfg.Domains) == 0 {
		return Config{}, errors.New("DOMAINS is required (comma separated list of domains)")
	}

	switch {
	case cfg.StateBucket == "" && cfg.StateDir == "":
		return Config{}, errors.New("one of STATE_BUCKET or STATE_DIR is required")
	case cfg.StateBucket != "" && cfg.StateDir != "":
		return Config{}, errors.New("STATE_BUCKET and STATE_DIR are mutually exclusive")
	}

	warn := strings.TrimSpace(getenv("WARN_DAYS"))
	if warn == "" {
		warn = defaultWarnDays
	}
	days, err := parseWarnDays(warn)
	if err != nil {
		return Config{}, err
	}
	cfg.WarnDays = days

	return cfg, nil
}

func parseWarnDays(s string) ([]int, error) {
	var days []int
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("WARN_DAYS: %q is not a non-negative integer", part)
		}
		days = append(days, n)
	}
	sort.Ints(days)
	return days, nil
}
