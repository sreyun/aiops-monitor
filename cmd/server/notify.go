package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/template"
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
	// SRE hooks (set during server wiring; nil-safe).
	incidents   *incidentManager
	remediation *remediationManager
}

func NewNotifier(store *Store, cfg *ConfigStore) *Notifier {
	return &Notifier{
		store:  store,
		cfg:    cfg,
		httpc:  newGuardedHTTPClient(8 * time.Second), // SSRF：飞书/钉钉 webhook 用户可配，拦元数据/链路本地
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

// ActiveAlerts returns a copy of the alerts currently firing (for AI inspection).
func (n *Notifier) ActiveAlerts() []Alert {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make([]Alert, 0, len(n.active))
	for _, a := range n.active {
		out = append(out, a)
	}
	return out
}

// ActiveSince returns a copy of the first-fired times keyed by alertKey,
// letting the alerts API show "elapsed X minutes".
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
		// SRE closed loop: open/resolve an incident, and on a firing alert run any
		// matching auto-remediation rule (scoped to the affected host).
		if n.incidents != nil {
			incID := n.incidents.OnAlertTransition(t.a, alertKey(t.a), t.firing)
			if t.firing && incID != 0 && n.remediation != nil {
				n.remediation.OnAlert(t.a, incID)
			}
		}
	}
}

func (n *Notifier) dispatch(cfg ServerConfig, a Alert, firing bool) {
	// activity log: the machine-detected threshold transition (intervention)
	verb, tlvl := Tz("notify.alert_fired"), a.Level
	if !firing {
		verb, tlvl = Tz("notify.alert_recovered"), "info"
	}
	n.store.AddLog(LogEntry{Kind: KindSystem, Level: tlvl, Actor: Tz("notify.alert_engine"), Host: a.Hostname, Message: verb + "：" + a.Message})
	n.pushChannels(cfg, a, firing)
}

