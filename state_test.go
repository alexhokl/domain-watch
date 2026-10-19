package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileStoreReturnsNilOnFirstRun(t *testing.T) {
	s := FileStore{Dir: t.TempDir(), Prefix: "domain-watch"}

	got, err := s.Load(context.Background(), "example.com")

	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestFileStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := FileStore{Dir: dir, Prefix: "domain-watch"}
	want := State{
		Registered:  true,
		Expiry:      time.Date(2027, 5, 10, 13, 56, 7, 0, time.UTC),
		Statuses:    []string{"clienttransferprohibited"},
		LastChecked: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC),
	}

	require.NoError(t, s.Save(context.Background(), "example.com", want))
	got, err := s.Load(context.Background(), "example.com")

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, want, *got)
	assert.FileExists(t, filepath.Join(dir, "domain-watch", "example.com.json"))
}

func TestFileStoreRejectsCorruptState(t *testing.T) {
	dir := t.TempDir()
	s := FileStore{Dir: dir, Prefix: "p"}
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "p"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "p", "example.com.json"), []byte("{oops"), 0o600))

	_, err := s.Load(context.Background(), "example.com")

	assert.Error(t, err)
}
