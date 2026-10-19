package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	userAgent = "domain-watch (+https://github.com/alexhokl/domain-watch)"

	// rdapBootstrap redirects to the registry that is authoritative for a TLD.
	rdapBootstrap = "https://rdap.org/domain/"
)

// registryBases lists registries queried directly: going straight to the
// registry avoids a hop through a third-party redirector for the TLDs we care
// about most.
var registryBases = map[string]string{
	"com": "https://rdap.verisign.com/com/v1/domain/",
	"net": "https://rdap.verisign.com/net/v1/domain/",
}

// Result is what RDAP says about a domain right now.
type Result struct {
	Registered bool
	// Expiry is zero when the registry does not publish an expiration event.
	Expiry time.Time
	// Statuses are normalised (see normaliseStatus), sorted and de-duplicated.
	Statuses []string
}

type rdapClient struct {
	http *http.Client
	// baseOverride replaces the per-TLD lookup; tests point it at httptest.
	baseOverride string
}

func newRDAPClient() *rdapClient {
	return &rdapClient{http: &http.Client{Timeout: 15 * time.Second}}
}

func (c *rdapClient) baseFor(domain string) string {
	if c.baseOverride != "" {
		return c.baseOverride
	}
	tld := domain[strings.LastIndex(domain, ".")+1:]
	if base, ok := registryBases[tld]; ok {
		return base
	}
	return rdapBootstrap
}

type rdapResponse struct {
	Events []struct {
		Action string `json:"eventAction"`
		Date   string `json:"eventDate"`
	} `json:"events"`
	Status []string `json:"status"`
}

func (c *rdapClient) lookup(ctx context.Context, domain string) (Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseFor(domain)+url.PathEscape(domain), nil)
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Accept", "application/rdap+json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("rdap request for %s: %w", domain, err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return Result{Registered: false}, nil
	case resp.StatusCode != http.StatusOK:
		return Result{}, fmt.Errorf("rdap request for %s: unexpected status %s", domain, resp.Status)
	}

	var body rdapResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Result{}, fmt.Errorf("rdap response for %s: %w", domain, err)
	}
	return parseRDAP(body)
}

func parseRDAP(body rdapResponse) (Result, error) {
	res := Result{Registered: true, Statuses: normaliseStatuses(body.Status)}
	for _, e := range body.Events {
		if e.Action != "expiration" {
			continue
		}
		t, err := time.Parse(time.RFC3339, e.Date)
		if err != nil {
			return Result{}, errors.Join(errors.New("rdap expiration event has an invalid date"), err)
		}
		res.Expiry = t.UTC()
	}
	return res, nil
}

// normaliseStatus lower-cases a status and strips separators so that the RDAP
// spelling ("pending delete") and the EPP spelling ("pendingDelete") compare
// equal.
func normaliseStatus(s string) string {
	s = strings.ToLower(s)
	for _, sep := range []string{" ", "-", "_"} {
		s = strings.ReplaceAll(s, sep, "")
	}
	return s
}

func normaliseStatuses(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		n := normaliseStatus(s)
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