// pushChannels sends the alert text to every enabled bot channel and logs the
// push result. Shared by threshold alerts and custom-check alerts.
func (n *Notifier) pushChannels(cfg ServerConfig, a Alert, firing bool) {
	// 告警治理：仅对「触发」通知做静默/抑制；「恢复」通知一律照发，避免规则造成"永远告警"错觉。
	if firing {
		if ok, rule := govSilenced(cfg.Governance, a, time.Now()); ok {
			n.store.AddLog(LogEntry{Kind: KindSystem, Level: "info", Actor: Tz("notify.notification"), Host: a.Hostname, Message: "静默规则「" + rule + "」已抑制通知：" + a.Message})
			return
		}
		if ok, rule := govInhibited(cfg.Governance, a, n.ActiveAlerts()); ok {
			n.store.AddLog(LogEntry{Kind: KindSystem, Level: "info", Actor: Tz("notify.notification"), Host: a.Hostname, Message: "抑制规则「" + rule + "」已抑制通知：" + a.Message})
			return
		}
	}
	// 通知路由：命中路由则仅发其指定渠道；无任何路由命中=默认全部启用渠道（向后兼容）。
	routeSel, routed := govRouteChannels(cfg.Governance, a)
	send := func(ch string) bool { return !routed || routeSel[ch] }

	text := formatAlert(a, firing)
	var sent []string
	if send("feishu") && cfg.Feishu.Enabled && cfg.Feishu.Webhook != "" {
		if err := n.sendFeishu(cfg.Feishu, text); err != nil {
			slog.Error(Tz("notify.feishu_failed"), "err", err)
			n.store.AddLog(LogEntry{Kind: KindSystem, Level: "warning", Actor: Tz("notify.notification"), Host: a.Hostname, Message: Tz("log.feishu_failed", err.Error())})
		} else {
			sent = append(sent, Tz("notify.feishu"))
		}
	}
	if send("dingtalk") && cfg.Dingtalk.Enabled && cfg.Dingtalk.Webhook != "" {
		if err := n.sendDingtalk(cfg.Dingtalk, text); err != nil {
			slog.Error(Tz("notify.dingtalk_failed"), "err", err)
			n.store.AddLog(LogEntry{Kind: KindSystem, Level: "warning", Actor: Tz("notify.notification"), Host: a.Hostname, Message: Tz("log.dingtalk_failed", err.Error())})
		} else {
			sent = append(sent, Tz("notify.dingtalk"))
		}
	}
	// Email alert notification — sent to the operator's bound email if SMTP is configured
	if send("email") && cfg.SMTP.Enabled && cfg.SMTP.Host != "" {
		html := alertEmailHTML(a, firing)
		okAny := false
		for _, to := range n.cfg.AlertEmails() {
			if err := sendEmail(cfg.SMTP, to, Tz("notify.alert_subject", a.Hostname), html); err != nil {
				slog.Error(Tz("notify.email_failed"), "err", err)
				n.store.AddLog(LogEntry{Kind: KindSystem, Level: "warning", Actor: Tz("notify.notification"), Host: a.Hostname, Message: Tz("log.email_failed", err.Error())})
			} else {
				okAny = true
			}
		}
		if okAny {
			sent = append(sent, Tz("notify.email"))
		}
	}
	// Custom webhook
	if send("webhook") && cfg.CustomWebhook.Enabled && cfg.CustomWebhook.URL != "" {
		if err := sendCustomWebhook(cfg.CustomWebhook, text, a, firing); err != nil {
			slog.Error(Tz("notify.custom_webhook_failed"), "err", err)
			n.store.AddLog(LogEntry{Kind: KindSystem, Level: "warning", Actor: Tz("notify.notification"), Host: a.Hostname, Message: Tz("log.custom_webhook_failed", err.Error())})
		} else {
			sent = append(sent, Tz("notify.custom_webhook"))
		}
	}
	// SMS notification
	if send("sms") && cfg.SMS.Enabled && cfg.SMS.AccessKey != "" {
		if err := n.sendSMS(cfg.SMS, text); err != nil {
			slog.Error("sms send failed", "err", err)
			n.store.AddLog(LogEntry{Kind: KindSystem, Level: "warning", Actor: Tz("notify.notification"), Host: a.Hostname, Message: "短信发送失败: " + err.Error()})
		} else {
			sent = append(sent, "短信")
		}
	}
	// Voice call notification
	if send("voicecall") && cfg.VoiceCall.Enabled && cfg.VoiceCall.AccessKey != "" {
		if err := n.sendVoiceCall(cfg.VoiceCall, text); err != nil {
			slog.Error("voice call send failed", "err", err)
			n.store.AddLog(LogEntry{Kind: KindSystem, Level: "warning", Actor: Tz("notify.notification"), Host: a.Hostname, Message: "电话通知失败: " + err.Error()})
		} else {
			sent = append(sent, "电话")
		}
	}
	if len(sent) > 0 {
		n.store.AddLog(LogEntry{Kind: KindSystem, Level: "info", Actor: Tz("notify.notification"), Host: a.Hostname, Message: Tz("log.pushed", strings.Join(sent, "/"), a.Message)})
	}
}

func formatAlert(a Alert, firing bool) string {
	status := Tz("notify.fire")
	if !firing {
		status = Tz("notify.recover")
	}
	lv := Tz("notify.warn")
	if a.Level == "critical" {
		lv = Tz("notify.critical")
	}
	typeMap := map[string]string{
		"cpu": Tz("notify.type_cpu"), "memory": Tz("notify.type_memory"), "disk": Tz("notify.type_disk"), "offline": Tz("notify.type_offline"),
		"load": Tz("notify.type_load"), "gpu": Tz("notify.type_gpu"), "check": Tz("notify.type_check"),
		"api": Tz("notify.type_api"), "task": Tz("notify.type_task"),
	}
	typeLabel := typeMap[a.Type]
	if typeLabel == "" {
		typeLabel = a.Type
	}
	ipLine := ""
	if a.IP != "" {
		ipLine = fmt.Sprintf("\n%s: %s", Tz("notify.ip"), a.IP)
	}
	return fmt.Sprintf("%s\n%s: %s%s\n%s: %s\n%s: %s\n%s: %s\n%s: %s",
		Tz("notify.title", status), Tz("notify.host"), a.Hostname, ipLine,
		Tz("notify.level"), lv, Tz("notify.type"), typeLabel,
		Tz("notify.detail"), a.Message, Tz("notify.time"), time.Unix(a.Timestamp, 0).Format("2006-01-02 15:04:05"))
}

