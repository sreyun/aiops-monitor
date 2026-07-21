package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExtractJSONObject(t *testing.T) {
	cases := []struct{ in, want string }{
		{"前言\n```json\n{\"a\":1}\n```\n后语", `{"a":1}`},
		{"直接 {\"b\":2} 结束", `{"b":2}`},
		{"```\n{\"c\":3}\n```", `{"c":3}`},
		{"没有任何 JSON", ""},
	}
	for _, c := range cases {
		if got := extractJSONObject(c.in); got != c.want {
			t.Fatalf("extractJSONObject(%q)=%q，应为 %q", c.in, got, c.want)
		}
	}
}

func TestSanitizeAIDash(t *testing.T) {
	raw := `{
      "name": "t",
      "vars": [{"name":"instance","type":"weird"}],
      "panels": [
        {"title":"A","type":"timeseries","w":12,"h":8,"targets":[{"expr":"up"}]},
        {"title":"B","type":"nodegraph","w":12,"h":8,"targets":[{"expr":"rate(x[5m])","legend":"{{job}}"}]},
        {"title":"C","type":"stat","w":6,"h":4,"targets":[{"expr":"  "}]},
        {"title":"D","type":"text","w":24,"h":3,"text":"hi"},
        {"title":"E","type":"timeseries","w":18,"h":8,"targets":[{"expr":"y"}]}
      ]
    }`
	var spec aiDashSpec
	if err := json.Unmarshal([]byte(raw), &spec); err != nil {
		t.Fatal(err)
	}
	d, warns := sanitizeAIDash(spec, "", "ai")
	if len(d.Panels) != 4 {
		t.Fatalf("应保留 4 个面板（C 无有效查询被跳过），实为 %d", len(d.Panels))
	}
	if len(warns) != 1 {
		t.Fatalf("应有 1 条警告（C 被跳过），实为 %d", len(warns))
	}
	if d.Vars[0].Type != "query" {
		t.Fatalf("未知变量类型应回退 query，实为 %q", d.Vars[0].Type)
	}
	by := map[string]DashPanel{}
	for _, p := range d.Panels {
		by[p.Title] = p
	}
	if by["B"].Type != "timeseries" {
		t.Fatalf("未知类型(heatmap)应回退 timeseries，实为 %q", by["B"].Type)
	}
	if by["D"].Type != "text" {
		t.Fatalf("text 面板应保留，实为 %q", by["D"].Type)
	}
	// 布局：A(0,0) B(12,0) 同行；D 超宽换到 y=8；E 再换行
	if by["A"].Grid.X != 0 || by["A"].Grid.Y != 0 {
		t.Fatalf("A 应在 (0,0)，实为 (%d,%d)", by["A"].Grid.X, by["A"].Grid.Y)
	}
	if by["B"].Grid.X != 12 || by["B"].Grid.Y != 0 {
		t.Fatalf("B 应在 (12,0)，实为 (%d,%d)", by["B"].Grid.X, by["B"].Grid.Y)
	}
	if by["D"].Grid.Y != 8 {
		t.Fatalf("D 应换行到 y=8，实为 y=%d", by["D"].Grid.Y)
	}
}

// TestDecodeAIDashSpecGrafanaAliases locks in the "应用优化后看板为空" fix: LLMs often
// emit Grafana-native JSON (outer {"dashboard":{...}}, title instead of name, target
// query/legendFormat instead of expr/legend, gridPos instead of w/h). These must still
// produce populated panels rather than an empty dashboard.
func TestDecodeAIDashSpecGrafanaAliases(t *testing.T) {
	raw := "```json\n" + `{
      "dashboard": {
        "title": "Grafana 风格看板",
        "panels": [
          {"title":"CPU","type":"timeseries","gridPos":{"w":12,"h":8},
           "targets":[{"query":"rate(cpu[5m])","legendFormat":"{{instance}}"}]},
          {"title":"Mem","type":"stat","w":6,"h":4,
           "targets":[{"expr":"mem_used"}]}
        ]
      }
    }` + "\n```"
	spec, ok := decodeAIDashSpec(raw)
	if !ok {
		t.Fatal("decodeAIDashSpec 应能解析 Grafana 包裹格式")
	}
	if spec.specName() != "Grafana 风格看板" {
		t.Fatalf("title 别名应作为看板名，实为 %q", spec.specName())
	}
	d, _ := sanitizeAIDash(spec, "", "ai")
	if len(d.Panels) != 2 {
		t.Fatalf("应解析出 2 个面板（不再为空），实为 %d", len(d.Panels))
	}
	cpu := d.Panels[0]
	if len(cpu.Targets) != 1 || cpu.Targets[0].Expr != "rate(cpu[5m])" {
		t.Fatalf("query 别名应映射为 expr，实为 %+v", cpu.Targets)
	}
	if cpu.Targets[0].Legend != "{{instance}}" {
		t.Fatalf("legendFormat 别名应映射为 legend，实为 %q", cpu.Targets[0].Legend)
	}
	if cpu.Grid.W != 12 {
		t.Fatalf("gridPos.w 应映射为宽度 12，实为 %d", cpu.Grid.W)
	}
}

func TestTokenize(t *testing.T) {
	got := tokenize("MySQL 连接数 qps_total rate() a")
	// 期望：mysql, qps_total, rate（"a" 单字符被丢，CJK 被分隔）
	want := map[string]bool{"mysql": true, "qps_total": true, "rate": true}
	if len(got) != 3 {
		t.Fatalf("分词数应为 3，实为 %d：%v", len(got), got)
	}
	for _, tok := range got {
		if !want[tok] {
			t.Fatalf("意外的词元 %q（全部：%v）", tok, got)
		}
	}
}

func TestWithNoThinkHint(t *testing.T) {
	cfg := AIConfig{Model: "qwen3-max", Endpoint: "https://dashscope.aliyuncs.com/compatible-mode/v1"}
	msgs := []map[string]string{
		{"role": "system", "content": "sys"},
		{"role": "user", "content": "生成看板"},
	}
	out := withNoThinkHint(msgs, cfg)
	if !strings.Contains(out[0]["content"], "禁止深度思考") {
		t.Fatalf("system 应注入禁止深度思考：%q", out[0]["content"])
	}
	if !strings.Contains(out[1]["content"], "/no_think") {
		t.Fatalf("user 应对 Qwen 追加 /no_think：%q", out[1]["content"])
	}
}

func TestThinkingModelOrGateway(t *testing.T) {
	if !thinkingModelOrGateway(AIConfig{Model: "qwen3-32b", Endpoint: "http://x"}) {
		t.Fatal("qwen3 应判定为思考模型")
	}
	if thinkingModelOrGateway(AIConfig{Model: "gpt-4o-mini", Endpoint: "https://api.openai.com/v1"}) {
		t.Fatal("gpt-4o-mini 不应注入 enable_thinking（避免 OpenAI 400）")
	}
}
