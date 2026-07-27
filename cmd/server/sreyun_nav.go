package main

import (
	"fmt"
	"strings"
)

// uiViewCatalog is the server-side mirror of PAGE_META / switchView targets.
// Keep labels short; aliases help natural-language resolve.
type uiViewDef struct {
	View    string
	Title   string
	Group   string
	Aliases []string
}

var uiViewCatalog = []uiViewDef{
	{View: "overview", Title: "首页", Group: "总览", Aliases: []string{"首页", "概览", "dashboard home"}},
	{View: "hosts", Title: "主机", Group: "总览", Aliases: []string{"主机列表", "服务器", "nodes"}},
	{View: "dashboards", Title: "仪表盘", Group: "可视化", Aliases: []string{"看板", "grafana", "仪表板"}},
	{View: "alerts", Title: "告警", Group: "告警", Aliases: []string{"当前告警", "报警"}},
	{View: "governance", Title: "告警治理", Group: "告警", Aliases: []string{"静默", "抑制", "路由"}},
	{View: "thresholds", Title: "告警阈值", Group: "告警", Aliases: []string{"阈值"}},
	{View: "checks", Title: "拨测监控", Group: "监控", Aliases: []string{"拨测", "合成监控", "uptime"}},
	{View: "apimon", Title: "API 监控", Group: "监控", Aliases: []string{"接口监控", "api监控"}},
	{View: "scrape", Title: "抓取目标", Group: "监控", Aliases: []string{"scrape", "exporter"}},
	{View: "automation", Title: "编排", Group: "编排", Aliases: []string{"自动化", "剧本", "playbook"}},
	{View: "forward", Title: "端口转发", Group: "编排", Aliases: []string{"转发", "隧道"}},
	{View: "sre", Title: "SRE 中枢", Group: "SRE", Aliases: []string{"事件", "故障", "诊断中心", "值班"}},
	{View: "logs", Title: "日志", Group: "可观测", Aliases: []string{"日志探索", "loki"}},
	{View: "log", Title: "审计日志", Group: "可观测", Aliases: []string{"操作日志", "审计"}},
	{View: "datasource", Title: "数据源", Group: "配置", Aliases: []string{"数据源管理"}},
	{View: "sql-toolkit", Title: "SQL 工具", Group: "配置", Aliases: []string{"sql", "mysql工具", "explain"}},
	{View: "hardware", Title: "物理硬件", Group: "资源", Aliases: []string{"硬件", "服务器硬件"}},
	{View: "hyperv", Title: "虚拟机", Group: "资源", Aliases: []string{"hyper-v", "vm"}},
	{View: "containers", Title: "容器", Group: "资源", Aliases: []string{"docker", "podman"}},
	{View: "k8s", Title: "Kubernetes", Group: "资源", Aliases: []string{"k8s", "kubernetes", "集群"}},
	{View: "netflow", Title: "网络流量", Group: "网络", Aliases: []string{"流量", "netflow"}},
	{View: "snmp", Title: "网络设备", Group: "网络", Aliases: []string{"snmp", "交换机"}},
	{View: "security-overview", Title: "安全总览", Group: "安全", Aliases: []string{"安全中心", "安全态势"}},
	{View: "host-security", Title: "主机安全", Group: "安全", Aliases: []string{"主机扫描", "漏洞扫描", "主机加固"}},
	{View: "web-security", Title: "Web 扫描", Group: "安全", Aliases: []string{"web漏洞", "漏扫", "web安全"}},
	{View: "content-audit", Title: "内容审计", Group: "安全", Aliases: []string{"明文审计", "http审计"}},
	{View: "ai-tool-audit", Title: "AI 工具审计", Group: "安全", Aliases: []string{"ai审计", "工具审计"}},
	{View: "audit-export", Title: "审计外发", Group: "安全", Aliases: []string{"外发", "审计导出"}},
	{View: "oidc", Title: "单点登录", Group: "安全", Aliases: []string{"sso", "oidc", "登录配置"}},
}

func (h *SreyunCore) registerNavTools() {
	h.tools["list_ui_views"] = SreyunTool{
		Name:        "list_ui_views",
		Description: "列出平台可打开的界面视图（view id / 标题 / 分组）。打开某页面前可先调用以确认 view。",
		Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
		Execute:     h.execListUIViews,
	}
	h.tools["navigate_ui"] = SreyunTool{
		Name: "navigate_ui",
		Description: "在客户端打开/调度到指定界面（安全中心、主机、告警、SRE、仪表盘、网络等）。" +
			"用户说「打开安全总览」「去主机页」「切到 Web 漏扫」时必须调用。" +
			"view 可用 list_ui_views 返回的 id，或中文别名。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"view":  map[string]string{"type": "string", "description": "视图 id 或别名，如 security-overview / 安全中心 / hosts"},
				"label": map[string]string{"type": "string", "description": "按钮文案（可选）"},
			},
			"required": []string{"view"},
		},
		Execute: h.execNavigateUI,
	}
}

func resolveUIView(raw string) (uiViewDef, bool) {
	key := strings.ToLower(strings.TrimSpace(raw))
	key = strings.ReplaceAll(key, "_", "-")
	key = strings.ReplaceAll(key, " ", "")
	if key == "" {
		return uiViewDef{}, false
	}
	for _, v := range uiViewCatalog {
		if strings.EqualFold(v.View, key) || strings.EqualFold(v.Title, raw) {
			return v, true
		}
		for _, a := range v.Aliases {
			aNorm := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(a, " ", ""), "_", "-"))
			if aNorm == key || strings.EqualFold(a, raw) {
				return v, true
			}
		}
	}
	return uiViewDef{}, false
}

func (h *SreyunCore) execListUIViews(args map[string]any) (string, error) {
	type row struct {
		View  string `json:"view"`
		Title string `json:"title"`
		Group string `json:"group"`
	}
	out := make([]row, 0, len(uiViewCatalog))
	for _, v := range uiViewCatalog {
		out = append(out, row{View: v.View, Title: v.Title, Group: v.Group})
	}
	return capabilityJSON(capabilityResult{
		OK:      true,
		Summary: fmt.Sprintf("共 %d 个可导航界面", len(out)),
		Data:    out,
	}), nil
}

func (h *SreyunCore) execNavigateUI(args map[string]any) (string, error) {
	raw, _ := args["view"].(string)
	label, _ := args["label"].(string)
	v, ok := resolveUIView(raw)
	if !ok {
		return capabilityJSON(capabilityResult{
			OK:    false,
			Error: fmt.Sprintf("未知界面 %q，请先 list_ui_views", strings.TrimSpace(raw)),
		}), nil
	}
	if strings.TrimSpace(label) == "" {
		label = "打开 · " + v.Title
	}
	return capabilityJSON(capabilityResult{
		OK:        true,
		Summary:   fmt.Sprintf("已调度打开「%s」(%s)", v.Title, v.View),
		Data:      map[string]any{"view": v.View, "title": v.Title, "group": v.Group},
		UIActions: []map[string]any{navigateViewAction(v.View, label, v.Title)},
	}), nil
}
