package main

// appVersion is injected at build time:
//
//	go build -ldflags "-X main.appVersion=v0.19.3" ./cmd/agent
//
// Docker / release workflows must use main.appVersion (not main.Version).
var appVersion = "dev"

func agentVersion() string {
	v := appVersion
	if v == "" {
		return "dev"
	}
	return v
}
