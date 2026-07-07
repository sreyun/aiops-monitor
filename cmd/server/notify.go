package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Notifier evaluates alerts on a timer and pushes deduplicated notifications
// to Feishu / DingTalk bots. Only alert transitions (fire / resolve) are sent,
// so a persistent condition never spams the channel.
type Notifier struct {
	store  *Store
	cfg    *ConfigStore
	httpc  *http.Client
	mu     sync.Mutex
	active map[string]Alert // alertKey -> alert currently firing
	since  map[string]int64 // alertKey -> unix time the alert first fired
}

func NewNotifier(store *Store, cfg *ConfigStore) *Notifier {
	return &Notifier{
		store:  store,
		cfg:    cfg,
		httpc:  &http.Client{Timeout: 8 * time.Second},
		active: map[string]Alert{},
		since:  map[string]int64{},
	}
}

func alertKey(a Alert) string { return a.HostID + "/" + a.Type + "/" + a.Scope }

// Run evaluates alerts every interval and notifies on state transitions.
func (n *Notifier) Run(interval time.Duration) {
	n.tick() // evaluate promptly on startup
	t := time.NewTicker(interval)
	defer t.Stop()
	for range t.C {
		n.tick()
	}
}

// ResetState clears the fire/resolve memory so the next evaluation re-pushes
// every currently-active alert. Called after the alert config changes, so a
// freshly configured webhook receives the outstanding alerts instead of them
// being silently swallowed as "already seen".
func (n *Notifier) ResetState() {
	n.mu.Lock()
	n.active = map[string]Alert{}
	n.mu.Unlock()
}

// Trigger runs one evaluation immediately (used right after a config save).
func (n *Notifier) Trigger() { n.tick() }

// ActiveSince returns a copy of the first-fired times keyed by alertKey,
// letting the alerts API show "已持续 X 分钟".
func (n *Notifier) ActiveSince() map[string]int64 {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make(map[string]int64, len(n.since))
	for k, v := range n.since {
		out[k] = v
	}
	return out
}

func (n *Notifier) tick() {
	cfg := n.cfg.Get()
	alerts := Evaluate(n.store.ListHosts(), n.cfg.Thresholds())
	cur := make(map[string]Alert, len(alerts))
	for _, a := range alerts {
		cur[alertKey(a)] = a
	}
	// Compute transitions under the lock, then dispatch (network I/O) outside it.
	type transition struct {
		a      Alert
		firing bool
	}
	var todo []transition
	now := time.Now().Unix()
	n.mu.Lock()
	if cfg.AlertsEnabled {
		for k, a := range cur { // newly fired
			if _, ok := n.active[k]; !ok {
				todo = append(todo, transition{a, true})
			}
		}
		for k, a := range n.active { // resolved
			if _, ok := cur[k]; !ok {
				todo = append(todo, transition{a, false})
			}
		}
	}
	// first-fired bookkeeping (kept across ResetState so durations survive
	// config saves; only set when absent, cleared when the alert resolves)
	for k := range cur {
		if _, ok := n.since[k]; !ok {
			n.since[k] = now
		}
	}
	for k := range n.since {
		if _, ok := cur[k]; !ok {
			delete(n.since, k)
		}
	}
	n.active = cur // track state even while disabled, so re-enabling won't replay
	n.mu.Unlock()

	for _, t := range todo {
		n.dispatch(cfg, t.a, t.firing)
	}
}

func (n *Notifier) dispatch(cfg ServerConfig, a Alert, firing bool) {
	// activity log: the machine-detected threshold transition (intervention)
	verb, tlvl := "告警触发", a.Level
	if !firing {
		verb, tlvl = "告警恢复", "info"
	}
	n.store.AddLog(LogEntry{Kind: "系统", Level: tlvl, Actor: "告警引擎", Host: a.Hostname, Message: verb + "：" + a.Message})
	n.pushChannels(cfg, a, firing)
}

// pushChannels sends the alert text to every enabled bot channel and logs the
// push result. Shared by threshold alerts and custom-check alerts.
func (n *Notifier) pushChannels(cfg ServerConfig, a Alert, firing bool) {
	text := formatAlert(a, firing)
	var sent []string
	if cfg.Feishu.Enabled && cfg.Feishu.Webhook != "" {
		if err := n.sendFeishu(cfg.Feishu, text); err != nil {
			log.Printf("飞书推送失败: %v", err)
			n.store.AddLog(LogEntry{Kind: "系统", Level: "warning", Actor: "通知", Host: a.Hostname, Message: "飞书推送失败：" + err.Error()})
		} else {
			sent = append(sent, "飞书")
		}
	}
	if cfg.Dingtalk.Enabled && cfg.Dingtalk.Webhook != "" {
		if err := n.sendDingtalk(cfg.Dingtalk, text); err != nil {
			log.Printf("钉钉推送失败: %v", err)
			n.store.AddLog(LogEntry{Kind: "系统", Level: "warning", Actor: "通知", Host: a.Hostname, Message: "钉钉推送失败：" + err.Error()})
		} else {
			sent = append(sent, "钉钉")
		}
	}
	if len(sent) > 0 {
		n.store.AddLog(LogEntry{Kind: "系统", Level: "info", Actor: "通知", Host: a.Hostname, Message: "已推送 " + strings.Join(sent, "/") + "：" + a.Message})
	}
}

