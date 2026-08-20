package bridge

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// MCPProbeTimeout bounds the whole mcp_responds sequence.
const MCPProbeTimeout = 10 * time.Second

// mcpProtocolVersion is the version the probe negotiates. It is pinned rather
// than "latest": the probe must keep answering the same question across MCP
// releases, and a silently shifting handshake is a moving oracle.
const mcpProtocolVersion = "2024-11-05"

// jsonRPCResponse is the shape the probe reads a verdict from.
//
// Error is a pointer on purpose. A dead MCP behind mcp-proxy answers
// {"error":{"code":0,"message":""}} — an error object whose every field is a
// zero value. A non-pointer struct could not tell that apart from "no error",
// and the probe would report a dead MCP as healthy.
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *jsonRPCError   `json:"error"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// mcpCapabilities is the subset of the initialize result the probe needs: which
// list method is legitimate to call.
type mcpCapabilities struct {
	Tools     *struct{} `json:"tools"`
	Resources *struct{} `json:"resources"`
	Prompts   *struct{} `json:"prompts"`
}

type initializeResult struct {
	ProtocolVersion string          `json:"protocolVersion"`
	Capabilities    mcpCapabilities `json:"capabilities"`
	ServerInfo      struct {
		Name string `json:"name"`
	} `json:"serverInfo"`
}

// ProbeMCPResponds reports whether the MCP process behind endpoint actually
// answered — not whether something is listening, and not whether a handshake
// completed.
//
// endpoint is the full streamable-HTTP URL, e.g.
// "http://127.0.0.1:8080/mcp" locally or "https://sn-mcp.example.com/mcp"
// through the tunnel.
//
// The sequence, and why it is not shorter (see ADR 0003):
//
//  1. initialize — establishes the session and yields the mcp-session-id
//     header and the server's capabilities. Its success proves NOTHING about
//     the MCP: mcp-proxy answers it from the state negotiated at startup, and
//     keeps answering it after the MCP process is dead. Same for `ping`.
//  2. notifications/initialized — required by the protocol before any call.
//  3. a list call chosen from the declared capabilities — the only step that
//     must traverse to the MCP process itself.
//
// The verdict is read from the JSON-RPC body, never the HTTP status: a dead MCP
// still returns HTTP 200.
//
// decorate, if non-nil, is called on every request before it is sent — this is
// where Cloudflare Access credentials are attached. Called with no decoration,
// a success proves the endpoint answers *without* credentials, which is what
// the access-policy check in ADR 0001 relies on.
func ProbeMCPResponds(ctx context.Context, client *http.Client, endpoint string, decorate func(*http.Request)) Check {
	check := Check{Name: CheckMCPResponds, Detail: endpoint}

	fail := func(format string, args ...any) Check {
		check.Err = fmt.Errorf(format, args...)
		return check
	}

	// 1. initialize
	initBody := map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{
			"protocolVersion": mcpProtocolVersion,
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "mcp-remote-bridge-probe", "version": "0"},
		},
	}
	resp, hdr, err := mcpCall(ctx, client, endpoint, "", initBody, decorate)
	if err != nil {
		return fail("initialize: %w", err)
	}
	if resp.Error != nil {
		return fail("initialize rejected: %s", describeRPCError(resp.Error))
	}
	var initRes initializeResult
	if err := json.Unmarshal(resp.Result, &initRes); err != nil {
		return fail("initialize returned an unreadable result: %w", err)
	}
	session := hdr.Get("mcp-session-id")

	listMethod, err := listMethodFor(initRes.Capabilities)
	if err != nil {
		// Not a liveness failure: we cannot form a question this MCP can answer.
		// Say so rather than reporting it dead.
		return fail("cannot probe %q: %w", initRes.ServerInfo.Name, err)
	}

	// 2. notifications/initialized (a notification: no id, no response body)
	notify := map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}
	if _, _, err := mcpCall(ctx, client, endpoint, session, notify, decorate); err != nil {
		return fail("notifications/initialized: %w", err)
	}

	// 3. the call that must reach the MCP process
	listBody := map[string]any{"jsonrpc": "2.0", "id": 2, "method": listMethod, "params": map[string]any{}}
	resp, _, err = mcpCall(ctx, client, endpoint, session, listBody, decorate)
	if err != nil {
		return fail("%s: %w", listMethod, err)
	}
	if resp.Error != nil {
		// The observed signature of a dead MCP is an empty error object, so this
		// message says what happened rather than relaying a blank string.
		return fail("the MCP did not answer %s (%s) — the proxy is up but the MCP behind it is not responding",
			listMethod, describeRPCError(resp.Error))
	}
	if len(resp.Result) == 0 {
		return fail("%s returned neither a result nor an error", listMethod)
	}

	check.Detail = fmt.Sprintf("%s (%s via %s)", endpoint, initRes.ServerInfo.Name, listMethod)
	check.OK = true
	return check
}

// listMethodFor picks the list call the server declared it can answer.
//
// Calling tools/list on a server that declares no tools gets -32601 from
// mcp-proxy without ever reaching the MCP — which would read as "dead" for a
// perfectly healthy resources-only server.
func listMethodFor(c mcpCapabilities) (string, error) {
	switch {
	case c.Tools != nil:
		return "tools/list", nil
	case c.Resources != nil:
		return "resources/list", nil
	case c.Prompts != nil:
		return "prompts/list", nil
	}
	return "", fmt.Errorf("the server declares no tools, resources or prompts, so no call can be made that reaches it")
}

func describeRPCError(e *jsonRPCError) string {
	if e.Message == "" {
		return fmt.Sprintf("empty JSON-RPC error, code %d", e.Code)
	}
	return fmt.Sprintf("%s (code %d)", e.Message, e.Code)
}

// mcpCall sends one JSON-RPC message and decodes the response.
//
// It accepts both content types mcp-proxy may answer with: application/json,
// and text/event-stream carrying the payload in a data: line.
func mcpCall(ctx context.Context, client *http.Client, endpoint, session string, body map[string]any, decorate func(*http.Request)) (jsonRPCResponse, http.Header, error) {
	var out jsonRPCResponse

	raw, err := json.Marshal(body)
	if err != nil {
		return out, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return out, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if session != "" {
		req.Header.Set("mcp-session-id", session)
	}
	if decorate != nil {
		decorate(req)
	}

	resp, err := client.Do(req)
	if err != nil {
		return out, nil, err
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return out, resp.Header, err
	}
	// A notification is answered with 202 and no body.
	if len(bytes.TrimSpace(payload)) == 0 {
		return out, resp.Header, nil
	}
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		payload = firstSSEData(payload)
	}
	if err := json.Unmarshal(payload, &out); err != nil {
		return out, resp.Header, fmt.Errorf("HTTP %d, unreadable body: %w", resp.StatusCode, err)
	}
	return out, resp.Header, nil
}

// firstSSEData extracts the first `data:` payload of an SSE stream.
func firstSSEData(b []byte) []byte {
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		if after, ok := strings.CutPrefix(sc.Text(), "data:"); ok {
			return []byte(strings.TrimSpace(after))
		}
	}
	return b
}
