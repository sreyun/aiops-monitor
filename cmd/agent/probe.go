package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"aiops-monitor/shared"
)

// ============================================================================
// 分布式多点探测 · agent 侧（迭代 D）
//
// 服务端在上报响应里下发 ProbeTask（被标为「分布式」的 API 接口），本 agent 作为一个
// 探测点从自身网络位置执行 HTTP 探测并把结果回报。多地 agent 各自探测同一接口，服务端
// 据此聚合出区域性 vs 全局故障。同一 target 同时只跑一轮（probeMu.TryLock），避免慢
// 探测在高频上报下堆积。
// ============================================================================

// runProbeTasks 执行下发的探测任务并把结果回报给该 target。
func (t *serverTarget) runProbeTasks(hostID, hostname, fingerprint string, tasks []shared.ProbeTask) {
	if !t.probeMu.TryLock() {
		return // 上一轮还在跑，跳过本轮
	}
	defer t.probeMu.Unlock()

	client := &http.Client{} // 探测任意目标；每个任务用 context 控制超时（不与上报 client 混用）
	results := make([]shared.ProbeResult, 0, len(tasks))
	for _, task := range tasks {
		results = append(results, execProbe(client, task))
	}
	if len(results) == 0 {
		return
	}
	body, err := json.Marshal(shared.ProbeResultReport{
		HostID: hostID, Hostname: hostname, Fingerprint: fingerprint, Results: results,
	})
	if err != nil {
		return
	}
	req, err := http.NewRequest("POST", t.server+"/api/v1/agent/probe-results", bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Fingerprint", fingerprint) // 指纹鉴权（与其它 agent ingest 一致）
	resp, err := t.httpc.Do(req)
	if err != nil {
		return
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	resp.Body.Close()
}

// execProbe 执行单个 HTTP 探测任务并计时，按期望状态码/默认 <400 判定成功。
func execProbe(client *http.Client, task shared.ProbeTask) shared.ProbeResult {
	to := task.TimeoutSec
	if to <= 0 {
		to = 10
	}
	method := task.Method
	if method == "" {
		method = "GET"
	}
	res := shared.ProbeResult{TaskID: task.ID, Ts: time.Now().Unix()}
	var bodyReader io.Reader
	if task.Body != "" {
		bodyReader = bytes.NewReader([]byte(task.Body))
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(to)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, task.URL, bodyReader)
	if err != nil {
		res.Msg = err.Error()
		return res
	}
	for k, v := range task.Headers {
		if k != "" {
			req.Header.Set(k, v)
		}
	}
	if req.Header.Get("traceparent") == "" {
		if tp := agentTraceparent(); tp != "" {
			req.Header.Set("traceparent", tp) // 分布式探测请求可被后端 trace 关联
		}
	}
	start := time.Now()
	resp, err := client.Do(req)
	res.LatencyMs = float64(time.Since(start).Microseconds()) / 1000
	if err != nil {
		res.Msg = err.Error()
		return res
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	resp.Body.Close()
	res.Code = resp.StatusCode
	if task.ExpectStatus > 0 {
		res.OK = resp.StatusCode == task.ExpectStatus
	} else {
		res.OK = resp.StatusCode < 400
	}
	if !res.OK && res.Msg == "" {
		res.Msg = "HTTP " + resp.Status
	}
	return res
}

// agentTraceparent 生成 W3C traceparent 头值（00-<32hex traceID>-<16hex spanID>-01），
// 使分布式探测请求可被后端分布式追踪系统关联。
func agentTraceparent() string {
	b := make([]byte, 24) // 16B trace-id + 8B span-id
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return "00-" + hex.EncodeToString(b[:16]) + "-" + hex.EncodeToString(b[16:24]) + "-01"
}
