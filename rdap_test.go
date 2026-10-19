package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const registeredBody = `{
  "events": [
    {"eventAction": "registration", "eventDate": "2022-05-10T13:56:07Z"},
    {"eventAction": "expiration", "eventDate": "2027-05-10T13:56:07Z"},
    {"eventAction": "last changed", "eventDate": "2026-04-27T09:21:22Z"}
  ],
  "status": ["client transfer prohibited", "pending delete"]
}`

func newTestClient(t *testing.T, handler http.HandlerFunc) *rdapClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := newRDAPClient()
	c.baseOverride = srv.URL + "/domain/"
	return c
}

func TestLookup(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		want    Result
		wantErr bool
	}{
		{
			name:   "a registered domain returns expiry and normalised statuses",
			status: http.StatusOK,
			body:   registeredBody,
			want: Result{
				Registered: true,
				Expiry:     time.Date(2027, 5, 10, 13, 56, 7, 0, time.UTC),
				Statuses:   []string{"clienttransferprohibited", "pendingdelete"},
			},
		},
		{
			name:   "a 404 means the domain is not registered",
			status: http.StatusNotFound,
			want:   Result{Registered: false},
		},
		{
			name:   "a registered domain without an expiration event has a zero expiry",
			status: http.StatusOK,
			body:   `{"events": [], "status": ["active"]}`,
			want:   Result{Registered: true, Statuses: []string{"active"}},
		},
		{name: "rate limiting is an error", status: http.StatusTooManyRequests, wantErr: true},
		{name: "a server error is an error", status: http.StatusBadGateway, wantErr: true},
		{name: "malformed JSON is an error", status: http.StatusOK, body: `{not json`, wantErr: true},
		{
			name:    "an invalid expiration date is an error",
			status:  http.StatusOK,
			body:    `{"events": [{"eventAction": "expiration", "eventDate": "soon"}]}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			})

			got, err := c.lookup(context.Background(), "example.com")

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLookupSendsAnRDAPRequestForTheDomain(t *testing.T) {
	var path, accept, agent string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		path, accept, agent = r.URL.Path, r.Header.Get("Accept"), r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusNotFound)
	})

	_, err := c.lookup(context.Background(), "example.com")

	require.NoError(t, err)
	assert.Equal(t, "/domain/example.com", path)
	assert.Equal(t, "application/rdap+json", accept)
	assert.Contains(t, agent, "domain-watch")
}

func TestBaseFor(t *testing.T) {
	c := newRDAPClient()
	assert.Equal(t, "https://rdap.verisign.com/com/v1/domain/", c.baseFor("ottimo-consulting.com"))
	assert.Equal(t, "https://rdap.verisign.com/net/v1/domain/", c.baseFor("example.net"))
	assert.Equal(t, rdapBootstrap, c.baseFor("example.hk"))
}

func TestNormaliseStatuses(t *testing.T) {
	got := normaliseStatuses([]string{"pending delete", "pendingDelete", "Client Transfer Prohibited", ""})
	assert.Equal(t, []string{"clienttransferprohibited", "pendingdelete"}, got)
}
