# search-mcp

Minimal MCP server for web search. Works over **stdio**, configured via ENV variables.  
Providers: **DuckDuckGo** (no key required) and **Brave Search** (API key required).

## Install

```bash
pip install -e .
```

Or without cloning:

```bash
pip install git+https://github.com/nikita-popov/search-mcp.git
```

## Run

```bash
search-mcp
# or
python -m search_mcp
```

## ENV variables

| Variable              | Default      | Description                              |
|-----------------------|--------------|------------------------------------------|
| `SEARCH_PROVIDER`     | `duckduckgo` | Default provider: `duckduckgo` or `brave`|
| `BRAVE_API_KEY`       | —            | Required only when using Brave provider  |
| `SEARCH_MAX_RESULTS`  | `5`          | Default number of results to return      |
| `SEARCH_TIMEOUT`      | `10`         | HTTP request timeout in seconds          |

## MCP client config

### DuckDuckGo (no key)

```json
{
  "mcpServers": {
    "search": {
      "command": "search-mcp"
    }
  }
}
```

### Brave Search

```json
{
  "mcpServers": {
    "search": {
      "command": "search-mcp",
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

Arguments:

| Name          | Type    | Required | Description                                         |
|---------------|---------|----------|-----------------------------------------------------|
| `query`       | string  | ✅       | Search query                                        |
| `provider`    | string  | —        | Override provider for this call (`duckduckgo`, `brave`) |
| `max_results` | integer | —        | Override max results for this call (1–20)           |
