package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

func (h *SreyunCore) registerResourceTools() {
	h.tools["query_containers"] = SreyunTool{
		Name: "query_containers",
		Description: "查询纳管主机上的 Docker/Podman 容器清单（名称/镜像/状态/端口）。" +
			"排查「某主机上跑了哪些容器、哪些已退出」时使用。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"host_id": map[string]string{"type": "string", "description": "主机 ID；空则返回全部主机摘要"},
			},
		},
		Execute: h.execQueryContainers,
	}
	h.tools["query_k8s"] = SreyunTool{
		Name: "query_k8s",
		Description: "查询已登记的 Kubernetes 集群资源。kind=clusters|nodes|pods|deployments|events|log。" +
			"需要 cluster_id（clusters 除外）。排查集群/Pod/Node 时优先用此工具，勿依赖服务端本机 kubectl。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"kind":        map[string]string{"type": "string", "description": "clusters|nodes|pods|deployments|events|log"},
				"cluster_id":  map[string]string{"type": "string", "description": "集群 ID"},
				"namespace":   map[string]string{"type": "string", "description": "命名空间，空=全部/默认"},
				"pod":         map[string]string{"type": "string", "description": "kind=log 时的 Pod 名"},
				"limit":       map[string]string{"type": "integer", "description": "列表上限"},
			},
		},
		Execute: h.execQueryK8s,
	}
	h.tools["k8s_scale"] = SreyunTool{
		Name:        "k8s_scale",
		Description: "调整 Deployment 副本数（写操作，需审批策略允许）。参数：cluster_id、namespace、name、replicas。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"cluster_id": map[string]string{"type": "string"},
				"namespace":  map[string]string{"type": "string"},
				"name":       map[string]string{"type": "string"},
				"replicas":   map[string]string{"type": "integer"},
			},
			"required": []string{"cluster_id", "namespace", "name", "replicas"},
		},
		Execute: h.execK8sScale,
	}
	h.tools["k8s_restart"] = SreyunTool{
		Name:        "k8s_restart",
		Description: "对 Deployment 执行 rollout restart（写操作）。参数：cluster_id、namespace、name。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"cluster_id": map[string]string{"type": "string"},
				"namespace":  map[string]string{"type": "string"},
				"name":       map[string]string{"type": "string"},
			},
			"required": []string{"cluster_id", "namespace", "name"},
		},
		Execute: h.execK8sRestart,
	}
	h.tools["locate_resource"] = SreyunTool{
		Name: "locate_resource",
		Description: "跨层定位资源：输入 host:<id> / vm:<host>/<vm> / container:<host>/<id> / pod:<cluster>/<ns>/<name> / svc:<name>，" +
			"返回硬件→虚拟机→主机→容器/Pod 关联链与告警摘要。容器/服务异常时先用它定位落点。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ref": map[string]string{"type": "string", "description": "资源引用，如 host:abc、pod:prod/default/api-0"},
			},
			"required": []string{"ref"},
		},
		Execute: h.execLocateResource,
	}
}

func (h *SreyunCore) execQueryContainers(args map[string]any) (string, error) {
	if h.s == nil || h.s.pg == nil {
		return "无 PostgreSQL，无法查询容器清单", nil
	}
	hostID, _ := args["host_id"].(string)
	var rows []map[string]any
	if strings.TrimSpace(hostID) != "" {
		if inv, ok := h.s.pg.getContainerInventory(hostID); ok {
			rows = []map[string]any{inv}
		}
	} else {
		var err error
		rows, err = h.s.pg.getAllContainerInventories()
		if err != nil {
			return "", err
		}
	}
	b, _ := json.MarshalIndent(rows, "", "  ")
	if len(b) > 12000 {
		b = b[:12000]
	}
	return string(b), nil
}

func (h *SreyunCore) execQueryK8s(args map[string]any) (string, error) {
	kind, _ := args["kind"].(string)
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "" {
		kind = "clusters"
	}
	if kind == "clusters" {
		list := h.s.cfg.ListK8sClusters()
		out := make([]map[string]any, 0, len(list))
		for _, c := range list {
			out = append(out, map[string]any{"id": c.ID, "name": c.Name, "enabled": c.Enabled, "default_namespace": c.DefaultNS})
		}
		b, _ := json.MarshalIndent(out, "", "  ")
		return string(b), nil
	}
	cid, _ := args["cluster_id"].(string)
	c, ok := h.s.cfg.GetK8sCluster(cid)
	if !ok || !c.Enabled {
		return "", fmt.Errorf("cluster not found or disabled")
	}
	cli, err := newK8sRESTClient(c)
	if err != nil {
		return "", err
	}
	ns, _ := args["namespace"].(string)
	if ns == "" {
		ns = c.DefaultNS
	}
	limit := 100
	if v, ok := args["limit"].(float64); ok && int(v) > 0 {
		limit = int(v)
	}
	if v, ok := args["limit"].(string); ok {
		if n, e := strconv.Atoi(v); e == nil && n > 0 {
			limit = n
		}
	}
	switch kind {
	case "nodes":
		items, err := cli.ListNodes()
		if err != nil {
			return "", err
		}
		rows := []map[string]any{}
		for _, it := range items {
			_, name := k8sMetaName(it)
			row := map[string]any{"name": name, "ready": k8sNodeReady(it)}
			h.s.enrichK8sNodeRow(it, row)
			rows = append(rows, row)
		}
		b, _ := json.MarshalIndent(rows, "", "  ")
		return string(b), nil
	case "pods":
		items, err := cli.ListPods(ns, limit)
		if err != nil {
			return "", err
		}
		b, _ := json.MarshalIndent(summarizeK8sPods(h.s, items), "", "  ")
		return string(b), nil
	case "deployments":
		items, err := cli.ListDeployments(ns, limit)
		if err != nil {
			return "", err
		}
		rows := []map[string]any{}
		for _, it := range items {
			dns, name := k8sMetaName(it)
			d, ready, avail := k8sDeployReplicas(it)
			rows = append(rows, map[string]any{"namespace": dns, "name": name, "replicas": d, "ready": ready, "available": avail})
		}
		b, _ := json.MarshalIndent(rows, "", "  ")
		return string(b), nil
	case "events":
		items, err := cli.ListEvents(ns, limit)
		if err != nil {
			return "", err
		}
		b, _ := json.MarshalIndent(items, "", "  ")
		if len(b) > 12000 {
			b = b[:12000]
		}
		return string(b), nil
	case "log":
		pod, _ := args["pod"].(string)
		if pod == "" || ns == "" {
			return "", fmt.Errorf("log 需要 namespace 与 pod")
		}
		text, err := cli.PodLogs(ns, pod, 200)
		if err != nil {
			return "", err
		}
		if len(text) > 12000 {
			text = text[:12000]
		}
		return text, nil
	default:
		return "", fmt.Errorf("unknown kind %q", kind)
	}
}

