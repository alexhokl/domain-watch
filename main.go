// domain-watch checks domains over RDAP and logs when they are renewed, change
// status, near expiry or are released. It runs once and exits, so it is meant
// to be started by a scheduler (Cloud Run job plus Cloud Scheduler).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"cloud.google.com/go/storage"
)

func main() {
	logger := newLogger(os.Stdout)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := run(ctx, logger); err != nil {
		logger.Error("run failed", slog.String("event", "run_failed"), slog.String("error", err.Error()))
		stop()
		os.Exit(1) //nolint:gocritic // stop is called explicitly above
	}
}

func run(ctx context.Context, logger *slog.Logger) error {
	cfg, err := loadConfig(os.Getenv)
	if err != nil {
		return err
	}

	store, closeStore, err := newStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer closeStore()

	rdap := newRDAPClient()
	now := time.Now().UTC()

	var errs []error
	for _, domain := range cfg.Domains {
		if err := checkDomain(ctx, logger, store, rdap, domain, now, cfg.WarnDays); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func newStore(ctx context.Context, cfg Config) (Store, func(), error) {
	if cfg.StateBucket == "" {
		return FileStore{Dir: cfg.StateDir, Prefix: cfg.StatePrefix}, func() {}, nil
	}
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("create storage client: %w", err)
	}
	return GCSStore{Bucket: client.Bucket(cfg.StateBucket), Prefix: cfg.StatePrefix},
		func() { _ = client.Close() }, nil
}

type lookuper interface {
	lookup(ctx context.Context, domain string) (Result, error)
}

// checkDomain looks the domain up, logs what changed and saves the new state.
// On a lookup failure it leaves the saved state untouched so a transient RDAP
// outage cannot hide a change that happened in the meantime.
func checkDomain(ctx context.Context, logger *slog.Logger, store Store, rdap lookuper,
	domain string, now time.Time, warnDays []int,
) error {
	cur, err := rdap.lookup(ctx, domain)
	if err != nil {
		logger.Error("check failed",
			slog.String("event", "check_failed"),
			slog.String("domain", domain),
			slog.String("error", err.Error()))
		return err
	}

	prev, err := store.Load(ctx, domain)
	if err != nil {
		logger.Error("load state failed",
			slog.String("event", "check_failed"),
			slog.String("domain", domain),
			slog.String("error", err.Error()))
		return fmt.Errorf("load state for %s: %w", domain, err)
	}

	for _, e := range evaluate(prev, cur, now, warnDays) {
		attrs := []slog.Attr{
			slog.String("event", e.Name),
			slog.String("domain", domain),
			slog.Bool("registered", cur.Registered),
			slog.Any("statuses", cur.Statuses),
		}
		if !cur.Expiry.IsZero() {
			attrs = append(attrs,
				slog.String("expiry", formatTime(cur.Expiry)),
				slog.Int("days_left", daysLeft(cur.Expiry, now)))
		}
		attrs = append(attrs, e.Attrs...)
		logger.LogAttrs(ctx, e.Level, e.Name, attrs...)
	}

	st := State{Registered: cur.Registered, Expiry: cur.Expiry, Statuses: cur.Statuses, LastChecked: now}
	if err := store.Save(ctx, domain, st); err != nil {
		logger.Error("save state failed",
			slog.String("event", "check_failed"),
			slog.String("domain", domain),
			slog.String("error", err.Error()))
		return fmt.Errorf("save state for %s: %w", domain, err)
	}
	return nil
}
