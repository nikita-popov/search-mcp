# search-mcp

Minimal MCP web search server written in Go. Works over **stdio**, configured via ENV variables.  
Providers: **DuckDuckGo** (no key required) and **Brave Search** (API key required).

## Build

```bash
make
```

Or install directly:

```bash
go install github.com/nikita-popov/search-mcp@latest
```

## Run

```bash
search-mcp
```

## ENV variables

| Variable             | Default      | Description                               |
|----------------------|--------------|-------------------------------------------|
| `SEARCH_LOG_LEVEL`   |               | Log to `stderr`                               |
| `SEARCH_PROVIDER`    | `duckduckgo` | Default provider: `duckduckgo` or `brave` |
| `BRAVE_API_KEY`      | —            | Required only when using Brave provider   |
| `SEARCH_MAX_RESULTS` | `5`          | Default number of results to return       |
| `SEARCH_TIMEOUT`     | `10`         | HTTP request timeout in seconds           |

## MCP client config

### DuckDuckGo (no key)

```json
{
  "mcpServers": {
    "search": {
      "command": "/usr/local/bin/search-mcp"
    }
  }
}
```

### Brave Search

```json
{
  "mcpServers": {
    "search": {
      "command": "/usr/local/bin/search-mcp",
      "env": {
        "SEARCH_PROVIDER": "brave",
        "BRAVE_API_KEY": "YOUR_KEY_HERE"
      }
    }
  }
}
```

## Tool

**`search`** — search the web.

| Argument      | Type   | Required | Description                                      |
|---------------|--------|----------|--------------------------------------------------|
| `query`       | string | ✅       | Search query                                     |
| `provider`    | string | —        | Override provider for this call                  |
| `max_results` | number | —        | Override max results for this call (1–20)        |

## Adding a provider

1. Write `func searchFoo(query string, max int) ([]Result, error)`
2. Register it: `providers["foo"] = searchFoo`

Done.
