package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"time"

	"cloud.google.com/go/storage"
)

// State is what the previous run saw; evaluate compares against it.
type State struct {
	Registered  bool      `json:"registered"`
	Expiry      time.Time `json:"expiry"`
	Statuses    []string  `json:"statuses"`
	LastChecked time.Time `json:"last_checked"`
}

// Store persists one State per domain. Load returns (nil, nil) on the first
// run for a domain.
type Store interface {
	Load(ctx context.Context, domain string) (*State, error)
	Save(ctx context.Context, domain string, st State) error
}

func objectName(prefix, domain string) string {
	return path.Join(prefix, domain+".json")
}

// FileStore keeps state under a local directory, for running outside GCP.
type FileStore struct {
	Dir    string
	Prefix string
}

func (s FileStore) file(domain string) string {
	return filepath.Join(s.Dir, filepath.FromSlash(objectName(s.Prefix, domain)))
}

func (s FileStore) Load(_ context.Context, domain string) (*State, error) {
	b, err := os.ReadFile(s.file(domain))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return decodeState(b)
}

func (s FileStore) Save(_ context.Context, domain string, st State) error {
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	name := s.file(domain)
	if err := os.MkdirAll(filepath.Dir(name), 0o750); err != nil {
		return err
	}
	return os.WriteFile(name, b, 0o600)
}

// GCSStore keeps state as objects in a Cloud Storage bucket.
type GCSStore struct {
	Bucket *storage.BucketHandle
	Prefix string
}

func (s GCSStore) Load(ctx context.Context, domain string) (*State, error) {
	r, err := s.Bucket.Object(objectName(s.Prefix, domain)).NewReader(ctx)
	if errors.Is(err, storage.ErrObjectNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()

	b, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return decodeState(b)
}

func (s GCSStore) Save(ctx context.Context, domain string, st State) error {
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	w := s.Bucket.Object(objectName(s.Prefix, domain)).NewWriter(ctx)
	w.ContentType = "application/json"
	if _, err := w.Write(b); err != nil {
		_ = w.Close()
		return err
	}
	return w.Close()
}

func decodeState(b []byte) (*State, error) {
	var st State
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, fmt.Errorf("decode state: %w", err)
	}
	return &st, nil
}
