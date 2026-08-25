package cloudflared

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func tokenServer(t *testing.T, existing string, secret string, posts *[]map[string]any) *Exposer {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			if existing == "" {
				_, _ = w.Write([]byte(`{"success":true,"errors":[],"result":[]}`))
				return
			}
			_, _ = w.Write([]byte(`{"success":true,"errors":[],"result":[
				{"id":"t1","name":"` + existing + `","client_id":"abc.access"}]}`))
		case http.MethodPost:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			*posts = append(*posts, body)
			_, _ = w.Write([]byte(`{"success":true,"errors":[],"result":
				{"id":"t2","name":"mcp-remote-bridge","client_id":"new.access","client_secret":"` + secret + `"}}`))
		}
	}))
	t.Cleanup(srv.Close)
	e := New("acc", "zone", "tun", "test-token")
	e.BaseURL = srv.URL
	return e
}

func TestCreateServiceTokenReturnsTheSecret(t *testing.T) {
	var posts []map[string]any
	e := tokenServer(t, "", "the-secret-value", &posts)

	got, err := e.CreateServiceToken("mcp-remote-bridge")
	if err != nil {
		t.Fatalf("CreateServiceToken: %v", err)
	}
	if got.ClientID != "new.access" {
		t.Errorf("ClientID = %q", got.ClientID)
	}
	// The whole reason for using the API: the dashboard shows this once.
	if got.Secret != "the-secret-value" {
		t.Errorf("Secret = %q, want the value from the creation response", got.Secret)
	}
	if len(posts) != 1 || posts[0]["name"] != "mcp-remote-bridge" {
		t.Errorf("unexpected creation body: %v", posts)
	}
}

// Two tokens of the same name are two indistinguishable credentials, and no way
// to tell which is in use.
func TestCreateServiceTokenRefusesADuplicateName(t *testing.T) {
	var posts []map[string]any
	e := tokenServer(t, "mcp-remote-bridge", "x", &posts)

	_, err := e.CreateServiceToken("mcp-remote-bridge")

	if err == nil {
		t.Fatal("a second token with an existing name was created")
	}
	if len(posts) != 0 {
		t.Errorf("a creation was attempted despite the duplicate: %v", posts)
	}
	if !strings.Contains(err.Error(), "abc.access") {
		t.Errorf("the error should name the existing token so the user can act: %v", err)
	}
}

// A token created without a secret is unrecoverable, so reporting success would
// strand the user with a credential they can never use.
func TestCreateServiceTokenFailsWhenNoSecretComesBack(t *testing.T) {
	var posts []map[string]any
	e := tokenServer(t, "", "", &posts)

	_, err := e.CreateServiceToken("mcp-remote-bridge")

	if err == nil {
		t.Fatal("a token with no secret was reported as created")
	}
	if !strings.Contains(err.Error(), "delete") {
		t.Errorf("the error should say how to recover: %v", err)
	}
}

func TestFindServiceToken(t *testing.T) {
	var posts []map[string]any

	e := tokenServer(t, "mcp-remote-bridge", "x", &posts)
	got, err := e.FindServiceToken("mcp-remote-bridge")
	if err != nil || got == nil {
		t.Fatalf("FindServiceToken = %v, %v", got, err)
	}
	if got.Secret != "" {
		t.Error("listing returned a secret; only creation does")
	}

	if got, _ := e.FindServiceToken("something-else"); got != nil {
		t.Errorf("FindServiceToken matched the wrong name: %v", got)
	}
}
