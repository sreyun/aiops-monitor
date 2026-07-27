package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// configB64Re matches AIOPS_CONFIG_B64='...' / $AiopsConfigB64 = '...' in install scripts.
var configB64Re = regexp.MustCompile(`((?:AIOPS_CONFIG_B64|\$AiopsConfigB64)\s*=\s*')([A-Za-z0-9+/=]+)(')`)

// rewriteInstallScriptForRelay rewrites SERVER= / $Server and embedded CONFIG_B64
// so internal agents point at the relay instead of the cloud PublicURL.
func rewriteInstallScriptForRelay(body, relayURL string) string {
	relayURL = strings.TrimRight(strings.TrimSpace(relayURL), "/")
	if relayURL == "" {
		return body
	}
	out := serverLineRe.ReplaceAllString(body, "${1}"+relayURL+"${2}")
	out = configB64Re.ReplaceAllStringFunc(out, func(m string) string {
		sub := configB64Re.FindStringSubmatch(m)
		if len(sub) != 4 {
			return m
		}
		raw, err := base64.StdEncoding.DecodeString(sub[2])
		if err != nil {
			return m
		}
		rewritten := rewriteAgentConfigYAMLForRelay(string(raw), relayURL)
		return sub[1] + base64.StdEncoding.EncodeToString([]byte(rewritten)) + sub[3]
	})
	return out
}

// rewriteAgentConfigYAMLForRelay forces server to relayURL and drops servers[]
// (multi-server is incompatible with gateway relay mode). When dropping a
// single-line JSON servers array, the first entry's token is preserved so the
// agent can still register.
func rewriteAgentConfigYAMLForRelay(yamlText, relayURL string) string {
	relayURL = strings.TrimRight(strings.TrimSpace(relayURL), "/")
	lines := strings.Split(yamlText, "\n")
	out := make([]string, 0, len(lines)+2)
	skipServers := false
	wroteServer := false
	hasToken := false
	pendingToken := ""
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "token:") {
			hasToken = true
		}
		if skipServers {
			if trim == "" || strings.HasPrefix(trim, "#") || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") || strings.HasPrefix(trim, "-") {
				continue
			}
			skipServers = false
		}
		if strings.HasPrefix(trim, "servers:") {
			if !wroteServer {
				out = append(out, fmt.Sprintf("server: %q", relayURL))
				wroteServer = true
			}
			// Single-line JSON servers: drop entirely. Multi-line block: skip indented.
			rest := strings.TrimSpace(strings.TrimPrefix(trim, "servers:"))
			if tok := extractFirstServersToken(rest); tok != "" && pendingToken == "" {
				pendingToken = tok
			}
			if rest == "" || rest == "|" || rest == ">" {
				skipServers = true
			}
			continue
		}
		if strings.HasPrefix(trim, "server:") {
			if !wroteServer {
				out = append(out, fmt.Sprintf("server: %q", relayURL))
				wroteServer = true
			}
			continue
		}
		out = append(out, line)
	}
	if !wroteServer {
		// Insert near top after comments.
		inserted := false
		final := make([]string, 0, len(out)+1)
		for i, line := range out {
			trim := strings.TrimSpace(line)
			if !inserted && trim != "" && !strings.HasPrefix(trim, "#") {
				final = append(final, fmt.Sprintf("server: %q", relayURL))
				inserted = true
			}
			final = append(final, line)
			_ = i
		}
		if !inserted {
			final = append([]string{fmt.Sprintf("server: %q", relayURL)}, out...)
		}
		out = final
	}
	if pendingToken != "" && !hasToken {
		out = append(out, fmt.Sprintf("token: %q", pendingToken))
	}
	return strings.Join(out, "\n")
}

// extractFirstServersToken pulls token from a single-line JSON servers: value.
func extractFirstServersToken(rest string) string {
	rest = strings.TrimSpace(rest)
	if rest == "" || rest == "|" || rest == ">" {
		return ""
	}
	var entries []struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal([]byte(rest), &entries); err != nil || len(entries) == 0 {
		return ""
	}
	return strings.TrimSpace(entries[0].Token)
}
