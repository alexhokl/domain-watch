package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeLookup struct {
	res Result
	err error
}

func (f fakeLookup) lookup(context.Context, string) (Result, error) { return f.res, f.err }

func logLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &m))
		out = append(out, m)
	}
	return out
}

func TestCheckDomainLogsCloudLoggingFieldsAndSavesState(t *testing.T) {
	var buf bytes.Buffer
	store := FileStore{Dir: t.TempDir(), Prefix: "p"}
	cur := registered(expiry, "clienttransferprohibited")

	err := checkDomain(context.Background(), newLogger(&buf), store, fakeLookup{res: cur},
		"example.com", now, warn)

	require.NoError(t, err)
	lines := logLines(t, &buf)
	require.Len(t, lines, 1)
	assert.Equal(t, "NOTICE", lines[0]["severity"])
	assert.Equal(t, "first_check", lines[0]["message"])
	assert.Equal(t, "first_check", lines[0]["event"])
	assert.Equal(t, "example.com", lines[0]["domain"])
	assert.NotEmpty(t, lines[0]["timestamp"])

	saved, err := store.Load(context.Background(), "example.com")
	require.NoError(t, err)
	require.NotNil(t, saved)
	assert.True(t, saved.Expiry.Equal(expiry))
	assert.True(t, saved.LastChecked.Equal(now))
}

func TestCheckDomainLogsAnErrorWhenAvailable(t *testing.T) {
	var buf bytes.Buffer
	store := FileStore{Dir: t.TempDir(), Prefix: "p"}

	err := checkDomain(context.Background(), newLogger(&buf), store, fakeLookup{res: Result{}},
		"example.com", now, warn)

	require.NoError(t, err)
	lines := logLines(t, &buf)
	require.Len(t, lines, 1)
	assert.Equal(t, "ERROR", lines[0]["severity"])
	assert.Equal(t, "domain_available", lines[0]["event"])
}

func TestCheckDomainKeepsPreviousStateWhenLookupFails(t *testing.T) {
	var buf bytes.Buffer
	store := FileStore{Dir: t.TempDir(), Prefix: "p"}
	before := State{Registered: true, Expiry: expiry, LastChecked: now.Add(-24 * time.Hour)}
	require.NoError(t, store.Save(context.Background(), "example.com", before))

	err := checkDomain(context.Background(), newLogger(&buf), store,
		fakeLookup{err: errors.New("boom")}, "example.com", now, warn)

	require.Error(t, err)
	lines := logLines(t, &buf)
	require.Len(t, lines, 1)
	assert.Equal(t, "ERROR", lines[0]["severity"])
	assert.Equal(t, "check_failed", lines[0]["event"])

	after, err := store.Load(context.Background(), "example.com")
	require.NoError(t, err)
	require.NotNil(t, after)
	assert.True(t, after.LastChecked.Equal(before.LastChecked))
}

func TestSeverity(t *testing.T) {
	assert.Equal(t, "ERROR", severity(8))
	assert.Equal(t, "WARNING", severity(4))
	assert.Equal(t, "NOTICE", severity(levelNotice))
	assert.Equal(t, "INFO", severity(0))
	assert.Equal(t, "DEBUG", severity(-4))
}
