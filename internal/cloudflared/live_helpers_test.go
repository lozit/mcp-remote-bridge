package cloudflared_test

import (
	"net/http"
	"time"
)

func newAuthedRequest(url, token string) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	// Header only: a token in a query string reaches server logs and proxies.
	req.Header.Set("Authorization", "Bearer "+token)
	return req, nil
}

func httpClient() *http.Client { return &http.Client{Timeout: 30 * time.Second} }