func summarizeK8sPods(s *Server, items []map[string]any) []map[string]any {
	rows := make([]map[string]any, 0, len(items))
	for _, it := range items {
		pns, name := k8sMetaName(it)
		node := ""
		if spec, _ := it["spec"].(map[string]any); spec != nil {
			node, _ = spec["nodeName"].(string)
		}
		row := map[string]any{"namespace": pns, "name": name, "phase": k8sPodPhase(it), "node": node}
		if node != "" {
			if hid, hname := s.hostIDForK8sNodeName(node); hid != "" {
				row["linked_host_id"] = hid
				row["linked_host_name"] = hname
			}
		}
		rows = append(rows, row)
	}
	return rows
}

func (h *SreyunCore) sreyunWriteBlocked(tool, detail string) (string, bool) {
	aiCfg := h.s.cfg.AIConfig()
	blocked := aiCfg.WriteToolsRequireApproval || !aiCfg.SreyunAutoApprove
	if h.s.aiGov != nil {
		h.s.aiGov.recordTool(aiToolAuditEntry{
			Actor: "sreyun", Tool: tool, Action: tool, Approved: !blocked, Blocked: blocked, Detail: detail,
		})
	}
	if blocked {
		return fmt.Sprintf("工具 %s 属于高风险写操作，需人工确认。请操作员在面板执行，或在 AI 治理中允许写工具自动执行后重试。", tool), true
	}
	return "", false
}

func (h *SreyunCore) execK8sScale(args map[string]any) (string, error) {
	cid, _ := args["cluster_id"].(string)
	ns, _ := args["namespace"].(string)
	name, _ := args["name"].(string)
	if msg, blocked := h.sreyunWriteBlocked("k8s_scale", cid+"/"+ns+"/"+name); blocked {
		return msg, nil
	}
	var replicas int32
	switch v := args["replicas"].(type) {
	case float64:
		replicas = int32(v)
	case string:
		n, _ := strconv.Atoi(v)
		replicas = int32(n)
	case int:
		replicas = int32(v)
	}
	c, ok := h.s.cfg.GetK8sCluster(cid)
	if !ok || !c.Enabled {
		return "", fmt.Errorf("cluster not found or disabled")
	}
	cli, err := newK8sRESTClient(c)
	if err != nil {
		return "", err
	}
	old, _ := cli.GetDeploymentScale(ns, name)
	if err := cli.ScaleDeployment(ns, name, replicas); err != nil {
		return "", err
	}
	if h.s.store != nil {
		h.s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: "sreyun",
			Message: fmt.Sprintf("Sreyun K8s Scale：集群=%s ns=%s deploy=%s replicas=%d→%d", c.Name, ns, name, old, replicas)})
	}
	return fmt.Sprintf("ok scale %s/%s %d→%d", ns, name, old, replicas), nil
}

func (h *SreyunCore) execK8sRestart(args map[string]any) (string, error) {
	cid, _ := args["cluster_id"].(string)
	ns, _ := args["namespace"].(string)
	name, _ := args["name"].(string)
	if msg, blocked := h.sreyunWriteBlocked("k8s_restart", cid+"/"+ns+"/"+name); blocked {
		return msg, nil
	}
	c, ok := h.s.cfg.GetK8sCluster(cid)
	if !ok || !c.Enabled {
		return "", fmt.Errorf("cluster not found or disabled")
	}
	cli, err := newK8sRESTClient(c)
	if err != nil {
		return "", err
	}
	if err := cli.RestartDeployment(ns, name); err != nil {
		return "", err
	}
	if h.s.store != nil {
		h.s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: "sreyun",
			Message: fmt.Sprintf("Sreyun K8s Restart：集群=%s ns=%s deploy=%s", c.Name, ns, name)})
	}
	return fmt.Sprintf("ok restart %s/%s", ns, name), nil
}

func (h *SreyunCore) execLocateResource(args map[string]any) (string, error) {
	ref, _ := args["ref"].(string)
	if strings.TrimSpace(ref) == "" {
		return "", fmt.Errorf("ref required")
	}
	res := h.s.locateResource(ref)
	b, _ := json.MarshalIndent(res, "", "  ")
	return string(b), nil
}
