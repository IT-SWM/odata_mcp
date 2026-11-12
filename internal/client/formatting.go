// Copyright (c) 2024 OData MCP Contributors
// SPDX-License-Identifier: MIT

package client

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/zmcp/odata-mcp/internal/models"
)

// encodeQueryParams encodes URL query parameters with proper space encoding
// OData servers expect spaces to be encoded as %20, not + (RFC 3986)
func encodeQueryParams(params url.Values) string {
	encoded := params.Encode()
	// Replace '+' with '%20' for OData compatibility
	return strings.ReplaceAll(encoded, "+", "%20")
}

// buildKeyPredicate builds OData key predicate from key-value pairs
func (c *ODataClient) buildKeyPredicate(key map[string]interface{}) string {
	if len(key) == 1 {
		// Single key
		for _, value := range key {
			return c.formatKeyValue(value)
		}
	}

	// Composite key
	var parts []string
	for k, v := range key {
		parts = append(parts, fmt.Sprintf("%s=%s", k, c.formatKeyValue(v)))
	}
	return strings.Join(parts, ",")
}

// formatKeyValue formats a key value for OData URL
func (c *ODataClient) formatKeyValue(value interface{}) string {
	switch v := value.(type) {
	case models.GUIDValue:
		// SAP OData requires GUID values to be prefixed: guid'value'
		return fmt.Sprintf("guid'%s'", string(v))
	case string:
		return quoteKeyString(v)
	case int, int32, int64:
		return fmt.Sprintf("%d", v)
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32)
	case float64:
		// JSON numbers arrive as float64; %g would turn 1234567 into 1.234567e+06
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return fmt.Sprintf("%t", v)
	default:
		return quoteKeyString(fmt.Sprintf("%v", v))
	}
}

// quoteKeyString builds an OData string literal for a URL path segment: a
// quote inside the value is doubled, and '/', '?', '#', '%' must not leak
// into the path, so the value is path-escaped.
func quoteKeyString(s string) string {
	return "'" + url.PathEscape(strings.ReplaceAll(s, "'", "''")) + "'"
}

// formatFunctionParameter formats a function parameter for OData URL
func (c *ODataClient) formatFunctionParameter(key string, value interface{}) string {
	switch v := value.(type) {
	case string:
		// OData requires string parameters to be single-quoted, with a quote
		// inside the value doubled. URL encode the value but not the quotes.
		return fmt.Sprintf("%s='%s'", key, url.QueryEscape(strings.ReplaceAll(v, "'", "''")))
	case int, int32, int64:
		return fmt.Sprintf("%s=%d", key, v)
	case float32:
		return key + "=" + strconv.FormatFloat(float64(v), 'f', -1, 32)
	case float64:
		return key + "=" + strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return fmt.Sprintf("%s=%t", key, v)
	default:
		// Default to string representation with quotes
		return fmt.Sprintf("%s='%s'", key, url.QueryEscape(strings.ReplaceAll(fmt.Sprintf("%v", v), "'", "''")))
	}
}
