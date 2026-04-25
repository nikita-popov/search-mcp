package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ── config ────────────────────────────────────────────────────────────────────

var (
	braveAPIKey      = os.Getenv("BRAVE_API_KEY")
	defaultProvider  = envOr("SEARCH_PROVIDER", "duckduckgo")
	defaultMaxResult = envInt("SEARCH_MAX_RESULTS", 5)
	timeoutSec       = envInt("SEARCH_TIMEOUT", 10)
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

// ── HTTP client ───────────────────────────────────────────────────────────────

var httpClient = &http.Client{
	Timeout: time.Duration(timeoutSec) * time.Second,
}

// ── result type ───────────────────────────────────────────────────────────────

type Result struct {
	Title   string
	URL     string
	Snippet string
}

func formatResults(results []Result) string {
	if len(results) == 0 {
		return "No results found."
	}
	var sb strings.Builder
	for i, r := range results {
		fmt.Fprintf(&sb, "%d. %s\n", i+1, r.Title)
		if r.URL != "" {
			fmt.Fprintf(&sb, "   %s\n", r.URL)
		}
		if r.Snippet != "" {
			fmt.Fprintf(&sb, "   %s\n", r.Snippet)
		}
		sb.WriteByte('\n')
	}
	return strings.TrimSpace(sb.String())
}

// ── DuckDuckGo ────────────────────────────────────────────────────────────────

func searchDuckDuckGo(query string, max int) ([]Result, error) {
	params := url.Values{
		"q":           {query},
		"format":      {"json"},
		"no_html":     {"1"},
		"no_redirect": {"1"},
	}
	req, _ := http.NewRequest(http.MethodGet,
		"https://api.duckduckgo.com/?" + params.Encode(), nil)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var data struct {
		Heading       string `json:"Heading"`
		Abstract      string `json:"Abstract"`
		AbstractURL   string `json:"AbstractURL"`
		RelatedTopics []struct {
			Text     string `json:"Text"`
			FirstURL string `json:"FirstURL"`
		} `json:"RelatedTopics"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	var results []Result
	if data.Abstract != "" {
		results = append(results, Result{
			Title:   data.Heading,
			URL:     data.AbstractURL,
			Snippet: data.Abstract,
		})
	}
	for _, t := range data.RelatedTopics {
		if len(results) >= max {
			break
		}
		if t.Text != "" {
			title := t.Text
			if len(title) > 80 {
				title = title[:80]
			}
			results = append(results, Result{
				Title:   title,
				URL:     t.FirstURL,
				Snippet: t.Text,
			})
		}
	}
	return results[:min(len(results), max)], nil
}

// ── Brave Search ──────────────────────────────────────────────────────────────

func searchBrave(query string, max int) ([]Result, error) {
	if braveAPIKey == "" {
		return nil, fmt.Errorf("BRAVE_API_KEY env variable is not set")
	}
	params := url.Values{
		"q":     {query},
		"count": {strconv.Itoa(max)},
	}
	req, _ := http.NewRequest(http.MethodGet,
		"https://api.search.brave.com/res/v1/web/search?"+params.Encode(), nil)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("X-Subscription-Token", braveAPIKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var data struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	var results []Result
	for _, item := range data.Web.Results {
		if len(results) >= max {
			break
		}
		results = append(results, Result{
			Title:   item.Title,
			URL:     item.URL,
			Snippet: item.Description,
		})
	}
	return results, nil
}

// ── providers registry ────────────────────────────────────────────────────────

type providerFunc func(query string, max int) ([]Result, error)

var providers = map[string]providerFunc{
	"duckduckgo": searchDuckDuckGo,
	"brave":      searchBrave,
}

// ── MCP tool handler ──────────────────────────────────────────────────────────

func handleSearch(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := req.RequireString("query")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	providerName := defaultProvider
	if p, ok := req.GetString("provider"); ok && p != "" {
		providerName = p
	}

	maxResults := defaultMaxResult
	if n, ok := req.GetInt("max_results"); ok && n > 0 {
		maxResults = n
	}

	provider, ok := providers[providerName]
	if !ok {
		return mcp.NewToolResultError(
			fmt.Sprintf("unknown provider %q, use: duckduckgo, brave", providerName),
		), nil
	}

	results, err := provider(query, maxResults)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(formatResults(results)), nil
}

// ── main ──────────────────────────────────────────────────────────────────────

func main() {
	s := server.NewMCPServer(
		"search-mcp",
		"0.1.0",
		server.WithToolCapabilities(false),
	)

	providerNames := make([]string, 0, len(providers))
	for k := range providers {
		providerNames = append(providerNames, k)
	}

	s.AddTool(
		mcp.NewTool("search",
			mcp.WithDescription(fmt.Sprintf(
				"Search the web. Providers: %s. Default: %s.",
				strings.Join(providerNames, ", "), defaultProvider,
			)),
			mcp.WithString("query",
				mcp.Required(),
				mcp.Description("Search query"),
			),
			mcp.WithString("provider",
				mcp.Description("Override provider: duckduckgo or brave"),
				mcp.Enum("duckduckgo", "brave"),
			),
			mcp.WithNumber("max_results",
				mcp.Description("Max results (1-20), overrides SEARCH_MAX_RESULTS"),
			),
		),
		handleSearch,
	)

	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