// SendTest pushes a one-off test message on the enabled channels of the given
// config and returns human-readable errors (empty on full success).
func (n *Notifier) SendTest(cfg ServerConfig) []string {
	msg := Tz("notify.test_msg", time.Now().Format("2006-01-02 15:04:05"))
	var errs []string
	if cfg.Feishu.Enabled && cfg.Feishu.Webhook != "" {
		if err := n.sendFeishu(cfg.Feishu, msg); err != nil {
			errs = append(errs, Tz("notify.feishu")+": "+err.Error())
		}
	}
	if cfg.Dingtalk.Enabled && cfg.Dingtalk.Webhook != "" {
		if err := n.sendDingtalk(cfg.Dingtalk, msg); err != nil {
			errs = append(errs, Tz("notify.dingtalk")+": "+err.Error())
		}
	}
	if cfg.SMTP.Enabled && cfg.SMTP.Host != "" {
		emails := n.cfg.AlertEmails()
		if len(emails) == 0 {
			errs = append(errs, Tz("notify.email")+": "+Tz("notify.no_email"))
		} else {
			html := `<div style="font-family:sans-serif;padding:20px"><h2>AIOps Monitor</h2><p>` + Tz("notify.test_email_body") + `</p><p>` + Tz("notify.time") + ": " + time.Now().Format("2006-01-02 15:04:05") + `</p></div>`
			for _, to := range emails {
				if err := sendEmail(cfg.SMTP, to, Tz("notify.test_email_subject"), html); err != nil {
					errs = append(errs, Tz("notify.email")+": "+err.Error())
					break
				}
			}
		}
	}
	if cfg.CustomWebhook.Enabled && cfg.CustomWebhook.URL != "" {
		if err := sendCustomWebhook(cfg.CustomWebhook, msg, Alert{}, false); err != nil {
			errs = append(errs, Tz("notify.custom_webhook")+": "+err.Error())
		}
	}
	if cfg.SMS.Enabled && cfg.SMS.AccessKey != "" {
		if err := n.sendSMS(cfg.SMS, msg); err != nil {
			errs = append(errs, "短信: "+err.Error())
		}
	}
	if cfg.VoiceCall.Enabled && cfg.VoiceCall.AccessKey != "" {
		if err := n.sendVoiceCall(cfg.VoiceCall, msg); err != nil {
			errs = append(errs, "电话: "+err.Error())
		}
	}
	if !cfg.Feishu.Enabled && !cfg.Dingtalk.Enabled && !cfg.SMTP.Enabled && !cfg.CustomWebhook.Enabled && !cfg.SMS.Enabled && !cfg.VoiceCall.Enabled {
		errs = append(errs, Tz("notify.no_channel"))
	}
	return errs
}

// alertEmailHTML renders an alert notification as an HTML email body.
func alertEmailHTML(a Alert, firing bool) string {
	status := Tz("notify.email_alert_fired")
	headColor := "#e74c3c"
	lvlColor := "#f39c12"
	if a.Level == "critical" {
		lvlColor = "#e74c3c"
	}
	if !firing {
		status = Tz("notify.email_alert_recovered")
		headColor = "#27ae60"
		lvlColor = "#27ae60"
	}
	lv := Tz("notify.warn")
	if a.Level == "critical" {
		lv = Tz("notify.critical")
	}
	typeMap := map[string]string{
		"cpu": Tz("notify.type_cpu"), "memory": Tz("notify.type_memory"), "disk": Tz("notify.type_disk"), "offline": Tz("notify.type_offline"),
		"load": Tz("notify.type_load"), "gpu": Tz("notify.type_gpu"), "check": Tz("notify.type_check"),
		"api": Tz("notify.type_api"), "task": Tz("notify.type_task"),
	}
	typeLabel := typeMap[a.Type]
	if typeLabel == "" {
		typeLabel = a.Type
	}
	ipLine := ""
	if a.IP != "" {
		ipLine = `<tr><td style="color:#888;padding:4px 0">` + Tz("notify.ip") + `</td><td style="padding:4px 0">` + a.IP + `</td></tr>`
	}
	return fmt.Sprintf(`<div style="font-family:sans-serif;max-width:600px;margin:0 auto;padding:20px">
  <h2 style="color:%s">%s</h2>
  <table style="width:100%%;border-collapse:collapse">
    <tr><td style="color:#888;padding:4px 0;width:80px">%s</td><td style="padding:4px 0;font-weight:bold">%s</td></tr>
    %s
    <tr><td style="color:#888;padding:4px 0">%s</td><td style="padding:4px 0;color:%s">%s</td></tr>
    <tr><td style="color:#888;padding:4px 0">%s</td><td style="padding:4px 0">%s</td></tr>
    <tr><td style="color:#888;padding:4px 0">%s</td><td style="padding:4px 0">%s</td></tr>
    <tr><td style="color:#888;padding:4px 0">%s</td><td style="padding:4px 0">%s</td></tr>
  </table>
</div>`,
		headColor, status, Tz("notify.host"), a.Hostname, ipLine,
		Tz("notify.level"), lvlColor, lv,
		Tz("notify.type"), typeLabel,
		Tz("notify.detail"), a.Message,
		Tz("notify.time"), time.Unix(a.Timestamp, 0).Format("2006-01-02 15:04:05"))
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
		return fmt.Errorf("API returned code=%d %s", code, msg)
	}
	return nil
}

