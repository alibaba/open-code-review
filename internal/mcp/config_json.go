// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package mcp

import (
	"encoding/json"

	"github.com/alibaba/open-code-review/internal/config/jsonfields"
)

func (c *MCPServerConfig) UnmarshalJSON(data []byte) error {
	type mcpServerConfigAlias MCPServerConfig
	var decoded mcpServerConfigAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	unknown, err := jsonfields.Collect(data, jsonfields.Names(MCPServerConfig{}))
	if err != nil {
		return err
	}
	*c = MCPServerConfig(decoded)
	c.unknownJSONFields = unknown
	return nil
}

func (c MCPServerConfig) MarshalJSON() ([]byte, error) {
	type mcpServerConfigAlias MCPServerConfig
	data, err := json.Marshal(mcpServerConfigAlias(c))
	if err != nil {
		return nil, err
	}
	return jsonfields.Merge(data, c.unknownJSONFields)
}

func (c *MCPConfig) UnmarshalJSON(data []byte) error {
	type mcpConfigAlias MCPConfig
	var decoded mcpConfigAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	unknown, err := jsonfields.Collect(data, jsonfields.Names(MCPConfig{}))
	if err != nil {
		return err
	}
	*c = MCPConfig(decoded)
	c.unknownJSONFields = unknown
	return nil
}

func (c MCPConfig) MarshalJSON() ([]byte, error) {
	type mcpConfigAlias MCPConfig
	data, err := json.Marshal(mcpConfigAlias(c))
	if err != nil {
		return nil, err
	}
	return jsonfields.Merge(data, c.unknownJSONFields)
}