func formatAlert(a Alert, firing bool) string {
	status := "🔴 触发"
	if !firing {
		status = "✅ 恢复"
	}
	lv := "警告"
	if a.Level == "critical" {
		lv = "严重"
	}
	typeMap := map[string]string{
		"cpu": "CPU", "memory": "内存", "disk": "磁盘", "offline": "主机失联",
		"load": "系统负载", "gpu": "GPU", "check": "自定义监控",
	}
	typeLabel := typeMap[a.Type]
	if typeLabel == "" {
		typeLabel = a.Type
	}
	ipLine := ""
	if a.IP != "" {
		ipLine = fmt.Sprintf("\nIP: %s", a.IP)
	}
	return fmt.Sprintf("【AIOps Monitor】%s\n主机: %s%s\n级别: %s\n类型: %s\n详情: %s\n时间: %s",
		status, a.Hostname, ipLine, lv, typeLabel, a.Message, time.Unix(a.Timestamp, 0).Format("2006-01-02 15:04:05"))
}

// SendTest pushes a one-off test message on the enabled channels of the given
// config and returns human-readable errors (empty on full success).
func (n *Notifier) SendTest(cfg ServerConfig) []string {
	msg := "【AIOps Monitor】测试消息：告警通道连通正常 ✅\n时间: " + time.Now().Format("2006-01-02 15:04:05")
	var errs []string
	if cfg.Feishu.Enabled && cfg.Feishu.Webhook != "" {
		if err := n.sendFeishu(cfg.Feishu, msg); err != nil {
			errs = append(errs, "飞书: "+err.Error())
		}
	}
	if cfg.Dingtalk.Enabled && cfg.Dingtalk.Webhook != "" {
		if err := n.sendDingtalk(cfg.Dingtalk, msg); err != nil {
			errs = append(errs, "钉钉: "+err.Error())
		}
	}
	if !cfg.Feishu.Enabled && !cfg.Dingtalk.Enabled {
		errs = append(errs, "未启用任何告警通道")
	}
	return errs
}

func (n *Notifier) sendFeishu(c WebhookConfig, text string) error {
	body, _ := json.Marshal(map[string]any{
		"msg_type": "text",
		"content":  map[string]string{"text": text},
	})
	return n.post(c.Webhook, body)
}

func (n *Notifier) sendDingtalk(c WebhookConfig, text string) error {
	webhook := c.Webhook
	if c.Secret != "" {
		ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
		sep := "?"
		if strings.Contains(webhook, "?") {
			sep = "&"
		}
		webhook = webhook + sep + "timestamp=" + ts + "&sign=" + dingSign(ts, c.Secret)
	}
	body, _ := json.Marshal(map[string]any{
		"msgtype": "text",
		"text":    map[string]string{"content": text},
	})
	return n.post(webhook, body)
}

// dingSign implements DingTalk's HMAC-SHA256 signature: base64(hmac(secret,
// "timestamp\nsecret")), URL-encoded.
func dingSign(ts, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(ts + "\n" + secret))
	return url.QueryEscape(base64.StdEncoding.EncodeToString(h.Sum(nil)))
}

func (n *Notifier) post(webhook string, body []byte) error {
	resp, err := n.httpc.Post(webhook, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(rb)))
	}
	// Feishu / DingTalk return HTTP 200 even on business errors (bad keyword,
	// sign mismatch, ...); the real status is in the body's code / errcode.
	var r struct {
		Code    int    `json:"code"`
		Msg     string `json:"msg"`
		Errcode int    `json:"errcode"`
		Errmsg  string `json:"errmsg"`
	}
	if json.Unmarshal(rb, &r) == nil && (r.Code != 0 || r.Errcode != 0) {
		code, msg := r.Code, r.Msg
		if code == 0 {
			code, msg = r.Errcode, r.Errmsg
		}
		return fmt.Errorf("接口返回 code=%d %s", code, msg)
	}
	return nil
}