// ----- cloud SMS / voice notification helpers -----

// aliyunSign builds the Alibaba Cloud API V1 signature (HMAC-SHA1).
func aliyunSign(method string, params map[string]string, secret string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var qs []string
	for _, k := range keys {
		qs = append(qs, url.QueryEscape(k)+"="+url.QueryEscape(params[k]))
	}
	canonical := strings.Join(qs, "&")
	stringToSign := method + "&" + url.QueryEscape("/") + "&" + url.QueryEscape(canonical)
	h := hmac.New(sha1.New, []byte(secret+"&"))
	h.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// sendSMS sends an alert via cloud SMS API (Aliyun / Huawei / Tencent).
func (n *Notifier) sendSMS(cfg SMSConfig, text string) error {
	if cfg.Provider == "" || cfg.Provider == "aliyun" {
		return n.sendAliyunSMS(cfg, text)
	}
	// Huawei Cloud and Tencent Cloud SMS: use simple HTTP POST (placeholder for future full signing)
	return fmt.Errorf("SMS provider %s not yet implemented", cfg.Provider)
}

func (n *Notifier) sendAliyunSMS(cfg SMSConfig, text string) error {
	phones := strings.Join(cfg.Phones, ",")
	if phones == "" {
		return fmt.Errorf("no phone numbers configured")
	}
	ts := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	nonce := strconv.FormatInt(time.Now().UnixNano(), 10)[:16]
	params := map[string]string{
		"AccessKeyId":      cfg.AccessKey,
		"Action":           "SendSms",
		"Format":           "JSON",
		"PhoneNumbers":     phones,
		"SignName":         cfg.SignName,
		"TemplateCode":     cfg.TemplateCode,
		"TemplateParam":    fmt.Sprintf(`{"message":"%s"}`, strings.ReplaceAll(text, `"`, `\"`)),
		"SignatureMethod":  "HMAC-SHA1",
		"SignatureNonce":   nonce,
		"SignatureVersion": "1.0",
		"Timestamp":        ts,
		"Version":          "2017-05-25",
	}
	params["Signature"] = aliyunSign("GET", params, cfg.SecretKey)

	var qs []string
	for k, v := range params {
		qs = append(qs, url.QueryEscape(k)+"="+url.QueryEscape(v))
	}
	reqURL := "https://dysmsapi.aliyuncs.com/?" + strings.Join(qs, "&")
	resp, err := n.httpc.Get(reqURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var r struct {
		Code    string `json:"Code"`
		Message string `json:"Message"`
	}
	if json.Unmarshal(rb, &r) == nil && r.Code != "OK" {
		return fmt.Errorf("SMS API: %s (%s)", r.Message, r.Code)
	}
	return nil
}

// sendVoiceCall sends an alert via cloud voice call (TTS) API.
func (n *Notifier) sendVoiceCall(cfg VoiceCallConfig, text string) error {
	if cfg.Provider == "" || cfg.Provider == "aliyun" {
		return n.sendAliyunVoiceCall(cfg, text)
	}
	return fmt.Errorf("voice call provider %s not yet implemented", cfg.Provider)
}

func (n *Notifier) sendAliyunVoiceCall(cfg VoiceCallConfig, text string) error {
	phones := strings.Join(cfg.CalledNumbers, ",")
	if phones == "" {
		return fmt.Errorf("no called numbers configured")
	}
	// Build TTS params: merge template param with the alert message
	tsParam := cfg.TTSParam
	if tsParam == "" {
		tsParam = fmt.Sprintf(`{"message":"%s"}`, strings.ReplaceAll(text, `"`, `\"`))
	}
	calledNumber := cfg.CalledNumbers[0] // SingleCallByTts only supports one callee per call
	ts := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	nonce := strconv.FormatInt(time.Now().UnixNano(), 10)[:16]
	params := map[string]string{
		"AccessKeyId":      cfg.AccessKey,
		"Action":           "SingleCallByTts",
		"Format":           "JSON",
		"CalledNumber":     calledNumber,
		"TtsCode":          cfg.TTSCode,
		"TtsParam":         tsParam,
		"SignatureMethod":  "HMAC-SHA1",
		"SignatureNonce":   nonce,
		"SignatureVersion": "1.0",
		"Timestamp":        ts,
		"Version":          "2017-05-25",
	}
	params["Signature"] = aliyunSign("GET", params, cfg.SecretKey)

	var qs []string
	for k, v := range params {
		qs = append(qs, url.QueryEscape(k)+"="+url.QueryEscape(v))
	}
	reqURL := "https://dyvmsapi.aliyuncs.com/?" + strings.Join(qs, "&")
	resp, err := n.httpc.Get(reqURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var r struct {
		Code    string `json:"Code"`
		Message string `json:"Message"`
	}
	if json.Unmarshal(rb, &r) == nil && r.Code != "OK" {
		return fmt.Errorf("VoiceCall API: %s (%s)", r.Message, r.Code)
	}
	return nil
}

// sendCustomWebhook sends an alert to a user-defined HTTP(S) endpoint.
func sendCustomWebhook(cfg CustomWebhookConfig, text string, a Alert, firing bool) error {
	method := cfg.Method
	if method == "" {
		method = "POST"
	}
	contentType := cfg.ContentType
	if contentType == "" {
		contentType = "application/json"
	}

	// Build body: use template if provided, otherwise default JSON
	var body []byte
	if cfg.BodyTemplate != "" {
		tmpl, err := template.New("webhook").Parse(cfg.BodyTemplate)
		if err != nil {
			return fmt.Errorf("template parse error: %w", err)
		}
		data := map[string]any{
			"Level":     a.Level,
			"Type":      a.Type,
			"Hostname":  a.Hostname,
			"HostID":    a.HostID,
			"IP":        a.IP,
			"Message":   a.Message,
			"Value":     a.Value,
			"Timestamp": a.Timestamp,
			"Firing":    firing,
			"Text":      text,
		}
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, data); err != nil {
			return fmt.Errorf("template exec error: %w", err)
		}
		body = buf.Bytes()
	} else {
		body, _ = json.Marshal(map[string]any{
			"text":      text,
			"level":     a.Level,
			"type":      a.Type,
			"hostname":  a.Hostname,
			"message":   a.Message,
			"value":     a.Value,
			"timestamp": a.Timestamp,
			"firing":    firing,
		})
	}

	var req *http.Request
	var err error
	if method == "GET" {
		req, err = http.NewRequest("GET", cfg.URL, nil)
	} else {
		req, err = http.NewRequest(method, cfg.URL, bytes.NewReader(body))
	}
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)

	// Parse optional custom headers (JSON key-value)
	if cfg.Headers != "" {
		var hdrs map[string]string
		if json.Unmarshal([]byte(cfg.Headers), &hdrs) == nil {
			for k, v := range hdrs {
				req.Header.Set(k, v)
			}
		}
	}

	client := newGuardedHTTPClient(8 * time.Second) // SSRF：自定义 webhook URL 用户可配，拦元数据/链路本地
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(rb)))
	}
	return nil
}
