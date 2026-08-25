# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **HTTP transport security hardening** (Document 004)
  - New CLI flags: `--mcp-token`, `--mcp-token-file`
  - TLS support: `--tls`, `--tls-cert`, `--tls-key`
  - Explicit all-interfaces binding: `--allow-all-interfaces`
  - Strict security model: token always required for HTTP transport
  - Non-localhost requires token + TLS
  - Binding to 0.0.0.0/:: requires explicit flag + token + TLS
  - Constant-time token comparison to prevent timing attacks
- **Streamable HTTP transport** (`--transport streamable-http`)
  - Modern MCP protocol support (version 2024-11-05)
  - Single `/mcp` endpoint for all operations
  - Automatic SSE upgrade for streaming responses
  - Bidirectional communication support
  - Session management with Last-Event-ID support
  - Backward compatibility with legacy SSE endpoint
  - Better alignment with Python MCP implementations
- **Operation type filtering** with `--enable` and `--disable` flags
  - Fine-grained control over which operation types are generated
  - Support for operation types: C (create), S (search), F (filter), G (get), U (update), D (delete), A (actions/function imports)
  - Special R (read) type that expands to S, F, and G operations
  - Case-insensitive operation codes
  - Helps reduce tool count for services with many entities (e.g., from 300+ to manageable numbers)
  - Examples:
    - `--disable "cud"` - Disable create, update, delete operations
    - `--enable "r"` - Enable only read operations (search, filter, get)
    - `--disable "a"` - Disable actions/function imports
- **Read-only mode flags** (`--read-only`/`-ro` and `--read-only-but-functions`/`-robf`)
  - Hide all modifying operations (create, update, delete) in read-only mode
  - Allow function imports in read-only-but-functions mode
- **MCP trace logging** (`--trace-mcp`) for debugging protocol communication
  - Captures all incoming/outgoing MCP messages
  - Saves detailed trace logs to `/tmp/mcp_trace_*.log` (Linux/WSL) or `%TEMP%\mcp_trace_*.log` (Windows)
  - Helps diagnose client compatibility issues
- **Flexible hint system** for service-specific guidance
  - JSON-based hint configuration with wildcard pattern matching
  - `--hints-file` flag to load custom hint files
  - `--hint` flag for direct CLI hint injection
  - Priority-based hint merging for multiple matching patterns
  - Default hints for SAP OData services including HTTP 501 workarounds
  - Hints appear in `odata_service_info` tool response
- **Full MCP protocol compliance**
  - Added missing `resources/list` and `prompts/list` handlers
  - Proper capability declarations in initialize response
  - Strict JSON-RPC 2.0 validation
- **Enhanced error handling**
  - Better null ID handling for Claude Desktop compatibility
  - Proper JSON-RPC error responses
  - Detailed error categorization
- **HTTP/SSE transport support** (in addition to stdio)
  - Support for Server-Sent Events transport with `--transport http`
  - Configurable HTTP server address with `--http-addr`
- **Legacy date format support** for SAP compatibility
  - Automatic conversion of SAP date formats
  - `--no-legacy-dates` flag to disable conversion
- **Enhanced response features**
  - Response size limits with `--max-response-size`
  - Item count limits with `--max-items`
  - Pagination hints with `--pagination-hints`
  - Response metadata inclusion with `--response-metadata`
  - Date conversion options with `--convert-dates-from-sap`
- OData v4 support with automatic version detection
- Query parameter translation ($inlinecount to $count for v4)
- Automatic versioning based on git tags and commit history
- GitHub Actions workflows for automated releases
- WSL-specific build targets
- Comprehensive test suite for v4 functionality

### Changed
- **Improved MCP protocol implementation**
  - Initialize response now includes all required capabilities (tools, resources, prompts)
  - Better compatibility with different MCP clients (Claude Desktop, RooCode, GitHub Copilot)
  - Stricter validation to prevent client-side errors
- **ID handling improvements**
  - Null IDs are converted to 0 for better client compatibility
  - Proper handling of different ID types (string, number, null)
- Improved response parsing for both v2 and v4 formats
- Enhanced error handling with detailed OData error messages
- Makefile now uses dynamic versioning instead of hardcoded version

### Fixed
- **`service_info` was unreachable in universal mode**
  - The handler existed but was never wired into the action switch, and the
    action was missing from the schema enum
  - It also tripped over the mandatory `target`, which it does not need;
    `target` is now validated per action and the error names the action
- **Search against OData v2 services always failed**
  - `$search` is v4 syntax; SAP Gateway v2 rejects it with HTTP 400
    "Ungueltige Systemabfrageoption angegeben"
  - v2 services now get plain `search`, v4 keeps `$search`
- **Error responses were built as invalid JSON and silently dropped**
  - The `data` field was quoted by hand (`fmt.Sprintf("\"%s\"", data)`), so any
    error text containing a quote -- a URL, a parameter name -- produced a broken
    `json.RawMessage`
  - Encoding then failed *before* writing anything: the client got an empty
    `200` with `Content-Length: 0` and waited out its own timeout
  - Combined with the SSE bug below this is what turned a plain 30s OData
    timeout into a 180s hang ending in `context canceled`
  - The data field is now marshalled, and both response paths fall back to a
    valid error instead of writing nothing
- **Every `tools/call` over streamable HTTP hung until the client gave up**
  - The response was sent as an SSE event, then the handler blocked in a
    keep-alive loop that only ended when the client cancelled -- nothing ever
    pushes further events into a POST stream
  - Clients waited out their own timeout and reported a bare cancellation
    (e.g. 180s, then `context canceled`), even for calls that never touch OData
  - The stream is now closed once the response is written
- **A panicking tool handler killed the process**
  - No `recover()` anywhere, so a panic left clients with no response at all
  - Panics are now returned as a JSON-RPC error and logged to stderr
- **Doubled timeout on modifying operations**
  - CSRF token fetch and the actual request each got the full HTTP timeout, so a
    hanging service made the caller wait 2x the timeout (60s with the 30s default)
  - Both now share one deadline per operation, so an error surfaces after 1x the timeout
  - New `--timeout <seconds>` flag (default 30) to tune it per service
- **SAP OData multi-schema metadata parsing** (Issue #12, Document 005)
  - Parser now handles multiple Schema elements in EDMX metadata
  - EntityTypes stored with qualified names (Namespace.TypeName)
  - Fallback lookup for short names (backward compatibility)
  - SAP namespace attributes (`sap:creatable`, `sap:deletable`, etc.) now parsed correctly
  - Function imports with `m:HttpMethod` attribute parsed correctly
- **Claude Desktop Zod validation errors**
  - Missing capability declarations that caused validation failures
  - Null ID handling that triggered type errors
  - Missing method handlers for resources and prompts
- **MCP client compatibility issues**
  - Fixed issues preventing tools from appearing in RooCode
  - Resolved connection problems with various MCP clients
  - Better error response formatting
- Multiple main function declarations in test files
- Type assertion panics in response parser
- Count value parsing for v2 string responses

## [0.1.0] - 2024-06-30

### Added
- Initial Go implementation of OData MCP Bridge
- Support for OData v2 services
- Dynamic tool generation based on metadata
- Basic auth and cookie authentication
- SAP OData extensions with CSRF token support
- Comprehensive CRUD operations
- Advanced query support with OData query options
- Function import support
- Cross-platform builds for Linux, Windows, and macOS

### Notes
- This is a Go port of the Python OData-MCP bridge
- Maintains CLI compatibility with the original implementation

[Unreleased]: https://github.com/odata-mcp/go/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/odata-mcp/go/releases/tag/v0.1.0