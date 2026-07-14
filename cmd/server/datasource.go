package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// DataSource is an external observability backend (Loki logs / Prometheus metrics)
// that operators configure themselves. Supports HTTP Basic Auth. Used by AI chat,
// log search, and alert queries to pull data directly for analysis / triage.
type DataSource struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"` // "loki" | "prometheus"
	URL       string `json:"url"`
	AuthUser  string `json:"auth_user,omitempty"`
	AuthPass  string `json:"auth_pass,omitempty"` // masked when read via the API
	Enabled   bool   `json:"enabled"`
	CreatedAt int64  `json:"created_at"`
}

// ---------------------------------------------------------------------------
// ConfigStore CRUD (persisted in ServerConfig.DataSources → app_config in PG)
// ---------------------------------------------------------------------------

func (cs *ConfigStore) ListDataSources() []DataSource {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return append([]DataSource{}, cs.cfg.DataSources...)
}

func (cs *ConfigStore) GetDataSource(id string) (DataSource, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	for _, d := range cs.cfg.DataSources {
		if d.ID == id {
			return d, true
		}
	}
	return DataSource{}, false
}

func (cs *ConfigStore) AddDataSource(ds DataSource) (DataSource, error) {
	cs.mu.Lock()
	if ds.ID == "" {
		ds.ID = termID()[:8]
	}
	if ds.CreatedAt == 0 {
		ds.CreatedAt = time.Now().Unix()
	}
	cs.cfg.DataSources = append(cs.cfg.DataSources, ds)
	cs.mu.Unlock()
	return ds, cs.save()
}

func (cs *ConfigStore) UpdateDataSource(id string, updated DataSource) error {
	cs.mu.Lock()
	found := false
	for i, d := range cs.cfg.DataSources {
		if d.ID == id {
			updated.ID = d.ID
			updated.CreatedAt = d.CreatedAt
			// Keep the stored password when the browser sent a blank/masked one.
			if updated.AuthPass == "" || strings.Contains(updated.AuthPass, "****") {
				updated.AuthPass = d.AuthPass
			}
			cs.cfg.DataSources[i] = updated
			found = true
			break
		}
	}
	cs.mu.Unlock()
	if !found {
		return fmt.Errorf("data source not found")
	}
	return cs.save()
}

func (cs *ConfigStore) DeleteDataSource(id string) error {
	cs.mu.Lock()
	var kept []DataSource
	for _, d := range cs.cfg.DataSources {
		if d.ID != id {
			kept = append(kept, d)
		}
	}
	cs.cfg.DataSources = kept
	cs.mu.Unlock()
	return cs.save()
}

// ---------------------------------------------------------------------------
// query clients
// ---------------------------------------------------------------------------

// dsHTTPClient: data sources are trusted operator-configured endpoints (like the
// VM / PG URLs), commonly INTERNAL (e.g. http://loki:3100) — so they are
// deliberately NOT behind the SSRF guard, matching the monitoring/probe path.
var dsHTTPClient = &http.Client{Timeout: 20 * time.Second}

func dataSourceGet(ds DataSource, path string, q url.Values) ([]byte, int, error) {
	full := strings.TrimRight(ds.URL, "/") + path
	if len(q) > 0 {
		full += "?" + q.Encode()
	}
	req, err := http.NewRequest(http.MethodGet, full, nil)
	if err != nil {
		return nil, 0, err
	}
	if ds.AuthUser != "" || ds.AuthPass != "" {
		req.SetBasicAuth(ds.AuthUser, ds.AuthPass)
	}
	resp, err := dsHTTPClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20)) // 2 MiB cap
	return body, resp.StatusCode, nil
}

func dsTruncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// queryDataSource dispatches a query to the right backend by data-source type.
func queryDataSource(ds DataSource, query string, limit, sinceMin int) (string, error) {
	switch ds.Type {
	case "prometheus":
		return queryPrometheus(ds, query)
	case "loki":
		return queryLoki(ds, query, limit, sinceMin)
	default:
		return "", fmt.Errorf("unknown data source type: %s", ds.Type)
	}
}

