// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"errors"
	"strings"
)

// EndpointInspection exposes only metadata, never the credential-bearing endpoint.
type EndpointInspection struct {
	Provider         string `json:"provider"`
	Model            string `json:"model"`
	Protocol         string `json:"protocol"`
	Source           string `json:"source"`
	TimeoutSec       int64  `json:"timeout_sec"`
	TimeoutSource    string `json:"timeout_source"`
	CredentialStatus string `json:"credential_status"`
}

// InspectEndpoint follows endpoint precedence without acquiring credentials.
// Dynamic credentials and model catalogs remain unresolved. URLs, headers,
// request bodies and credential commands are deliberately outside this schema.
func InspectEndpoint(configPath string) (EndpointInspection, error) {
	ep, err := ResolveEndpointWithOptions(configPath, ResolveOptions{inspectOnly: true})
	if err != nil {
		// Resolver diagnostics may quote credential-bearing URLs or header values.
		return EndpointInspection{}, errors.New("cannot inspect LLM configuration: invalid or incomplete configuration")
	}
	status := ep.credentialStatus
	if status == "" {
		status = credentialInspectionStatus(ep.Token, "", ep.AmbientAuth)
	}
	source := ep.Source
	if source == "" {
		source = "none"
	}
	timeoutSource := source
	if ep.Timeout == 0 {
		timeoutSource = "client_default"
	}
	if _, set, _ := parseTimeoutEnv(); set {
		timeoutSource = envOCRLLMTimeout
	}
	return EndpointInspection{
		Provider: ep.Provider, Model: ep.Model, Protocol: ep.Protocol, Source: source,
		TimeoutSec: int64(ep.Timeout.Seconds()), TimeoutSource: timeoutSource,
		CredentialStatus: status,
	}, nil
}

func credentialInspectionStatus(token, command string, ambient bool) string {
	switch {
	case ambient:
		return "ambient_not_resolved"
	case strings.TrimSpace(token) != "":
		return "configured"
	case strings.TrimSpace(command) != "":
		return "command_not_executed"
	default:
		return "not_configured"
	}
}
