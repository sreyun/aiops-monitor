package main

import "testing"

// 新工具必须真的注册进去，否则 LLM 根本看不到它们。
func TestHardwareToolsRegistered(t *testing.T) {
	h := &SreyunCore{tools: map[string]SreyunTool{}}
	h.registerTools()
	want := []string{
		"query_hardware", "query_hardware_events", "query_hardware_history",
		"query_hardware_changes", "query_netflow", "query_hyperv",
		"query_containers", "query_k8s", "locate_resource",
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
	}
	if len(h.tools) < 20 {
		t.Errorf("工具总数 = %d, 期望 >=20（含容器/K8s/locate）", len(h.tools))
	}
}
