package cloudflared

import (
	"fmt"
	"net/http"
	"strings"
)

// ServiceToken is a Cloudflare Access service token.
//
// Secret is populated ONLY when the token was just created: Cloudflare returns
// it once, in the creation response, and never again. That single fact is why
// creating one through the API is better than through the dashboard — the value
// can go straight from the response into a keychain, without passing through a
// terminal or a clipboard.
type ServiceToken struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ClientID string `json:"client_id"`
	Secret   string `json:"client_secret"`
}

// FindServiceToken returns the token with the given name, or nil.
//
// The returned token never carries a Secret: listing does not include it.
//
// Cloudflare allows two tokens to share a name, and that is refused here rather
// than resolved by picking one. Observed on a real account: two tokens named
// alike, a stored secret belonging to the second, and the first one returned —
// a client id and a secret that do not go together, which authenticate as a 403
// long after being written into a config. Ambiguity has no correct answer, so
// the caller is told instead of guessed at.
func (e *Exposer) FindServiceToken(name string) (*ServiceToken, error) {
	var got struct {
		Result []ServiceToken `json:"result"`
	}
	if err := e.call(http.MethodGet, "/accounts/"+e.AccountID+"/access/service_tokens", nil, &got); err != nil {
		return nil, fmt.Errorf("listing service tokens: %w", err)
	}

	var matches []ServiceToken
	for _, tok := range got.Result {
		if tok.Name == name {
			matches = append(matches, tok)
		}
	}
	switch len(matches) {
	case 0:
		return nil, nil
	case 1:
		return &matches[0], nil
	default:
		ids := make([]string, 0, len(matches))
		for _, m := range matches {
			ids = append(ids, m.ClientID)
		}
		return nil, fmt.Errorf("%d service tokens are named %q (%s); "+
			"a stored secret belongs to exactly one of them and there is no way to tell which from here — "+
			"delete the ones you do not use in the dashboard",
			len(matches), name, strings.Join(ids, ", "))
	}
}

// CreateServiceToken creates a token and returns it with its secret.
//
// It refuses to overwrite: a token of the same name already existing is
// reported as such, because creating a second one with the same name would
// leave two indistinguishable credentials and no way to tell which is in use.
func (e *Exposer) CreateServiceToken(name string) (*ServiceToken, error) {
	existing, err := e.FindServiceToken(name)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, fmt.Errorf("a service token named %q already exists (client id %s); "+
			"its secret cannot be read back, so either reuse it or delete it before creating another",
			name, existing.ClientID)
	}

	var got struct {
		Result ServiceToken `json:"result"`
	}
	if err := e.call(http.MethodPost, "/accounts/"+e.AccountID+"/access/service_tokens",
		map[string]any{"name": name}, &got); err != nil {
		return nil, fmt.Errorf("creating the service token: %w", err)
	}
	if got.Result.Secret == "" {
		// Verify the effect: a token whose secret we did not receive is useless
		// and unrecoverable, so say so rather than reporting success.
		return nil, fmt.Errorf("the API created a service token but returned no client secret; "+
			"it cannot be recovered, so delete %q in the dashboard and retry", name)
	}
	return &got.Result, nil
}
