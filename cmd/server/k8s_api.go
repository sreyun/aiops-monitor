package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) k8sClusterOrErr(w http.ResponseWriter, r *http.Request) (K8sClusterConfig, bool) {
	id := strings.TrimSpace(r.PathValue("id"))
	c, ok := s.cfg.GetK8sCluster(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "cluster not found"})
		return K8sClusterConfig{}, false
	}
	if !c.Enabled {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cluster disabled"})
		return K8sClusterConfig{}, false
	}
	return c, true
}

func (s *Server) k8sClientOrErr(w http.ResponseWriter, cfg K8sClusterConfig) (*k8sRESTClient, bool) {
	cli, err := newK8sRESTClient(cfg)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return nil, false
	}
	return cli, true
}

func (s *Server) handleListK8sClusters(w http.ResponseWriter, r *http.Request) {
	list := s.cfg.ListK8sClusters()
	out := make([]K8sClusterConfig, 0, len(list))
	for _, c := range list {
		out = append(out, maskK8sCluster(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"clusters": out})
}

func (s *Server) handleGetK8sCluster(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	c, ok := s.cfg.GetK8sCluster(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "cluster not found"})
		return
	}
	writeJSON(w, http.StatusOK, maskK8sCluster(c))
}

func (s *Server) handleUpsertK8sCluster(w http.ResponseWriter, r *http.Request) {
	var in K8sClusterConfig
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	if id := strings.TrimSpace(r.PathValue("id")); id != "" {
		in.ID = id
	}
	saved, err := s.cfg.UpsertK8sCluster(in)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.actorName(r), IP: s.clientIP(r),
		Message: fmt.Sprintf("更新 K8s 集群配置「%s」", saved.Name)})
	writeJSON(w, http.StatusOK, maskK8sCluster(saved))
}

func (s *Server) handleDeleteK8sCluster(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	c, ok := s.cfg.GetK8sCluster(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "cluster not found"})
		return
	}
	if err := s.cfg.DeleteK8sCluster(id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.actorName(r), IP: s.clientIP(r),
		Message: fmt.Sprintf("删除 K8s 集群配置「%s」", c.Name)})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleTestK8sCluster(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	c, ok := s.cfg.GetK8sCluster(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "cluster not found"})
		return
	}
	cli, err := newK8sRESTClient(c)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	ver, err := cli.Version()
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": ver})
}

