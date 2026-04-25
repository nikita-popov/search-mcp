#!/usr/bin/env python3
"""Minimal MCP web search server — DuckDuckGo & Brave, stdio transport."""
import asyncio
import os
import httpx
from mcp.server import Server
from mcp.server.stdio import stdio_server
from mcp.types import Tool, TextContent

# ── Config from ENV ──────────────────────────────────────────────────────────
BRAVE_API_KEY   = os.environ.get("BRAVE_API_KEY", "")
DEFAULT_PROVIDER = os.environ.get("SEARCH_PROVIDER", "duckduckgo").lower()
MAX_RESULTS     = int(os.environ.get("SEARCH_MAX_RESULTS", "5"))
TIMEOUT         = float(os.environ.get("SEARCH_TIMEOUT", "10"))

# ── Providers ────────────────────────────────────────────────────────────────
async def search_duckduckgo(query: str, max_results: int) -> list[dict]:
    """DuckDuckGo Instant Answer API — no key required."""
    url = "https://api.duckduckgo.com/"
    params = {"q": query, "format": "json", "no_html": "1", "no_redirect": "1"}
    async with httpx.AsyncClient(timeout=TIMEOUT) as client:
        r = await client.get(url, params=params)
        r.raise_for_status()
        data = r.json()

    results = []
    # Abstract (top answer)
    if data.get("Abstract"):
        results.append({
            "title": data.get("Heading", query),
            "url":   data.get("AbstractURL", ""),
            "snippet": data["Abstract"],
        })
    # Related topics
    for topic in data.get("RelatedTopics", []):
        if len(results) >= max_results:
            break
        if "Text" in topic:
            results.append({
                "title":   topic.get("Text", "")[:80],
                "url":     topic.get("FirstURL", ""),
                "snippet": topic.get("Text", ""),
            })
    return results[:max_results]


async def search_brave(query: str, max_results: int) -> list[dict]:
    """Brave Search API — requires BRAVE_API_KEY."""
    if not BRAVE_API_KEY:
        raise ValueError("BRAVE_API_KEY env variable is not set")
    url = "https://api.search.brave.com/res/v1/web/search"
    headers = {
        "Accept": "application/json",
        "Accept-Encoding": "gzip",
        "X-Subscription-Token": BRAVE_API_KEY,
    }
    params = {"q": query, "count": max_results}
    async with httpx.AsyncClient(timeout=TIMEOUT) as client:
        r = await client.get(url, headers=headers, params=params)
        r.raise_for_status()
        data = r.json()

    results = []
    for item in data.get("web", {}).get("results", [])[:max_results]:
        results.append({
            "title":   item.get("title", ""),
            "url":     item.get("url", ""),
            "snippet": item.get("description", ""),
        })
    return results


PROVIDERS = {
    "duckduckgo": search_duckduckgo,
    "brave":      search_brave,
}


def fmt_results(results: list[dict]) -> str:
    if not results:
        return "No results found."
    lines = []
    for i, r in enumerate(results, 1):
        lines.append(f"{i}. {r['title']}")
        if r["url"]:
            lines.append(f"   {r['url']}")
        if r["snippet"]:
            lines.append(f"   {r['snippet']}")
        lines.append("")
    return "\n".join(lines).strip()


# ── MCP Server ───────────────────────────────────────────────────────────────
app = Server("search-mcp")


@app.list_tools()
async def list_tools() -> list[Tool]:
    providers = ", ".join(PROVIDERS)
    return [
        Tool(
            name="search",
            description=(
                f"Search the web. Available providers: {providers}. "
                f"Default provider: {DEFAULT_PROVIDER}."
            ),
            inputSchema={
                "type": "object",
                "properties": {
                    "query": {
                        "type": "string",
                        "description": "Search query",
                    },
                    "provider": {
                        "type": "string",
                        "enum": list(PROVIDERS),
                        "description": "Search provider (overrides SEARCH_PROVIDER env)",
                    },
                    "max_results": {
                        "type": "integer",
                        "description": "Max results to return (overrides SEARCH_MAX_RESULTS env)",
                        "minimum": 1,
                        "maximum": 20,
                    },
                },
                "required": ["query"],
            },
        )
    ]


@app.call_tool()
async def call_tool(name: str, arguments: dict):
    if name != "search":
        raise ValueError(f"Unknown tool: {name}")

    query       = arguments["query"]
    provider    = arguments.get("provider", DEFAULT_PROVIDER)
    max_results = int(arguments.get("max_results", MAX_RESULTS))

    if provider not in PROVIDERS:
        raise ValueError(f"Unknown provider '{provider}'. Use: {', '.join(PROVIDERS)}")

    results = await PROVIDERS[provider](query, max_results)
    return [TextContent(type="text", text=fmt_results(results))]


async def main():
    async with stdio_server() as (read_stream, write_stream):
        await app.run(read_stream, write_stream, app.create_initialization_options())


if __name__ == "__main__":
    asyncio.run(main())
