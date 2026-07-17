package main

import "testing"

// 新工具必须真的注册进去，否则 LLM 根本看不到它们。
func TestHardwareToolsRegistered(t *testing.T) {
	h := &HermesCore{tools: map[string]HermesTool{}}
	h.registerTools()
	want := []string{
		"query_hardware", "query_hardware_events", "query_hardware_history",
		"query_hardware_changes", "query_netflow",
	}
	for _, n := range want {
		tool, ok := h.tools[n]
		if !ok {
			t.Errorf("工具 %s 未注册 —— LLM 看不到它", n)
			continue
		}
		if tool.Execute == nil {
			t.Errorf("工具 %s 没有 Execute", n)
		}
		if tool.Description == "" {
			t.Errorf("工具 %s 没有描述 —— 模型不知道何时该调用", n)
		}
		p, _ := tool.Parameters["properties"].(map[string]any)
		if _, ok := p["host_id"]; !ok {
			t.Errorf("工具 %s 缺少 host_id 参数", n)
		}
	}
	if len(h.tools) < 15 {
		t.Errorf("工具总数 = %d, 期望 >=15（原 10 + 新增 5）", len(h.tools))
	}
}
