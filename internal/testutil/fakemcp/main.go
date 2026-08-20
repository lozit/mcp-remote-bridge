// Command fakemcp is a minimal stdio MCP server used as a test fixture.
//
// It speaks just enough of the protocol for mcp-proxy to complete its startup
// handshake: initialize (declaring tools), and tools/list. Notifications are
// never answered.
//
// It also echoes selected environment variables back through tools/list, so a
// test can assert that a value reached the MCP process — which is how the
// launcher's end-to-end test proves a secret arrived through the environment
// rather than through argv.
package main

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
)

// echoEnvVars names the variables reported as tool names, one per variable
// that is set. The names are fixture-specific, never anything real.
var echoEnvVars = []string{"E2E_TOKEN", "SN_SERVER"}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 1<<20)
	out := json.NewEncoder(os.Stdout)

	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var msg struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		if len(msg.ID) == 0 { // a notification is never answered
			continue
		}

		var result any
		switch msg.Method {
		case "initialize":
			result = map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "fakemcp", "version": "0.0.1"},
			}
		case "tools/list":
			tools := []any{}
			for _, name := range echoEnvVars {
				if v, ok := os.LookupEnv(name); ok {
					tools = append(tools, map[string]any{
						"name":        "env:" + name,
						"description": v,
						"inputSchema": map[string]any{"type": "object"},
					})
				}
			}
			result = map[string]any{"tools": tools}
		case "ping":
			result = map[string]any{}
		default:
			_ = out.Encode(map[string]any{"jsonrpc": "2.0", "id": msg.ID,
				"error": map[string]any{"code": -32601, "message": "method not found"}})
			continue
		}
		_ = out.Encode(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": result})
	}
}