func (s *Server) handleK8sNamespaces(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.k8sClusterOrErr(w, r)
	if !ok {
		return
	}
	cli, ok := s.k8sClientOrErr(w, cfg)
	if !ok {
		return
	}
	items, err := cli.ListNamespaces()
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	rows := make([]map[string]any, 0, len(items))
	for _, it := range items {
		_, name := k8sMetaName(it)
		phase := ""
		if st, _ := it["status"].(map[string]any); st != nil {
			phase, _ = st["phase"].(string)
		}
		rows = append(rows, map[string]any{"name": name, "phase": phase})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

func (s *Server) handleK8sNodes(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.k8sClusterOrErr(w, r)
	if !ok {
		return
	}
	cli, ok := s.k8sClientOrErr(w, cfg)
	if !ok {
		return
	}
	items, err := cli.ListNodes()
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	rows := make([]map[string]any, 0, len(items))
	for _, it := range items {
		_, name := k8sMetaName(it)
		row := map[string]any{
			"name": name, "ready": k8sNodeReady(it),
		}
		s.enrichK8sNodeRow(it, row)
		rows = append(rows, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

func (s *Server) handleK8sPods(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.k8sClusterOrErr(w, r)
	if !ok {
		return
	}
	cli, ok := s.k8sClientOrErr(w, cfg)
	if !ok {
		return
	}
	ns := r.URL.Query().Get("namespace")
	if ns == "" {
		ns = cfg.DefaultNS
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 500
	}
	items, err := cli.ListPods(ns, limit)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	rows := make([]map[string]any, 0, len(items))
	for _, it := range items {
		pns, name := k8sMetaName(it)
		node := ""
		ip := ""
		if spec, _ := it["spec"].(map[string]any); spec != nil {
			node, _ = spec["nodeName"].(string)
		}
		if st, _ := it["status"].(map[string]any); st != nil {
			ip, _ = st["podIP"].(string)
		}
		row := map[string]any{
			"namespace": pns, "name": name, "phase": k8sPodPhase(it),
			"node": node, "ip": ip,
		}
		if node != "" {
			if hid, hname := s.hostIDForK8sNodeName(node); hid != "" {
				row["linked_host_id"] = hid
				row["linked_host_name"] = hname
			}
		}
		rows = append(rows, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

func (s *Server) handleK8sDeployments(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.k8sClusterOrErr(w, r)
	if !ok {
		return
	}
	cli, ok := s.k8sClientOrErr(w, cfg)
	if !ok {
		return
	}
	ns := r.URL.Query().Get("namespace")
	if ns == "" {
		ns = cfg.DefaultNS
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 500
	}
	items, err := cli.ListDeployments(ns, limit)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	rows := make([]map[string]any, 0, len(items))
	for _, it := range items {
		dns, name := k8sMetaName(it)
		d, ready, avail := k8sDeployReplicas(it)
		rows = append(rows, map[string]any{
			"namespace": dns, "name": name,
			"replicas": d, "ready": ready, "available": avail,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

func (s *Server) handleK8sEvents(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.k8sClusterOrErr(w, r)
	if !ok {
		return
	}
	cli, ok := s.k8sClientOrErr(w, cfg)
	if !ok {
		return
	}
	ns := r.URL.Query().Get("namespace")
	if ns == "" {
		ns = cfg.DefaultNS
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 200
	}
	items, err := cli.ListEvents(ns, limit)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	rows := make([]map[string]any, 0, len(items))
	for _, it := range items {
		ens, name := k8sMetaName(it)
		typ, _ := it["type"].(string)
		reason, _ := it["reason"].(string)
		msg, _ := it["message"].(string)
		count := 0
		switch v := it["count"].(type) {
		case float64:
			count = int(v)
		case int:
			count = v
		}
		obj := ""
		if involved, _ := it["involvedObject"].(map[string]any); involved != nil {
			kind, _ := involved["kind"].(string)
			oname, _ := involved["name"].(string)
			obj = strings.TrimSpace(kind + "/" + oname)
		}
		rows = append(rows, map[string]any{
			"namespace": ens, "name": name, "type": typ, "reason": reason,
			"message": msg, "count": count, "object": obj,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

func (s *Server) handleK8sPodLog(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.k8sClusterOrErr(w, r)
	if !ok {
		return
	}
	cli, ok := s.k8sClientOrErr(w, cfg)
	if !ok {
		return
	}
	ns := r.PathValue("ns")
	name := r.PathValue("name")
	tail, _ := strconv.Atoi(r.URL.Query().Get("tail"))
	text, err := cli.PodLogs(ns, name, tail)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"log": text})
}

func (s *Server) handleK8sScaleDeployment(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.k8sClusterOrErr(w, r)
	if !ok {
		return
	}
	cli, ok := s.k8sClientOrErr(w, cfg)
	if !ok {
		return
	}
	var req struct {
		Replicas int32 `json:"replicas"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	ns := r.PathValue("ns")
	name := r.PathValue("name")
	oldReplicas, _ := cli.GetDeploymentScale(ns, name)
	if err := cli.ScaleDeployment(ns, name, req.Replicas); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.actorName(r), IP: s.clientIP(r),
		Message: fmt.Sprintf("K8s Scale：集群=%s ns=%s deploy=%s replicas=%d→%d", cfg.Name, ns, name, oldReplicas, req.Replicas)})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "old_replicas": oldReplicas, "replicas": req.Replicas})
}

func (s *Server) handleK8sRestartDeployment(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.k8sClusterOrErr(w, r)
	if !ok {
		return
	}
	cli, ok := s.k8sClientOrErr(w, cfg)
	if !ok {
		return
	}
	ns := r.PathValue("ns")
	name := r.PathValue("name")
	if err := cli.RestartDeployment(ns, name); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.actorName(r), IP: s.clientIP(r),
		Message: fmt.Sprintf("K8s Restart：集群=%s ns=%s deploy=%s", cfg.Name, ns, name)})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleK8sUndoDeployment(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.k8sClusterOrErr(w, r)
	if !ok {
		return
	}
	cli, ok := s.k8sClientOrErr(w, cfg)
	if !ok {
		return
	}
	ns := r.PathValue("ns")
	name := r.PathValue("name")
	if err := cli.UndoDeploymentRollout(ns, name); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.actorName(r), IP: s.clientIP(r),
		Message: fmt.Sprintf("K8s Undo：集群=%s ns=%s deploy=%s", cfg.Name, ns, name)})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleK8sDeletePod(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.k8sClusterOrErr(w, r)
	if !ok {
		return
	}
	cli, ok := s.k8sClientOrErr(w, cfg)
	if !ok {
		return
	}
	ns := r.PathValue("ns")
	name := r.PathValue("name")
	if err := cli.DeletePod(ns, name); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.actorName(r), IP: s.clientIP(r),
		Message: fmt.Sprintf("K8s DeletePod：集群=%s ns=%s pod=%s", cfg.Name, ns, name)})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleK8sApply(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.k8sClusterOrErr(w, r)
	if !ok {
		return
	}
	var req struct {
		YAML      string `json:"yaml"`
		Namespace string `json:"namespace"`
		DryRun    bool   `json:"dry_run"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	out, err := ApplyYAML(cfg, req.YAML, req.Namespace, req.DryRun)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error(), "output": out})
		return
	}
	level := "info"
	if !req.DryRun {
		level = "warning"
	}
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: level, Actor: s.actorName(r), IP: s.clientIP(r),
		Message: fmt.Sprintf("K8s Apply%s：集群=%s ns=%s", map[bool]string{true: "(dry-run)", false: ""}[req.DryRun], cfg.Name, req.Namespace)})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "output": out, "dry_run": req.DryRun})
}

func (s *Server) handleK8sCreateNamespace(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.k8sClusterOrErr(w, r)
	if !ok {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	out, err := CreateNamespace(cfg, req.Name)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error(), "output": out})
		return
	}
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "warning", Actor: s.actorName(r), IP: s.clientIP(r),
		Message: fmt.Sprintf("K8s CreateNamespace：集群=%s ns=%s", cfg.Name, req.Name)})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "output": out})
}

func (s *Server) handleK8sPodExec(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.k8sClusterOrErr(w, r)
	if !ok {
		return
	}
	cli, ok := s.k8sClientOrErr(w, cfg)
	if !ok {
		return
	}
	var req struct {
		Command    string `json:"command"`
		TimeoutSec int    `json:"timeout_sec"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": Tr(r, "common.invalid_json")})
		return
	}
	ns := r.PathValue("ns")
	name := r.PathValue("name")
	out, err := cli.PodExecShort(ns, name, req.Command, req.TimeoutSec)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error(), "output": out})
		return
	}
	s.store.AddLog(LogEntry{Kind: KindOperation, Level: "info", Actor: s.actorName(r), IP: s.clientIP(r),
		Message: fmt.Sprintf("K8s Exec：集群=%s ns=%s pod=%s", cfg.Name, ns, name)})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "output": out})
}

func (s *Server) handleK8sOverview(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.k8sClusterOrErr(w, r)
	if !ok {
		return
	}
	cli, ok := s.k8sClientOrErr(w, cfg)
	if !ok {
		return
	}
	ver, err := cli.Version()
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	nodes, _ := cli.ListNodes()
	pods, _ := cli.ListPods(cfg.DefaultNS, 500)
	deploys, _ := cli.ListDeployments(cfg.DefaultNS, 500)
	readyNodes := 0
	for _, n := range nodes {
		if k8sNodeReady(n) == "Ready" {
			readyNodes++
		}
	}
	runningPods := 0
	for _, p := range pods {
		if k8sPodPhase(p) == "Running" {
			runningPods++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version": ver,
		"nodes":   map[string]any{"total": len(nodes), "ready": readyNodes},
		"pods":    map[string]any{"total": len(pods), "running": runningPods},
		"deployments": map[string]any{"total": len(deploys)},
	})
}
