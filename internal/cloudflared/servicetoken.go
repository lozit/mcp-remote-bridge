package cloudflared

import (
	"fmt"
	"net/http"
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
func (e *Exposer) FindServiceToken(name string) (*ServiceToken, error) {
	var got struct {
		Result []ServiceToken `json:"result"`
	}
	if err := e.call(http.MethodGet, "/accounts/"+e.AccountID+"/access/service_tokens", nil, &got); err != nil {
		return nil, fmt.Errorf("listing service tokens: %w", err)
	}
	for i := range got.Result {
		if got.Result[i].Name == name {
			return &got.Result[i], nil
		}
	}
	return nil, nil
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
