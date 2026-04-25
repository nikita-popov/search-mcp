// search-mcp — minimal MCP web search server (DuckDuckGo + Brave).
// Transport: stdio / JSON-RPC 2.0. Zero external dependencies.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

var version = "dev"

// ── config ───────────────────────────────────────────────────────────────────

var (
	braveAPIKey     = os.Getenv("BRAVE_API_KEY")
	defaultProvider = envOr("SEARCH_PROVIDER", "duckduckgo")
	defaultMax      = envInt("SEARCH_MAX_RESULTS", 5)
	httpTimeout     = time.Duration(envInt("SEARCH_TIMEOUT", 10)) * time.Second
)

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

var client = &http.Client{Timeout: httpTimeout}

// ── search result ─────────────────────────────────────────────────────────────

type result struct {
	Title   string
	URL     string
	Snippet string
}

func formatResults(rs []result) string {
	if len(rs) == 0 {
		return "No results found."
	}
	var b strings.Builder
	for i, r := range rs {
		fmt.Fprintf(&b, "%d. %s\n", i+1, r.Title)
		if r.URL != "" {
			fmt.Fprintf(&b, "   %s\n", r.URL)
		}
		if r.Snippet != "" {
			fmt.Fprintf(&b, "   %s\n", r.Snippet)
		}
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}

// ── DuckDuckGo ────────────────────────────────────────────────────────────────

func searchDDG(query string, max int) ([]result, error) {
	u := "https://api.duckduckgo.com/?" + url.Values{
		"q": {query}, "format": {"json"},
		"no_html": {"1"}, "no_redirect": {"1"},
	}.Encode()

	resp, err := client.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data struct {
		Heading     string `json:"Heading"`
		Abstract    string `json:"Abstract"`
		AbstractURL string `json:"AbstractURL"`
		RelatedTopics []struct {
			Text     string `json:"Text"`
			FirstURL string `json:"FirstURL"`
		} `json:"RelatedTopics"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	var rs []result
	if data.Abstract != "" {
		rs = append(rs, result{Title: data.Heading, URL: data.AbstractURL, Snippet: data.Abstract})
	}
	for _, t := range data.RelatedTopics {
		if len(rs) >= max {
			break
		}
		if t.Text != "" {
			title := t.Text
			if len(title) > 80 {
				title = title[:80]
			}
			rs = append(rs, result{Title: title, URL: t.FirstURL, Snippet: t.Text})
		}
	}
	if len(rs) > max {
		rs = rs[:max]
	}
	return rs, nil
}

// ── Brave Search ──────────────────────────────────────────────────────────────

func searchBrave(query string, max int) ([]result, error) {
	if braveAPIKey == "" {
		return nil, fmt.Errorf("BRAVE_API_KEY is not set")
	}
	u := "https://api.search.brave.com/res/v1/web/search?" + url.Values{
		"q": {query}, "count": {strconv.Itoa(max)},
	}.Encode()

	req, _ := http.NewRequest(http.MethodGet, u, nil)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", braveAPIKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	var rs []result
	for _, item := range data.Web.Results {
		if len(rs) >= max {
			break
		}
		rs = append(rs, result{Title: item.Title, URL: item.URL, Snippet: item.Description})
	}
	return rs, nil
}

// ── providers ─────────────────────────────────────────────────────────────────

type providerFn func(query string, max int) ([]result, error)

var providers = map[string]providerFn{
	"duckduckgo": searchDDG,
	"brave":      searchBrave,
}

// ── JSON-RPC 2.0 types ────────────────────────────────────────────────────────

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ── MCP types ─────────────────────────────────────────────────────────────────

type toolDef struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	InputSchema inputSchema `json:"inputSchema"`
}

type inputSchema struct {
	Type       string              `json:"type"`
	Properties map[string]schemaProp `json:"properties"`
	Required   []string            `json:"required"`
}

type schemaProp struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Enum        []string `json:"enum,omitempty"`
	Minimum     *int     `json:"minimum,omitempty"`
	Maximum     *int     `json:"maximum,omitempty"`
}

// ── tool list ─────────────────────────────────────────────────────────────────

func toolList() []toolDef {
	pmin, pmax := 1, 20
	providerNames := make([]string, 0, len(providers))
	for k := range providers {
		providerNames = append(providerNames, k)
	}
	return []toolDef{{
		Name: "search",
		Description: fmt.Sprintf(
			"Search the web. Providers: %s. Default: %s.",
			strings.Join(providerNames, ", "), defaultProvider,
		),
		InputSchema: inputSchema{
			Type: "object",
			Properties: map[string]schemaProp{
				"query": {
					Type:        "string",
					Description: "Search query",
				},
				"provider": {
					Type:        "string",
					Description: "Override provider (duckduckgo, brave)",
					Enum:        providerNames,
				},
				"max_results": {
					Type:        "integer",
					Description: "Max results to return (1–20)",
					Minimum:     &pmin,
					Maximum:     &pmax,
				},
			},
			Required: []string{"query"},
		},
	}}
}

// ── call tool ─────────────────────────────────────────────────────────────────

func callTool(params json.RawMessage) (any, *rpcError) {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{-32602, "invalid params: " + err.Error()}
	}
	if p.Name != "search" {
		return nil, &rpcError{-32601, "unknown tool: " + p.Name}
	}

	var args struct {
		Query      string `json:"query"`
		Provider   string `json:"provider"`
		MaxResults int    `json:"max_results"`
	}
	if err := json.Unmarshal(p.Arguments, &args); err != nil {
		return nil, &rpcError{-32602, "invalid arguments: " + err.Error()}
	}
	if args.Query == "" {
		return nil, &rpcError{-32602, "query is required"}
	}

	providerName := defaultProvider
	if args.Provider != "" {
		providerName = args.Provider
	}
	maxResults := defaultMax
	if args.MaxResults > 0 {
		maxResults = args.MaxResults
	}

	provider, ok := providers[providerName]
	if !ok {
		return nil, &rpcError{-32602, fmt.Sprintf("unknown provider %q", providerName)}
	}

	rs, err := provider(args.Query, maxResults)
	if err != nil {
		// tool error — return as content, not RPC error
		return map[string]any{
			"content": []map[string]any{{"type": "text", "text": "Error: " + err.Error()}},
			"isError": true,
		}, nil
	}
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": formatResults(rs)}},
	}, nil
}

// ── MCP dispatcher ────────────────────────────────────────────────────────────

func handle(req request) response {
	resp := response{JSONRPC: "2.0", ID: req.ID}

	switch req.Method {
	case "initialize":
		resp.Result = map[string]any{
			"protocolVersion": "2024-11-05",
			"serverInfo":      map[string]any{"name": "search-mcp", "version": version},
			"capabilities":    map[string]any{"tools": map[string]any{}},
		}

	case "notifications/initialized", "notifications/cancelled":
		// fire-and-forget — no response
		return response{}

	case "tools/list":
		resp.Result = map[string]any{"tools": toolList()}

	case "tools/call":
		result, rpcErr := callTool(req.Params)
		if rpcErr != nil {
			resp.Error = rpcErr
		} else {
			resp.Result = result
		}

	case "ping":
		resp.Result = map[string]any{}

	default:
		resp.Error = &rpcError{-32601, "method not found: " + req.Method}
	}

	return resp
}

// ── main loop ─────────────────────────────────────────────────────────────────

func main() {
	enc := json.NewEncoder(os.Stdout)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1<<20), 1<<20) // 1 MiB line buffer

	for scanner.Scan() {
		var req request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			_ = enc.Encode(response{
				JSONRPC: "2.0",
				Error:   &rpcError{-32700, "parse error: " + err.Error()},
			})
			continue
		}
		resp := handle(req)
		// skip notification responses (empty struct, no ID)
		if resp.JSONRPC == "" {
			continue
		}
		_ = enc.Encode(resp)
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		fmt.Fprintf(os.Stderr, "stdin error: %v\n", err)
		os.Exit(1)
	}
}
