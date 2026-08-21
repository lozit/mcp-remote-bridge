package bridge

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAccessCredentialsConfigured(t *testing.T) {
	tests := []struct {
		name string
		c    AccessCredentials
		want bool
	}{
		{"both", AccessCredentials{"id", "secret"}, true},
		{"neither", AccessCredentials{}, false},
		// A half-configured token is sent, rejected, and reads as "the MCP is
		// down" — a misleading red instead of a visible misconfiguration.
		{"id only", AccessCredentials{ClientID: "id"}, false},
		{"secret only", AccessCredentials{ClientSecret: "secret"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.c.Configured(); got != tt.want {
				t.Errorf("Configured() = %v, want %v", got, tt.want)
			}
		})
	}
}

// nil is meaningful: it is how the access-policy check asks "does this answer
// WITHOUT credentials?".
func TestDecorateIsNilWithoutCredentials(t *testing.T) {
	if (AccessCredentials{}).Decorate() != nil {
		t.Error("Decorate() returned a decorator with no credentials configured")
	}
	if (AccessCredentials{ClientID: "id"}).Decorate() != nil {
		t.Error("Decorate() returned a decorator with only half the credentials")
	}
	if (AccessCredentials{"id", "secret"}).Decorate() == nil {
		t.Error("Decorate() returned nil with credentials configured")
	}
}

func TestDecorateSetsBothHeadersAndNothingElse(t *testing.T) {
	var got *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Clone(r.Context())
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":1,"message":"stop"}}`))
	}))
	defer srv.Close()

	const secret = "SERVICE-TOKEN-SECRET-VALUE"
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	(AccessCredentials{ClientID: "abc.access", ClientSecret: secret}).Decorate()(req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if got.Header.Get(AccessClientIDHeader) != "abc.access" {
		t.Errorf("%s = %q", AccessClientIDHeader, got.Header.Get(AccessClientIDHeader))
	}
	if got.Header.Get(AccessClientSecretHeader) != secret {
		t.Errorf("%s was not set", AccessClientSecretHeader)
	}
	// The secret must not reach the URL: query strings land in server logs and
	// in every proxy along the way.
	if u := got.URL.String(); strings.Contains(u, secret) {
		t.Errorf("the secret leaked into the URL: %s", u)
	}
}