// queryPrometheus runs a PromQL instant query and returns a compact text result.
func queryPrometheus(ds DataSource, promQL string) (string, error) {
	body, code, err := dataSourceGet(ds, "/api/v1/query", url.Values{"query": {promQL}})
	if err != nil {
		return "", err
	}
	if code != http.StatusOK {
		return "", fmt.Errorf("Prometheus HTTP %d: %s", code, dsTruncate(string(body), 300))
	}
	var r struct {
		Status string `json:"status"`
		Error  string `json:"error"`
		Data   struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Metric map[string]string `json:"metric"`
				Value  []any             `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &r) != nil {
		return dsTruncate(string(body), 2000), nil
	}
	if r.Status != "success" {
		return "", fmt.Errorf("Prometheus error: %s", r.Error)
	}
	if len(r.Data.Result) == 0 {
		return "（查询成功，无匹配数据）", nil
	}
	var sb strings.Builder
	for i, res := range r.Data.Result {
		if i >= 100 {
			fmt.Fprintf(&sb, "…（共 %d 条序列，已截断至 100）\n", len(r.Data.Result))
			break
		}
		val := ""
		if len(res.Value) == 2 {
			val = fmt.Sprintf("%v", res.Value[1])
		}
		sb.WriteString(promLabelStr(res.Metric))
		sb.WriteString(" = ")
		sb.WriteString(val)
		sb.WriteByte('\n')
	}
	return sb.String(), nil
}

func promLabelStr(m map[string]string) string {
	if len(m) == 0 {
		return "{}"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+m[k])
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// queryLoki runs a LogQL range query over the last sinceMin minutes and returns
// the most recent matching log lines (newest first, up to limit).
func queryLoki(ds DataSource, logQL string, limit, sinceMin int) (string, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if sinceMin <= 0 {
		sinceMin = 60
	}
	now := time.Now()
	q := url.Values{
		"query":     {logQL},
		"limit":     {strconv.Itoa(limit)},
		"start":     {strconv.FormatInt(now.Add(-time.Duration(sinceMin)*time.Minute).UnixNano(), 10)},
		"end":       {strconv.FormatInt(now.UnixNano(), 10)},
		"direction": {"backward"},
	}
	body, code, err := dataSourceGet(ds, "/loki/api/v1/query_range", q)
	if err != nil {
		return "", err
	}
	if code != http.StatusOK {
		return "", fmt.Errorf("Loki HTTP %d: %s", code, dsTruncate(string(body), 300))
	}
	var r struct {
		Status string `json:"status"`
		Data   struct {
			Result []struct {
				Stream map[string]string `json:"stream"`
				Values [][]string        `json:"values"` // [ [ts_ns, line], ... ]
			} `json:"result"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &r) != nil {
		return dsTruncate(string(body), 2000), nil
	}
	type lokiLine struct {
		ts     int64
		stream string
		text   string
	}
	var lines []lokiLine
	for _, res := range r.Data.Result {
		lbl := promLabelStr(res.Stream)
		for _, v := range res.Values {
			if len(v) != 2 {
				continue
			}
			ts, _ := strconv.ParseInt(v[0], 10, 64)
			lines = append(lines, lokiLine{ts: ts, stream: lbl, text: v[1]})
		}
	}
	if len(lines) == 0 {
		return "（查询成功，无匹配日志）", nil
	}
	sort.Slice(lines, func(i, j int) bool { return lines[i].ts > lines[j].ts })
	if len(lines) > limit {
		lines = lines[:limit]
	}
	var sb strings.Builder
	for _, l := range lines {
		sb.WriteString(time.Unix(0, l.ts).Format("2006-01-02 15:04:05"))
		sb.WriteString("  ")
		sb.WriteString(l.text)
		sb.WriteByte('\n')
	}
	return sb.String(), nil
}

// testDataSource pings the data source to validate URL + auth reachability.
func testDataSource(ds DataSource) error {
	switch ds.Type {
	case "prometheus":
		_, err := queryPrometheus(ds, "vector(1)")
		return err
	case "loki":
		body, code, err := dataSourceGet(ds, "/loki/api/v1/labels", nil)
		if err != nil {
			return err
		}
		if code != http.StatusOK {
			return fmt.Errorf("Loki HTTP %d: %s", code, dsTruncate(string(body), 200))
		}
		return nil
	default:
		return fmt.Errorf("unknown data source type: %s", ds.Type)
	}
}
