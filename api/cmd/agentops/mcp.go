package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const mcpProtocolVersion = "2024-11-05"

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type mcpResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   interface{} `json:"error,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpServer struct {
	apiURL, token string
	client        *http.Client
}

func runMCP(args []string, in io.Reader, out io.Writer, client *http.Client) error {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	apiURL := fs.String("api-url", env("AGENTOPS_API_URL", "http://127.0.0.1:8080"), "authenticated API base URL")
	token := fs.String("token", env("AGENTOPS_API_TOKEN", ""), "selected-organization JWT")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("invalid mcp flags")
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("mcp does not accept positional arguments")
	}
	if strings.TrimSpace(*apiURL) == "" {
		return fmt.Errorf("--api-url or AGENTOPS_API_URL is required")
	}
	if strings.TrimSpace(*token) == "" {
		return fmt.Errorf("--token or AGENTOPS_API_TOKEN is required")
	}
	return (&mcpServer{strings.TrimRight(*apiURL, "/"), *token, client}).serve(context.Background(), in, out)
}

func (s *mcpServer) serve(ctx context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	encoder := json.NewEncoder(out)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var req mcpRequest
		if err := json.Unmarshal(line, &req); err != nil {
			if err := encoder.Encode(mcpResponse{JSONRPC: "2.0", Error: mcpError{-32700, "invalid JSON"}}); err != nil {
				return err
			}
			continue
		}
		resp, reply := s.handle(ctx, req)
		if !reply {
			continue
		}
		if err := encoder.Encode(resp); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func (s *mcpServer) handle(ctx context.Context, req mcpRequest) (mcpResponse, bool) {
	resp := mcpResponse{JSONRPC: "2.0", ID: json.RawMessage(req.ID)}
	if req.JSONRPC != "2.0" {
		resp.Error = mcpError{-32600, "invalid request"}
		return resp, true
	}
	if strings.HasPrefix(req.Method, "notifications/") {
		return mcpResponse{}, false
	}
	switch req.Method {
	case "initialize":
		resp.Result = map[string]any{"protocolVersion": mcpProtocolVersion, "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]string{"name": "agentops", "version": "1.0.0"}}
	case "tools/list":
		resp.Result = map[string]any{"tools": []map[string]any{
			{"name": "list_issues", "description": "List issues for a repository", "inputSchema": map[string]any{"type": "object", "required": []string{"repository_id"}, "properties": map[string]any{"repository_id": map[string]string{"type": "string"}, "page": map[string]string{"type": "integer"}, "per_page": map[string]string{"type": "integer"}}}},
			{"name": "wave_status", "description": "Get an organization wave", "inputSchema": map[string]any{"type": "object", "required": []string{"wave_id"}, "properties": map[string]any{"wave_id": map[string]string{"type": "string"}}}},
		}}
	case "tools/call":
		result, err := s.callTool(ctx, req.Params)
		if err != nil {
			resp.Error = mcpError{-32001, err.Error()}
		} else {
			resp.Result = result
		}
	default:
		resp.Error = mcpError{-32601, "method not found"}
	}
	return resp, true
}

func (s *mcpServer) callTool(ctx context.Context, raw json.RawMessage) (any, error) {
	var p struct {
		Name      string                     `json:"name"`
		Arguments map[string]json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &p); err != nil || p.Name == "" {
		return nil, fmt.Errorf("invalid tools/call arguments")
	}
	var path string
	switch p.Name {
	case "list_issues":
		repo, err := stringArg(p.Arguments, "repository_id")
		if err != nil {
			return nil, err
		}
		q := url.Values{"repository_id": []string{repo}}
		for _, key := range []string{"page", "per_page"} {
			if v, ok := p.Arguments[key]; ok {
				n, err := integerArg(v, key)
				if err != nil {
					return nil, err
				}
				q.Set(key, strconv.Itoa(n))
			}
		}
		path = "/api/v1/board?" + q.Encode()
	case "wave_status":
		id, err := stringArg(p.Arguments, "wave_id")
		if err != nil {
			return nil, err
		}
		path = "/api/v1/waves/" + url.PathEscape(id)
	default:
		return nil, fmt.Errorf("unknown tool %q", p.Name)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.apiURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Accept", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, readErr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("upstream API returned HTTP %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var value json.RawMessage
	if err := json.Unmarshal(body, &value); err != nil {
		return nil, fmt.Errorf("upstream API returned invalid JSON")
	}
	return map[string]any{"content": []map[string]any{{"type": "text", "text": string(value)}}}, nil
}

func stringArg(args map[string]json.RawMessage, name string) (string, error) {
	v, ok := args[name]
	if !ok {
		return "", fmt.Errorf("missing required argument %q", name)
	}
	var s string
	if json.Unmarshal(v, &s) != nil || strings.TrimSpace(s) == "" {
		return "", fmt.Errorf("argument %q must be a non-empty string", name)
	}
	return s, nil
}
func integerArg(v json.RawMessage, name string) (int, error) {
	var n int
	if json.Unmarshal(v, &n) != nil || n < 1 {
		return 0, fmt.Errorf("argument %q must be a positive integer", name)
	}
	return n, nil
}
