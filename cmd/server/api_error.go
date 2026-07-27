package main

import (
	"net/http"
)

// writeAPIError writes a structured JSON error with optional machine code and
// request correlation id. Prefer this over ad-hoc map[string]string{"error":…}
// on auth / diagnose / AI paths.
func writeAPIError(w http.ResponseWriter, r *http.Request, status int, code, msg string) {
	body := map[string]string{"error": msg}
	if code != "" {
		body["code"] = code
	}
	if rid := requestIDFrom(r); rid != "" {
		body["request_id"] = rid
	}
	writeJSON(w, status, body)
}
