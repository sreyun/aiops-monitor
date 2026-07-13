package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// WebhookConfig holds one bot channel (Feishu or DingTalk).
type WebhookConfig struct {
	Enabled bool   `json:"enabled"`
	Webhook string `json:"webhook"`
	Secret  string `json:"secret,omitempty"` // DingTalk optional HMAC-SHA256 sign secret
}

// SMTPConfig holds the email (SMTP) notification channel. The password is
// stored in plaintext on disk (like the DingTalk secret) but masked when
// echoed to the browser via handleGetConfig.
type SMTPConfig struct {
	Enabled  bool   `json:"smtp_enabled"`
	Host     string `json:"smtp_host"`     // e.g. smtp.gmail.com
	Port     int    `json:"smtp_port"`     // 465 (implicit TLS) or 587 (STARTTLS)
	Username string `json:"smtp_username"` // sender email account
	Password string `json:"smtp_password,omitempty"`
	FromName string `json:"smtp_from_name"` // display name, default "AIOps Monitor"
	UseTLS   bool   `json:"smtp_use_tls"`   // true = implicit TLS (465), false = STARTTLS/plain
}

// CustomWebhookConfig holds a user-defined generic HTTP(S) webhook channel.
// The body template supports placeholders like {{.Level}}, {{.Message}}, etc.
type CustomWebhookConfig struct {
	Enabled      bool   `json:"enabled"`
	URL          string `json:"url"`
	Method       string `json:"method"`        // POST (default) | GET
	ContentType  string `json:"content_type"`  // application/json (default) | text/plain
	Headers      string `json:"headers"`       // optional JSON key-value map, e.g. {"X-Token":"abc"}
	BodyTemplate string `json:"body_template"` // Go template; empty = default Markdown-like text
}

// SMSConfig holds the cloud SMS notification channel configuration.
type SMSConfig struct {
	Enabled       bool     `json:"enabled"`
	Provider      string   `json:"provider"` // aliyun | huawei | tencent
	AccessKey     string   `json:"access_key"`
	SecretKey     string   `json:"secret_key,omitempty"`
	AppID         string   `json:"app_id,omitempty"` // 华为云=project_id; 腾讯云=SmsSdkAppId
	SignName      string   `json:"sign_name"`
	TemplateCode  string   `json:"template_code"`
	TemplateParam string   `json:"template_param,omitempty"` // 自定义模板参数 JSON，如 {"code":"${code}"}；空时默认 {"message":"..."}
	Phones        []string `json:"phones"`
}

// VoiceCallConfig holds the cloud voice call (TTS) notification channel configuration.
type VoiceCallConfig struct {
	Enabled       bool     `json:"enabled"`
	Provider      string   `json:"provider"` // aliyun | huawei | tencent
	AccessKey     string   `json:"access_key"`
	SecretKey     string   `json:"secret_key,omitempty"`
	AppID         string   `json:"app_id,omitempty"` // 华为云=project_id; 腾讯云=VoiceSdkAppId
	CalledNumbers []string `json:"called_numbers"`
	TTSCode       string   `json:"tts_code"`
	TTSParam      string   `json:"tts_param"`
}

// ThresholdConfig is the JSON-friendly, operator-editable alert threshold set.
type ThresholdConfig struct {
	CPUWarn         float64 `json:"cpu_warn"`
	CPUCrit         float64 `json:"cpu_crit"`
	MemWarn         float64 `json:"mem_warn"`
	MemCrit         float64 `json:"mem_crit"`
	DiskWarn        float64 `json:"disk_warn"`
	DiskCrit        float64 `json:"disk_crit"`
	DiskIOWarn      float64 `json:"diskio_warn"`
	DiskIOCrit      float64 `json:"diskio_crit"`
	IOPSWarn        float64 `json:"iops_warn"`
	IOPSCrit        float64 `json:"iops_crit"`
	GPUWarn         float64 `json:"gpu_warn"`
	GPUCrit         float64 `json:"gpu_crit"`
	GPUTempWarn     float64 `json:"gpu_temp_warn"` // GPU 温度 警告 °C
	GPUTempCrit     float64 `json:"gpu_temp_crit"` // GPU 温度 严重 °C
	GPUMemWarn      float64 `json:"gpu_mem_warn"`  // GPU 显存占用 警告 %
	GPUMemCrit      float64 `json:"gpu_mem_crit"`  // GPU 显存占用 严重 %
	LoadWarn        float64 `json:"load_warn"`     // 按 CPU 核心数倍率，如 2.0 = 核心数×2
	LoadCrit        float64 `json:"load_crit"`
	ProcWarn        float64 `json:"proc_warn"` // 进程数突增/突降比例（如 0.5 = 50%）
	ConnWarn        int     `json:"conn_warn"` // 主机连接数（TCP+UDP 总数）警告
	ConnCrit        int     `json:"conn_crit"` // 主机连接数（TCP+UDP 总数）严重
	OfflineAfterSec int     `json:"offline_after_sec"`
	// ---- 拨测监控阈值（Ping / TCP / HTTP / 进程）----
	CheckPingLossWarn    float64 `json:"check_ping_loss_warn"`    // Ping 丢包率 警告 %
	CheckPingLossCrit    float64 `json:"check_ping_loss_crit"`    // Ping 丢包率 严重 %
	CheckPingLatencyWarn float64 `json:"check_ping_latency_warn"` // Ping 平均延迟 警告 ms
	CheckPingLatencyCrit float64 `json:"check_ping_latency_crit"` // Ping 平均延迟 严重 ms
	CheckTCPTimeoutWarn  float64 `json:"check_tcp_timeout_warn"`  // TCP 连接超时 警告 ms
	CheckTCPTimeoutCrit  float64 `json:"check_tcp_timeout_crit"`  // TCP 连接超时 严重 ms
	CheckHTTPRespWarn    float64 `json:"check_http_resp_warn"`    // HTTP 响应时间 警告 ms
	CheckHTTPRespCrit    float64 `json:"check_http_resp_crit"`    // HTTP 响应时间 严重 ms
	CheckHTTPStatusWarn  int     `json:"check_http_status_warn"`  // HTTP 非 2xx 次数 警告
	CheckHTTPStatusCrit  int     `json:"check_http_status_crit"`  // HTTP 非 2xx 次数 严重
	CheckProcFailWarn    int     `json:"check_proc_fail_warn"`    // 进程存活失败次数 警告
	CheckProcFailCrit    int     `json:"check_proc_fail_crit"`    // 进程存活失败次数 严重
	// ---- API 业务监控阈值 ----
	APIAvailWarn      float64 `json:"api_avail_warn"`      // 接口可用率 警告 %（低于此值告警）
	APIAvailCrit      float64 `json:"api_avail_crit"`      // 接口可用率 严重 %
	APIAvgRespWarn    float64 `json:"api_avg_resp_warn"`   // 平均响应时间 警告 ms
	APIAvgRespCrit    float64 `json:"api_avg_resp_crit"`   // 平均响应时间 严重 ms
	APIP95RespWarn    float64 `json:"api_p95_resp_warn"`   // P95 响应时间 警告 ms
	APIP95RespCrit    float64 `json:"api_p95_resp_crit"`   // P95 响应时间 严重 ms
	APIThroughputWarn float64 `json:"api_throughput_warn"` // 吞吐量 警告 req/s（低于此值告警）
	APIThroughputCrit float64 `json:"api_throughput_crit"` // 吞吐量 严重 req/s
	// ---- 编排定时任务阈值 ----
	TaskFailWarn    int     `json:"task_fail_warn"`    // 执行失败次数 警告
	TaskFailCrit    int     `json:"task_fail_crit"`    // 执行失败次数 严重
	TaskTimeoutWarn float64 `json:"task_timeout_warn"` // 超时时长 警告 s
	TaskTimeoutCrit float64 `json:"task_timeout_crit"` // 超时时长 严重 s
	// ---- 端口转发监控阈值 ----
	ForwardConnWarn int     `json:"forward_conn_warn"` // 活跃连接数 警告
	ForwardConnCrit int     `json:"forward_conn_crit"` // 活跃连接数 严重
	ForwardBwWarn   float64 `json:"forward_bw_warn"`   // 带宽使用率 警告 %
	ForwardBwCrit   float64 `json:"forward_bw_crit"`   // 带宽使用率 严重 %
	ForwardErrWarn  float64 `json:"forward_err_warn"`  // 错误率 警告 %
	ForwardErrCrit  float64 `json:"forward_err_crit"`  // 错误率 严重 %
	ForwardLatWarn  float64 `json:"forward_lat_warn"`  // 平均延迟 警告 ms
	ForwardLatCrit  float64 `json:"forward_lat_crit"`  // 平均延迟 严重 ms
}

func defaultThresholdConfig() ThresholdConfig {
	return ThresholdConfig{
		CPUWarn: 80, CPUCrit: 95,
		MemWarn: 85, MemCrit: 95,
		DiskWarn: 80, DiskCrit: 90,
		DiskIOWarn: 80, DiskIOCrit: 95,
		IOPSWarn: 50000, IOPSCrit: 100000,
		GPUWarn: 80, GPUCrit: 95,
		GPUTempWarn: 85, GPUTempCrit: 95,
		GPUMemWarn: 90, GPUMemCrit: 97,
		LoadWarn: 4.0, LoadCrit: 8.0,
		ProcWarn: 0.5,
		ConnWarn: 5000, ConnCrit: 10000,
		OfflineAfterSec: 60,
		// 拨测监控默认阈值
		CheckPingLossWarn: 10, CheckPingLossCrit: 30,
		CheckPingLatencyWarn: 100, CheckPingLatencyCrit: 500,
		CheckTCPTimeoutWarn: 1000, CheckTCPTimeoutCrit: 5000,
		CheckHTTPRespWarn: 1000, CheckHTTPRespCrit: 5000,
		CheckHTTPStatusWarn: 1, CheckHTTPStatusCrit: 5,
		CheckProcFailWarn: 1, CheckProcFailCrit: 3,
		// API 业务监控默认阈值
		APIAvailWarn: 99.0, APIAvailCrit: 95.0,
		APIAvgRespWarn: 500, APIAvgRespCrit: 2000,
		APIP95RespWarn: 1000, APIP95RespCrit: 5000,
		APIThroughputWarn: 100, APIThroughputCrit: 10,
		// 编排定时任务默认阈值
		TaskFailWarn: 1, TaskFailCrit: 5,
		TaskTimeoutWarn: 60, TaskTimeoutCrit: 300,
		// 端口转发监控默认阈值
		ForwardConnWarn: 200, ForwardConnCrit: 280,
		ForwardBwWarn: 80, ForwardBwCrit: 95,
		ForwardErrWarn: 5, ForwardErrCrit: 15,
		ForwardLatWarn: 1000, ForwardLatCrit: 5000,
	}
}

// backfillThresholdDefaults replaces any zero (i.e. unset) threshold field with
// its standard default. A zero threshold is never meaningful — the alert engine
// fires when metric >= threshold, so a 0 would alert constantly — so a 0 is
// treated as "not configured" and healed from defaultThresholdConfig(). This
// makes every metric fall back to a sane standard threshold even for configs
// saved before a field existed or saved with a blank form input. Returns true if
// any field changed.
func backfillThresholdDefaults(t *ThresholdConfig) bool {
	d := defaultThresholdConfig()
	changed := false
	fix := func(p *float64, def float64) {
		if *p == 0 {
			*p = def
			changed = true
		}
	}
	fix(&t.CPUWarn, d.CPUWarn)
	fix(&t.CPUCrit, d.CPUCrit)
	fix(&t.MemWarn, d.MemWarn)
	fix(&t.MemCrit, d.MemCrit)
	fix(&t.DiskWarn, d.DiskWarn)
	fix(&t.DiskCrit, d.DiskCrit)
	fix(&t.DiskIOWarn, d.DiskIOWarn)
	fix(&t.DiskIOCrit, d.DiskIOCrit)
	fix(&t.IOPSWarn, d.IOPSWarn)
	fix(&t.IOPSCrit, d.IOPSCrit)
	fix(&t.GPUWarn, d.GPUWarn)
	fix(&t.GPUCrit, d.GPUCrit)
	fix(&t.GPUTempWarn, d.GPUTempWarn)
	fix(&t.GPUTempCrit, d.GPUTempCrit)
	fix(&t.GPUMemWarn, d.GPUMemWarn)
	fix(&t.GPUMemCrit, d.GPUMemCrit)
	fix(&t.LoadWarn, d.LoadWarn)
	fix(&t.LoadCrit, d.LoadCrit)
	fix(&t.ProcWarn, d.ProcWarn)
	if t.ConnWarn == 0 {
		t.ConnWarn = d.ConnWarn
		changed = true
	}
	if t.ConnCrit == 0 {
		t.ConnCrit = d.ConnCrit
		changed = true
	}
	fix(&t.CheckPingLossWarn, d.CheckPingLossWarn)
	fix(&t.CheckPingLossCrit, d.CheckPingLossCrit)
	fix(&t.CheckPingLatencyWarn, d.CheckPingLatencyWarn)
	fix(&t.CheckPingLatencyCrit, d.CheckPingLatencyCrit)
	fix(&t.CheckTCPTimeoutWarn, d.CheckTCPTimeoutWarn)
	fix(&t.CheckTCPTimeoutCrit, d.CheckTCPTimeoutCrit)
	fix(&t.CheckHTTPRespWarn, d.CheckHTTPRespWarn)
	fix(&t.CheckHTTPRespCrit, d.CheckHTTPRespCrit)
	if t.CheckHTTPStatusWarn == 0 {
		t.CheckHTTPStatusWarn = d.CheckHTTPStatusWarn
		changed = true
	}
	if t.CheckHTTPStatusCrit == 0 {
		t.CheckHTTPStatusCrit = d.CheckHTTPStatusCrit
		changed = true
	}
	if t.CheckProcFailWarn == 0 {
		t.CheckProcFailWarn = d.CheckProcFailWarn
		changed = true
	}
	if t.CheckProcFailCrit == 0 {
		t.CheckProcFailCrit = d.CheckProcFailCrit
		changed = true
	}
	fix(&t.APIAvailWarn, d.APIAvailWarn)
	fix(&t.APIAvailCrit, d.APIAvailCrit)
	fix(&t.APIAvgRespWarn, d.APIAvgRespWarn)
	fix(&t.APIAvgRespCrit, d.APIAvgRespCrit)
	fix(&t.APIP95RespWarn, d.APIP95RespWarn)
	fix(&t.APIP95RespCrit, d.APIP95RespCrit)
	fix(&t.APIThroughputWarn, d.APIThroughputWarn)
	fix(&t.APIThroughputCrit, d.APIThroughputCrit)
	if t.TaskFailWarn == 0 {
		t.TaskFailWarn = d.TaskFailWarn
		changed = true
	}
	if t.TaskFailCrit == 0 {
		t.TaskFailCrit = d.TaskFailCrit
		changed = true
	}
	fix(&t.TaskTimeoutWarn, d.TaskTimeoutWarn)
	fix(&t.TaskTimeoutCrit, d.TaskTimeoutCrit)
	if t.ForwardConnWarn == 0 {
		t.ForwardConnWarn = d.ForwardConnWarn
		changed = true
	}
	if t.ForwardConnCrit == 0 {
		t.ForwardConnCrit = d.ForwardConnCrit
		changed = true
	}
	fix(&t.ForwardBwWarn, d.ForwardBwWarn)
	fix(&t.ForwardBwCrit, d.ForwardBwCrit)
	fix(&t.ForwardErrWarn, d.ForwardErrWarn)
	fix(&t.ForwardErrCrit, d.ForwardErrCrit)
	fix(&t.ForwardLatWarn, d.ForwardLatWarn)
	fix(&t.ForwardLatCrit, d.ForwardLatCrit)
	if t.OfflineAfterSec == 0 {
		t.OfflineAfterSec = d.OfflineAfterSec
		changed = true
	}
	return changed
}

func (t ThresholdConfig) toThresholds() Thresholds {
	return Thresholds{
		CPUWarn: t.CPUWarn, CPUCrit: t.CPUCrit,
		MemWarn: t.MemWarn, MemCrit: t.MemCrit,
		DiskWarn: t.DiskWarn, DiskCrit: t.DiskCrit,
		DiskIOWarn: t.DiskIOWarn, DiskIOCrit: t.DiskIOCrit,
		IOPSWarn: t.IOPSWarn, IOPSCrit: t.IOPSCrit,
		GPUWarn: t.GPUWarn, GPUCrit: t.GPUCrit,
		GPUTempWarn: t.GPUTempWarn, GPUTempCrit: t.GPUTempCrit,
		GPUMemWarn: t.GPUMemWarn, GPUMemCrit: t.GPUMemCrit,
		LoadWarn: t.LoadWarn, LoadCrit: t.LoadCrit,
		ProcWarn: t.ProcWarn,
		ConnWarn: float64(t.ConnWarn), ConnCrit: float64(t.ConnCrit),
		OfflineAfter: time.Duration(t.OfflineAfterSec) * time.Second,
		// 拨测监控
		CheckPingLossWarn: t.CheckPingLossWarn, CheckPingLossCrit: t.CheckPingLossCrit,
		CheckPingLatencyWarn: t.CheckPingLatencyWarn, CheckPingLatencyCrit: t.CheckPingLatencyCrit,
		CheckTCPTimeoutWarn: t.CheckTCPTimeoutWarn, CheckTCPTimeoutCrit: t.CheckTCPTimeoutCrit,
		CheckHTTPRespWarn: t.CheckHTTPRespWarn, CheckHTTPRespCrit: t.CheckHTTPRespCrit,
		CheckHTTPStatusWarn: t.CheckHTTPStatusWarn, CheckHTTPStatusCrit: t.CheckHTTPStatusCrit,
		CheckProcFailWarn: t.CheckProcFailWarn, CheckProcFailCrit: t.CheckProcFailCrit,
		// API 业务监控
		APIAvailWarn: t.APIAvailWarn, APIAvailCrit: t.APIAvailCrit,
		APIAvgRespWarn: t.APIAvgRespWarn, APIAvgRespCrit: t.APIAvgRespCrit,
		APIP95RespWarn: t.APIP95RespWarn, APIP95RespCrit: t.APIP95RespCrit,
		APIThroughputWarn: t.APIThroughputWarn, APIThroughputCrit: t.APIThroughputCrit,
		// 编排定时任务
		TaskFailWarn: t.TaskFailWarn, TaskFailCrit: t.TaskFailCrit,
		TaskTimeoutWarn: t.TaskTimeoutWarn, TaskTimeoutCrit: t.TaskTimeoutCrit,
		// 端口转发监控
		ForwardConnWarn: t.ForwardConnWarn, ForwardConnCrit: t.ForwardConnCrit,
		ForwardBwWarn: t.ForwardBwWarn, ForwardBwCrit: t.ForwardBwCrit,
		ForwardErrWarn: t.ForwardErrWarn, ForwardErrCrit: t.ForwardErrCrit,
		ForwardLatWarn: t.ForwardLatWarn, ForwardLatCrit: t.ForwardLatCrit,
	}
}

// AccountConfig is the dashboard login account + profile. The password is
// stored salted+hashed (never plaintext).
type AccountConfig struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Phone       string `json:"phone,omitempty"`
	Salt        string `json:"salt"`
	Hash        string `json:"hash"`
	// Optional TOTP (Google Authenticator) second factor. MFASecret is the base32
	// shared secret; it is never returned to the browser once enrollment completes.
	MFAEnabled bool   `json:"mfa_enabled"`
	MFASecret  string `json:"mfa_secret,omitempty"`
	// Role is the RBAC role: admin | operator | viewer.
	Role string `json:"role,omitempty"`
	// Terminal secondary password (v5.3.0): hashed+salted, never returned to browser.
	// Empty means the user has not set a terminal password yet.
	TerminalPasswordHash string `json:"terminal_password_hash,omitempty"`
	TerminalPasswordSalt string `json:"terminal_password_salt,omitempty"`
	// MustChangePassword forces the user to change their password on next login.
	// Set by the admin password reset tool (v5.4.0).
	MustChangePassword bool `json:"must_change_password,omitempty"`
}

func defaultAccount() AccountConfig {
	salt := genToken()[:16]
	return AccountConfig{
		Username:    "admin",
		DisplayName: Tz("user.default_display"),
		Salt:        salt,
		Hash:        hashPassword("admin", salt),
		Role:        RoleAdmin,
	}
}

// CustomCheck is an operator-defined synthetic monitor run by the server:
// an HTTP(S) URL probe, a TCP host:port probe, or a process-existence check.
// A failing check raises an alert and pushes a notification.
type CustomCheck struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`   // http | tcp | ping | process
	Target      string `json:"target"` // URL for http, host:port for tcp, hostID/procName for process
	IntervalSec int    `json:"interval_sec"`
	Level       string `json:"level"` // warning | critical
	Enabled     bool   `json:"enabled"`

	// HTTP 高级模式（仅 type=http，Advanced=true 时生效；均为可选，向后兼容）
	Advanced       bool              `json:"advanced,omitempty"`         // 启用高级检测
	Method         string            `json:"method,omitempty"`           // GET/POST/PUT/...（默认 GET）
	Headers        map[string]string `json:"headers,omitempty"`          // 自定义请求头（含 Authorization / X-API-Key 等静态鉴权）
	Body           string            `json:"body,omitempty"`             // 请求体（POST/PUT）
	ExpectStatus   int               `json:"expect_status,omitempty"`    // 期望状态码（0=默认 <400 即通过）
	ExpectKeyword  string            `json:"expect_keyword,omitempty"`   // 响应体应包含的关键字（或正则）
	KeywordIsRegex bool              `json:"keyword_is_regex,omitempty"` // 关键字是否按正则匹配
	JSONPath       string            `json:"json_path,omitempty"`        // JSON 断言的点路径，如 code / data.token
	JSONExpect     string            `json:"json_expect,omitempty"`      // JSON 断言期望值（字符串比较；留空=只要求路径存在）
	CertWarnDays   int               `json:"cert_warn_days,omitempty"`   // 证书剩余天数低于此值即判失败告警（0=不检测）
}

// HTTPProxyConfig is a saved HTTP proxy shortcut for quick access.
type HTTPProxyConfig struct {
	ID          string `json:"id"`           // Unique ID
	Name        string `json:"name"`         // Display name (e.g. "内部API服务")
	HostID      string `json:"host_id"`      // Target host ID
	Hostname    string `json:"hostname"`     // Target hostname (cached for display)
	TargetPort  int    `json:"target_port"`  // Target port
	DefaultPath string `json:"default_path"` // Default path prefix (e.g. "/api/v1")
	Operator    string `json:"operator"`     // Who created this
	CreatedAt   int64  `json:"created_at"`   // Creation timestamp
	Enabled     bool   `json:"enabled"`      // Whether this proxy is currently active
	IsCopy      bool   `json:"is_copy"`      // True for copies made via "duplicate"; the "(copy)" suffix is rendered at display time, not stored, so it localizes with the UI language.
}

// PersistedForwardRule is a serializable TCP forwarding rule stored in ServerConfig.
// The listener (net.Listener) is recreated on startup from the persisted fields.
type PersistedForwardRule struct {
	ID         string `json:"id"`
	HostID     string `json:"host_id"`
	Hostname   string `json:"hostname"`
	TargetPort int    `json:"target_port"`
	LocalPort  int    `json:"local_port"`
	ListenAddr string `json:"listen_addr"`
	Operator   string `json:"operator"`
	CreatedAt  int64  `json:"created_at"`
	Enabled    bool   `json:"enabled"`
	Protocol   string `json:"protocol,omitempty"` // "tcp"(默认/空) | "udp"
	GroupID    string `json:"group_id,omitempty"` // 端口范围批量组 ID（同组共享）
}

// ServerConfig is the operator-editable server configuration persisted to disk.
// Categories holds manual per-host category overrides (host id -> category).
type ServerConfig struct {
	AlertsEnabled bool                `json:"alerts_enabled"`
	Feishu        WebhookConfig       `json:"feishu"`
	Dingtalk      WebhookConfig       `json:"dingtalk"`
	CustomWebhook CustomWebhookConfig `json:"custom_webhook"`
	SMTP          SMTPConfig          `json:"smtp"`
	SMS           SMSConfig           `json:"sms"`
	VoiceCall     VoiceCallConfig     `json:"voice_call"`
	Thresholds    ThresholdConfig     `json:"thresholds"`
	Categories    map[string]string   `json:"categories"`
	InstallToken  string              `json:"install_token"`
	// PrevInstallToken + PrevTokenExpiresAt keep a rotated-out token valid during a
	// grace period, so existing agents don't drop offline the instant the token is
	// rotated. Managed by ResetToken (rotate).
	PrevInstallToken   string          `json:"prev_install_token,omitempty"`
	PrevTokenExpiresAt int64           `json:"prev_token_expires_at,omitempty"`
	RequireToken       bool            `json:"require_token"`
	Account            AccountConfig   `json:"account"`
	Checks             []CustomCheck   `json:"checks"`
	APISystems         []APISystem     `json:"api_systems,omitempty"` // API 性能监控：按业务系统分组的批量接口
	Governance         AlertGovernance `json:"governance,omitempty"`  // 告警治理：静默/抑制/生效时段/通知路由
	Playbooks          []Playbook      `json:"playbooks,omitempty"`
	// SRE workflow definitions (runtime state lives in the DB snapshot).
	RemediationRules []RemediationRule `json:"remediation_rules,omitempty"`
	SLOs             []SLO             `json:"slos,omitempty"`
	AI               AIConfig          `json:"ai,omitempty"`           // optional AI provider for inspection/diagnosis
	VM               VMConfig          `json:"vm,omitempty"`           // optional VictoriaMetrics writer (usually set via AIOPS_VM_URL)
	PostgresDSN      string            `json:"postgres_dsn,omitempty"` // optional PostgreSQL DSN (usually via AIOPS_POSTGRES_DSN)
	// TerminalDisabled is an inverted flag so remote terminal defaults ON for
	// existing configs (zero value = enabled); set true to globally disable it.
	TerminalDisabled bool `json:"terminal_disabled"`
	// ForwardDisabled is an inverted flag so port forwarding defaults ON for
	// existing configs (zero value = enabled); set true to globally disable it.
	ForwardDisabled bool `json:"forward_disabled"`
	// ForwardListen is the bind address for TCP port forwarding listeners.
	// Default "127.0.0.1" restricts access to the local machine only (v5.4.1).
	// Set to "0.0.0.0" to allow external access (e.g. Docker deployments), or
	// use the AIOPS_FORWARD_LISTEN environment variable to override.
	ForwardListen string `json:"forward_listen,omitempty"`
	// ForwardPortRange is the port range for TCP port forwarding ("min-max").
	// Default "10100-10300" for Docker deployments to expose a predictable range.
	// Set to "" or "0-0" to let the OS assign any available port.
	ForwardPortRange string `json:"forward_port_range,omitempty"`
	// HTTPProxies is the list of saved HTTP proxy shortcuts.
	// Each entry stores a target host+port+path for quick access.
	HTTPProxies []HTTPProxyConfig `json:"http_proxies,omitempty"`
	// ForwardRules is the list of persisted TCP forwarding rules.
	// Listeners are recreated on startup from these persisted fields.
	ForwardRules []PersistedForwardRule `json:"forward_rules,omitempty"`
	// AllowAnonymousAgents is an inverted flag: by default (zero value = false)
	// every agent MUST present a valid install token to register/report. Set true
	// only to permit token-less agents (not recommended).
	AllowAnonymousAgents bool `json:"allow_anonymous_agents"`
	// RelaySecret is the shared secret for gateway relay authentication (v5.4.1).
	// When set, all agent-facing requests (register, report, terminal, forward)
	// that arrive via a relay must carry the matching X-Relay-Secret header.
	// This prevents unauthorized relays from proxying to the upstream server.
	RelaySecret string `json:"relay_secret,omitempty"`
	// TrustProxy tells the server it sits behind a trusted reverse proxy, so it
	// may believe the X-Real-IP / X-Forwarded-For headers for the real client
	// address (used by login rate-limiting and audit logs). Default false: when
	// the server is directly exposed these headers are attacker-forgeable, so
	// they are ignored and the raw connection address is used instead.
	TrustProxy bool `json:"trust_proxy"`
	// MFARequired is the global MFA enforcement policy: when true, every user
	// without MFA enabled will be forced to enroll on their next login before
	// they can access the dashboard. Managed by admin via /api/v1/mfa/global.
	MFARequired bool `json:"mfa_required"`
	// Users is the multi-account list (RBAC). The legacy single Account above is
	// migrated into this list on load and then cleared.
	Users []AccountConfig `json:"users"`
}

func defaultServerConfig() ServerConfig {
	return ServerConfig{
		AlertsEnabled: true,
		Thresholds:    defaultThresholdConfig(),
		Categories:    map[string]string{},
		SMTP: SMTPConfig{
			FromName: "AIOps Monitor",
		},
	}
}

// Validate checks the server config for obvious misconfiguration before it is
// applied or persisted. Returns nil when the config is sound.
func (c ServerConfig) Validate() error {
	t := c.Thresholds
	// Threshold percentages must be in [0, 100].
	for name, v := range map[string]float64{
		"cpu_warn": t.CPUWarn, "cpu_crit": t.CPUCrit,
		"mem_warn": t.MemWarn, "mem_crit": t.MemCrit,
		"disk_warn": t.DiskWarn, "disk_crit": t.DiskCrit,
	} {
		if v < 0 || v > 100 {
			return fmt.Errorf("%s", Tz("config.threshold_range", name, v))
		}
	}
	// OfflineAfter must be positive.
	if t.OfflineAfterSec <= 0 {
		return fmt.Errorf("%s", Tz("config.offline_positive", t.OfflineAfterSec))
	}
	// SMTP port must be valid when SMTP is enabled.
	if c.SMTP.Enabled {
		if c.SMTP.Port < 1 || c.SMTP.Port > 65535 {
			return fmt.Errorf("%s", Tz("config.smtp_port_range", c.SMTP.Port))
		}
		// SMTP password (if set) must be at least 4 characters.
		if c.SMTP.Password != "" && len(c.SMTP.Password) < 4 {
			return fmt.Errorf("%s", Tz("config.smtp_password_short"))
		}
	}
	return nil
}

// ConfigStore wraps ServerConfig with disk persistence and thread safety.
type ConfigStore struct {
	mu      sync.RWMutex
	path    string
	cfg     ServerConfig
	prev    ServerConfig // snapshot before the last Set(), for Revert()
	hasPrev bool         // whether prev holds a valid snapshot
	pg      *pgStore     // when set, config persists to PostgreSQL instead of the JSON file
}

func NewConfigStore(path string, pg *pgStore) (*ConfigStore, error) {
	cs := &ConfigStore{path: path, cfg: defaultServerConfig(), pg: pg}
	loaded := false
	if pg != nil { // PostgreSQL is the source of truth in dual-DB mode
		if raw, ok, err := pg.loadConfigBlob(); err == nil && ok {
			var c ServerConfig
			if json.Unmarshal(raw, &c) == nil {
				if c.Categories == nil {
					c.Categories = map[string]string{}
				}
				cs.cfg = c
				loaded = true
			}
		}
	}
	if !loaded {
		if b, err := os.ReadFile(path); err == nil {
			var c ServerConfig
			if json.Unmarshal(b, &c) == nil {
				if c.Categories == nil {
					c.Categories = map[string]string{}
				}
				cs.cfg = c
			}
		}
	}
	// Decrypt any at-rest-encrypted secrets into plaintext for in-memory use
	// (no-op for plaintext / legacy values, or when no master key is set).
	decryptConfigSecrets(&cs.cfg)
	dirty := false
	if cs.cfg.InstallToken == "" {
		cs.cfg.InstallToken = genToken()
		dirty = true
	}
	// Heal thresholds: any metric left at 0 (missing in an older config, or saved
	// blank by the form) is backfilled to its standard default so every metric
	// always has a sane alert threshold. Persist the healed values.
	if backfillThresholdDefaults(&cs.cfg.Thresholds) {
		dirty = true
	}
	// Migrate the legacy single Account into the multi-user Users list and ensure
	// at least one admin exists (creates the default admin/admin on first run).
	if migrateUsers(&cs.cfg) {
		dirty = true
	}
	// Apply environment variable overrides (v5.4.1): Docker Compose users can
	// set AIOPS_* env vars to override config file values without editing JSON.
	cs.applyEnvOverrides()
	// Validate the loaded config — refuse to start with an obviously broken one.
	if err := cs.cfg.Validate(); err != nil {
		return nil, err
	}
	if dirty {
		_ = cs.save()
	}
	return cs, nil
}

// applyEnvOverrides reads AIOPS_* environment variables and overrides the
// corresponding config fields. This allows Docker Compose users to configure
// security-sensitive settings (forward_listen, relay_secret, etc.) via the
// environment block without editing server_config.json.
//
// Supported variables:
//
//	AIOPS_FORWARD_LISTEN          → forward_listen
//	AIOPS_FORWARD_PORT_RANGE      → forward_port_range
//	AIOPS_FORWARD_DISABLED        → forward_disabled (true/false)
//	AIOPS_TERMINAL_DISABLED       → terminal_disabled (true/false)
//	AIOPS_RELAY_SECRET            → relay_secret
//	AIOPS_ALLOW_ANONYMOUS_AGENTS  → allow_anonymous_agents (true/false)
//	AIOPS_TRUST_PROXY             → trust_proxy (true/false)
//	AIOPS_REQUIRE_TOKEN           → require_token (true/false)
func (cs *ConfigStore) applyEnvOverrides() {
	// External storage (Docker Compose points these at the VM / Postgres services):
	//   AIOPS_VM_URL         → enable VictoriaMetrics remote-write to this URL
	//   AIOPS_POSTGRES_DSN   → enable PostgreSQL persistence with this DSN
	if v, ok := os.LookupEnv("AIOPS_VM_URL"); ok && v != "" {
		cs.cfg.VM.Enabled = true
		cs.cfg.VM.URL = v
	}
	if v, ok := os.LookupEnv("AIOPS_POSTGRES_DSN"); ok && v != "" {
		cs.cfg.PostgresDSN = v
	}
	if v, ok := os.LookupEnv("AIOPS_FORWARD_LISTEN"); ok && v != "" {
		cs.cfg.ForwardListen = v
	}
	if v, ok := os.LookupEnv("AIOPS_FORWARD_PORT_RANGE"); ok && v != "" {
		cs.cfg.ForwardPortRange = v
	}
	if v, ok := os.LookupEnv("AIOPS_RELAY_SECRET"); ok && v != "" {
		cs.cfg.RelaySecret = v
	}
	if v, ok := os.LookupEnv("AIOPS_FORWARD_DISABLED"); ok && v != "" {
		cs.cfg.ForwardDisabled = v == "true" || v == "1"
	}
	if v, ok := os.LookupEnv("AIOPS_TERMINAL_DISABLED"); ok && v != "" {
		cs.cfg.TerminalDisabled = v == "true" || v == "1"
	}
	if v, ok := os.LookupEnv("AIOPS_ALLOW_ANONYMOUS_AGENTS"); ok && v != "" {
		cs.cfg.AllowAnonymousAgents = v == "true" || v == "1"
	}
	if v, ok := os.LookupEnv("AIOPS_TRUST_PROXY"); ok && v != "" {
		cs.cfg.TrustProxy = v == "true" || v == "1"
	}
	if v, ok := os.LookupEnv("AIOPS_REQUIRE_TOKEN"); ok && v != "" {
		cs.cfg.RequireToken = v == "true" || v == "1"
	}
}

func genToken() string {
	b := make([]byte, 16) // 32 hex characters
	if _, err := rand.Read(b); err != nil {
		return "aiops-token-fallback-0000000000000"
	}
	return hex.EncodeToString(b)
}

func (cs *ConfigStore) Get() ServerConfig {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.cfg
}

func (cs *ConfigStore) Thresholds() Thresholds {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.cfg.Thresholds.toThresholds()
}

func (cs *ConfigStore) InstallToken() string {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.cfg.InstallToken
}

func (cs *ConfigStore) RequireToken() bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.cfg.RequireToken
}

// AgentTokenRequired reports whether agents must present a valid install token.
// Enforced by default; only the explicit allow_anonymous_agents escape hatch
// disables it.
func (cs *ConfigStore) AgentTokenRequired() bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return !cs.cfg.AllowAnonymousAgents
}

// TerminalEnabled reports whether the remote terminal feature is available
// (default true; disabled only when terminal_disabled is set in config).
func (cs *ConfigStore) TerminalEnabled() bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return !cs.cfg.TerminalDisabled
}

// ForwardEnabled reports whether the port forwarding feature is available
// (default true; disabled only when forward_disabled is set in config).
func (cs *ConfigStore) ForwardEnabled() bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return !cs.cfg.ForwardDisabled
}

// ForwardListenAddr returns the configured bind address for TCP forwarding
// listeners. Defaults to "127.0.0.1" (localhost only) as a security measure —
// exposing forwarded ports to all network interfaces allows unauthenticated
// TCP connections to tunnel into internal services on monitored hosts.
// Set forward_listen to "0.0.0.0" explicitly in server_config.json to allow
// external access (e.g. for Docker deployments where 127.0.0.1 is only
// reachable inside the container).
func (cs *ConfigStore) ForwardListenAddr() string {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	if cs.cfg.ForwardListen == "" {
		return "127.0.0.1"
	}
	return cs.cfg.ForwardListen
}

// ForwardPortRangeBounds returns the min and max port for TCP forwarding.
// Defaults to 10100-10300 for predictable Docker port exposure.
// Returns (0, 0) to let the OS assign any port if not configured or "0-0".
func (cs *ConfigStore) ForwardPortRangeBounds() (minPort, maxPort int) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	if cs.cfg.ForwardPortRange == "" {
		return 10100, 10300 // default range for Docker (201 ports)
	}
	parts := strings.Split(cs.cfg.ForwardPortRange, "-")
	if len(parts) != 2 {
		return 10100, 10300
	}
	minPort = parseIntSafe(parts[0], 10100)
	maxPort = parseIntSafe(parts[1], 10300)
	if minPort <= 0 || maxPort <= 0 || minPort > maxPort {
		return 10100, 10300
	}
	return minPort, maxPort
}

func parseIntSafe(s string, def int) int {
	s = strings.TrimSpace(s)
	v := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			v = v*10 + int(c-'0')
		} else {
			return def
		}
	}
	if v == 0 {
		return def
	}
	return v
}

// TrustProxy reports whether to honor reverse-proxy client-IP headers
// (X-Real-IP / X-Forwarded-For). Off by default so a directly-exposed server
// can't be fooled by forged headers into miscounting login attempts.
func (cs *ConfigStore) TrustProxy() bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.cfg.TrustProxy
}

// MFARequired reports whether the global MFA enforcement policy is active.
func (cs *ConfigStore) MFARequired() bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.cfg.MFARequired
}

// RelaySecret returns the configured shared secret for gateway relay
// authentication (v5.4.1). Empty string means relay auth is disabled.
func (cs *ConfigStore) RelaySecret() string {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.cfg.RelaySecret
}

// SetMFARequired toggles the global MFA enforcement policy and persists it.
func (cs *ConfigStore) SetMFARequired(v bool) error {
	cs.mu.Lock()
	cs.cfg.MFARequired = v
	cs.mu.Unlock()
	return cs.save()
}

// tokenGracePeriod is how long a rotated-out token stays valid, so agents keep
// reporting after a rotation until their install command is updated.
const tokenGracePeriod = 7 * 24 * time.Hour

// ResetToken ROTATES the install token: the current token becomes the previous
// token (valid for tokenGracePeriod), then a fresh token is generated and
// returned. Existing agents keep working during the grace window — a rotation is
// no longer an instant "all agents offline" event.
func (cs *ConfigStore) ResetToken() string {
	cs.mu.Lock()
	if cs.cfg.InstallToken != "" {
		cs.cfg.PrevInstallToken = cs.cfg.InstallToken
		cs.cfg.PrevTokenExpiresAt = time.Now().Add(tokenGracePeriod).Unix()
	}
	cs.cfg.InstallToken = genToken()
	tok := cs.cfg.InstallToken
	cs.mu.Unlock()
	_ = cs.save()
	return tok
}

// PrevTokenValidUntil returns the unix expiry of the grace-period token, or 0 if
// none is active (used by the UI to show "old token valid until …").
func (cs *ConfigStore) PrevTokenValidUntil() int64 {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	if cs.cfg.PrevInstallToken == "" || time.Now().Unix() >= cs.cfg.PrevTokenExpiresAt {
		return 0
	}
	return cs.cfg.PrevTokenExpiresAt
}

// ValidInstallToken reports whether got matches the current token, or the
// previous token during its grace period. Constant-time.
func (cs *ConfigStore) ValidInstallToken(got string) bool {
	cs.mu.RLock()
	cur := cs.cfg.InstallToken
	prev := cs.cfg.PrevInstallToken
	prevExp := cs.cfg.PrevTokenExpiresAt
	cs.mu.RUnlock()
	if cur != "" && subtle.ConstantTimeCompare([]byte(got), []byte(cur)) == 1 {
		return true
	}
	if prev != "" && time.Now().Unix() < prevExp &&
		subtle.ConstantTimeCompare([]byte(got), []byte(prev)) == 1 {
		return true
	}
	return false
}

// ---- account ----

func (cs *ConfigStore) Account() AccountConfig {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.cfg.Account
}

func (cs *ConfigStore) SetProfile(display, email string) error {
	cs.mu.Lock()
	cs.cfg.Account.DisplayName = display
	cs.cfg.Account.Email = email
	cs.mu.Unlock()
	return cs.save()
}

// SetUsername changes the login username (the account identifier).
func (cs *ConfigStore) SetUsername(name string) error {
	cs.mu.Lock()
	cs.cfg.Account.Username = name
	cs.mu.Unlock()
	return cs.save()
}

// SetMFA enables or disables the TOTP second factor. Disabling clears the secret
// so a stale secret can never linger in the config.
func (cs *ConfigStore) SetMFA(enabled bool, secret string) error {
	cs.mu.Lock()
	cs.cfg.Account.MFAEnabled = enabled
	if enabled {
		cs.cfg.Account.MFASecret = secret
	} else {
		cs.cfg.Account.MFASecret = ""
	}
	cs.mu.Unlock()
	return cs.save()
}

func (cs *ConfigStore) SetPassword(newPass string) error {
	cs.mu.Lock()
	salt := genToken()[:16]
	cs.cfg.Account.Salt = salt
	cs.cfg.Account.Hash = hashPassword(newPass, salt)
	cs.mu.Unlock()
	return cs.save()
}

// ---- custom checks ----

func (cs *ConfigStore) Checks() []CustomCheck {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	out := make([]CustomCheck, len(cs.cfg.Checks))
	copy(out, cs.cfg.Checks)
	return out
}

// UpsertCheck adds a new check (assigning an id) or replaces one by id.
func (cs *ConfigStore) UpsertCheck(c CustomCheck) (CustomCheck, error) {
	cs.mu.Lock()
	if c.ID == "" {
		c.ID = genToken()[:8]
		cs.cfg.Checks = append(cs.cfg.Checks, c)
	} else {
		found := false
		for i := range cs.cfg.Checks {
			if cs.cfg.Checks[i].ID == c.ID {
				cs.cfg.Checks[i] = c
				found = true
				break
			}
		}
		if !found {
			cs.cfg.Checks = append(cs.cfg.Checks, c)
		}
	}
	cs.mu.Unlock()
	return c, cs.save()
}

func (cs *ConfigStore) DeleteCheck(id string) error {
	cs.mu.Lock()
	kept := cs.cfg.Checks[:0]
	for _, c := range cs.cfg.Checks {
		if c.ID != id {
			kept = append(kept, c)
		}
	}
	cs.cfg.Checks = kept
	cs.mu.Unlock()
	return cs.save()
}

// Set replaces the alert/threshold config, preserving category overrides, and
// persists to disk. The current config is snapshotted first so a bad change can
// be rolled back via Revert().
func (cs *ConfigStore) Set(c ServerConfig) error {
	// Backfill any zero threshold to its standard default BEFORE validating, so a
	// blank form field (which arrives as 0) becomes the default instead of an
	// invalid/meaningless 0 — this is what makes "save uses standard thresholds".
	backfillThresholdDefaults(&c.Thresholds)
	if err := c.Validate(); err != nil {
		return err
	}
	cs.mu.Lock()
	cs.prev = cs.cfg // snapshot for potential rollback
	cs.hasPrev = true
	c.Categories = cs.cfg.Categories             // categories managed via SetCategory
	c.InstallToken = cs.cfg.InstallToken         // token managed via install endpoints
	c.Account = cs.cfg.Account                   // account managed via auth endpoints
	c.Checks = cs.cfg.Checks                     // checks managed via check endpoints
	c.Playbooks = cs.cfg.Playbooks               // playbooks managed via playbook endpoints
	c.APISystems = cs.cfg.APISystems             // API 性能监控：由专用端点管理，保护不被表单清零
	c.Governance = cs.cfg.Governance             // 告警治理：由专用端点管理，保护不被表单清零
	c.RemediationRules = cs.cfg.RemediationRules // managed via remediation endpoints
	c.SLOs = cs.cfg.SLOs                         // managed via SLO endpoints
	c.AI = cs.cfg.AI                             // managed via AI config endpoint
	c.VM = cs.cfg.VM                             // managed via env / storage config
	c.PostgresDSN = cs.cfg.PostgresDSN           // managed via env / storage config
	c.RelaySecret = cs.cfg.RelaySecret           // managed via storage/relay config (masked in GET)
	c.HTTPProxies = cs.cfg.HTTPProxies           // managed via proxy endpoints
	c.ForwardRules = cs.cfg.ForwardRules         // managed via forward endpoints
	// Preserve SMTP password when the incoming value is blank or masked (same
	// strategy as webhook secrets — the browser may submit without re-typing it).
	if c.SMTP.Password == "" || strings.Contains(c.SMTP.Password, "****") {
		c.SMTP.Password = cs.cfg.SMTP.Password
	}
	if c.SMTP.FromName == "" {
		c.SMTP.FromName = cs.cfg.SMTP.FromName
	}
	// Preserve SMS secret key when the incoming value is blank or masked.
	if c.SMS.SecretKey == "" || strings.Contains(c.SMS.SecretKey, "****") {
		c.SMS.SecretKey = cs.cfg.SMS.SecretKey
	}
	// Preserve VoiceCall secret key when the incoming value is blank or masked.
	if c.VoiceCall.SecretKey == "" || strings.Contains(c.VoiceCall.SecretKey, "****") {
		c.VoiceCall.SecretKey = cs.cfg.VoiceCall.SecretKey
	}
	// Operational security flags are managed via the config file, not the alert
	// settings form — preserve them so a settings save can't silently flip them.
	c.RequireToken = cs.cfg.RequireToken
	c.TerminalDisabled = cs.cfg.TerminalDisabled
	c.ForwardDisabled = cs.cfg.ForwardDisabled
	c.ForwardListen = cs.cfg.ForwardListen
	c.ForwardPortRange = cs.cfg.ForwardPortRange
	c.AllowAnonymousAgents = cs.cfg.AllowAnonymousAgents
	c.TrustProxy = cs.cfg.TrustProxy
	c.MFARequired = cs.cfg.MFARequired
	c.Users = cs.cfg.Users // 保护多用户列表：前端表单不含 Users，必须保留现有值
	cs.cfg = c
	cs.mu.Unlock()
	return cs.save()
}

// Revert restores the config that was active before the most recent successful
// Set(), undoing a bad configuration change. Returns an error when there is no
// previous snapshot to revert to.
func (cs *ConfigStore) Revert() error {
	cs.mu.Lock()
	if !cs.hasPrev {
		cs.mu.Unlock()
		return fmt.Errorf("%s", Tz("config.no_revert"))
	}
	cs.cfg = cs.prev
	cs.prev = ServerConfig{}
	cs.hasPrev = false
	cs.mu.Unlock()
	return cs.save()
}

// SetCategory records (or clears, when cat is empty) a manual category override.
func (cs *ConfigStore) SetCategory(hostID, cat string) error {
	cs.mu.Lock()
	if cs.cfg.Categories == nil {
		cs.cfg.Categories = map[string]string{}
	}
	if cat == "" {
		delete(cs.cfg.Categories, hostID)
	} else {
		cs.cfg.Categories[hostID] = cat
	}
	cs.mu.Unlock()
	return cs.save()
}

func (cs *ConfigStore) CategoryOverride(hostID string) (string, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	c, ok := cs.cfg.Categories[hostID]
	return c, ok
}

func (cs *ConfigStore) save() error {
	cs.mu.RLock()
	// Value copy so field-level secret encryption below can't mutate the live,
	// plaintext in-memory config. Deep-copy Users (a slice) for the same reason.
	c := cs.cfg
	if len(c.Users) > 0 {
		users := make([]AccountConfig, len(c.Users))
		copy(users, c.Users)
		c.Users = users
	}
	pg := cs.pg
	cs.mu.RUnlock()
	// Encrypt reversible secrets at rest (no-op unless AIOPS_SECRET_KEY is set).
	encryptConfigSecrets(&c)
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if pg != nil { // PostgreSQL-backed: persist the whole config as one JSONB row
		return pg.saveConfigBlob(b)
	}
	// 0o600: this file holds password hashes, MFA secrets and the install token —
	// it must not be world-readable on a shared host.
	if err := os.WriteFile(cs.path, b, 0o600); err != nil {
		return err
	}
	// WriteFile keeps the existing mode when the file already exists, so force
	// 0o600 to also tighten configs written by earlier (0o644) versions.
	_ = os.Chmod(cs.path, 0o600)
	return nil
}

// ---- playbooks ----

func (cs *ConfigStore) Playbooks() []Playbook {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	out := make([]Playbook, len(cs.cfg.Playbooks))
	copy(out, cs.cfg.Playbooks)
	return out
}

func (cs *ConfigStore) UpsertPlaybook(p Playbook) (Playbook, error) {
	cs.mu.Lock()
	if p.ID == "" {
		cs.cfg.Playbooks = append(cs.cfg.Playbooks, p)
	} else {
		found := false
		for i := range cs.cfg.Playbooks {
			if cs.cfg.Playbooks[i].ID == p.ID {
				cs.cfg.Playbooks[i] = p
				found = true
				break
			}
		}
		if !found {
			cs.cfg.Playbooks = append(cs.cfg.Playbooks, p)
		}
	}
	cs.mu.Unlock()
	return p, cs.save()
}

func (cs *ConfigStore) DeletePlaybook(id string) error {
	cs.mu.Lock()
	kept := cs.cfg.Playbooks[:0]
	for _, p := range cs.cfg.Playbooks {
		if p.ID != id {
			kept = append(kept, p)
		}
	}
	cs.cfg.Playbooks = kept
	cs.mu.Unlock()
	return cs.save()
}

// ---- remediation rules ----

func (cs *ConfigStore) RemediationRules() []RemediationRule {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	out := make([]RemediationRule, len(cs.cfg.RemediationRules))
	copy(out, cs.cfg.RemediationRules)
	return out
}

func (cs *ConfigStore) UpsertRemediationRule(r RemediationRule) (RemediationRule, error) {
	cs.mu.Lock()
	if r.ID == "" {
		r.ID = genToken()[:8]
		r.CreatedAt = time.Now().Unix()
		r.UpdatedAt = r.CreatedAt
		cs.cfg.RemediationRules = append(cs.cfg.RemediationRules, r)
	} else {
		r.UpdatedAt = time.Now().Unix()
		found := false
		for i := range cs.cfg.RemediationRules {
			if cs.cfg.RemediationRules[i].ID == r.ID {
				r.CreatedAt = cs.cfg.RemediationRules[i].CreatedAt
				cs.cfg.RemediationRules[i] = r
				found = true
				break
			}
		}
		if !found {
			r.CreatedAt = time.Now().Unix()
			cs.cfg.RemediationRules = append(cs.cfg.RemediationRules, r)
		}
	}
	cs.mu.Unlock()
	return r, cs.save()
}

func (cs *ConfigStore) DeleteRemediationRule(id string) error {
	cs.mu.Lock()
	kept := cs.cfg.RemediationRules[:0]
	for _, r := range cs.cfg.RemediationRules {
		if r.ID != id {
			kept = append(kept, r)
		}
	}
	cs.cfg.RemediationRules = kept
	cs.mu.Unlock()
	return cs.save()
}

// ---- SLOs ----

func (cs *ConfigStore) SLOs() []SLO {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	out := make([]SLO, len(cs.cfg.SLOs))
	copy(out, cs.cfg.SLOs)
	return out
}

func (cs *ConfigStore) UpsertSLO(s SLO) (SLO, error) {
	cs.mu.Lock()
	if s.ID == "" {
		s.ID = genToken()[:8]
		s.CreatedAt = time.Now().Unix()
		s.UpdatedAt = s.CreatedAt
		cs.cfg.SLOs = append(cs.cfg.SLOs, s)
	} else {
		s.UpdatedAt = time.Now().Unix()
		found := false
		for i := range cs.cfg.SLOs {
			if cs.cfg.SLOs[i].ID == s.ID {
				s.CreatedAt = cs.cfg.SLOs[i].CreatedAt
				cs.cfg.SLOs[i] = s
				found = true
				break
			}
		}
		if !found {
			s.CreatedAt = time.Now().Unix()
			cs.cfg.SLOs = append(cs.cfg.SLOs, s)
		}
	}
	cs.mu.Unlock()
	return s, cs.save()
}

func (cs *ConfigStore) DeleteSLO(id string) error {
	cs.mu.Lock()
	kept := cs.cfg.SLOs[:0]
	for _, s := range cs.cfg.SLOs {
		if s.ID != id {
			kept = append(kept, s)
		}
	}
	cs.cfg.SLOs = kept
	cs.mu.Unlock()
	return cs.save()
}

// ---- AI provider config ----

func (cs *ConfigStore) AIConfig() AIConfig {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.cfg.AI
}

func (cs *ConfigStore) SetAIConfig(a AIConfig) error {
	cs.mu.Lock()
	// Preserve a previously-saved API key when the browser submits a masked/blank one.
	if a.APIKey == "" || strings.Contains(a.APIKey, "****") {
		a.APIKey = cs.cfg.AI.APIKey
	}
	// 嵌入 Key 同样：表单提交空/脱敏值时保留原值。
	if a.EmbedAPIKey == "" || strings.Contains(a.EmbedAPIKey, "****") {
		a.EmbedAPIKey = cs.cfg.AI.EmbedAPIKey
	}
	// AI 配置表单不含这些 Hermes 开关（由专门流程管理），保存表单时保留其现值，避免被表单清零。
	a.HermesEnabled = cs.cfg.AI.HermesEnabled
	a.HermesAutoApprove = cs.cfg.AI.HermesAutoApprove
	a.HermesTerminalEnabled = cs.cfg.AI.HermesTerminalEnabled
	cs.cfg.AI = a
	cs.mu.Unlock()
	return cs.save()
}

// SetHermesTerminalEnabled 单独设置「AI 终端只读巡检」开关
// （由 handleAITerminalAccess 在校验终端密码后调用；不走上面的表单保存路径）。
func (cs *ConfigStore) SetHermesTerminalEnabled(v bool) error {
	cs.mu.Lock()
	cs.cfg.AI.HermesTerminalEnabled = v
	cs.mu.Unlock()
	return cs.save()
}

// ---- external storage (VictoriaMetrics / PostgreSQL), usually set via env ----

func (cs *ConfigStore) VMConfig() VMConfig {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.cfg.VM
}

func (cs *ConfigStore) PostgresDSN() string {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.cfg.PostgresDSN
}

// --- HTTP Proxy Config Management ---

// ListHTTPProxies returns all saved HTTP proxy configurations.
func (cs *ConfigStore) ListHTTPProxies() []HTTPProxyConfig {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return append([]HTTPProxyConfig{}, cs.cfg.HTTPProxies...)
}

// AddHTTPProxy adds a new HTTP proxy configuration.
func (cs *ConfigStore) AddHTTPProxy(proxy HTTPProxyConfig) error {
	cs.mu.Lock()
	if proxy.ID == "" {
		proxy.ID = termID()[:8]
	}
	if proxy.CreatedAt == 0 {
		proxy.CreatedAt = time.Now().Unix()
	}
	cs.cfg.HTTPProxies = append(cs.cfg.HTTPProxies, proxy)
	cs.mu.Unlock()
	return cs.save()
}

// DeleteHTTPProxy removes an HTTP proxy configuration by ID.
func (cs *ConfigStore) DeleteHTTPProxy(id string) error {
	cs.mu.Lock()
	var kept []HTTPProxyConfig
	for _, p := range cs.cfg.HTTPProxies {
		if p.ID != id {
			kept = append(kept, p)
		}
	}
	cs.cfg.HTTPProxies = kept
	cs.mu.Unlock()
	return cs.save()
}

// ToggleHTTPProxy enables or disables an HTTP proxy configuration.
func (cs *ConfigStore) ToggleHTTPProxy(id string, enabled bool) error {
	cs.mu.Lock()
	found := false
	for i, p := range cs.cfg.HTTPProxies {
		if p.ID == id {
			cs.cfg.HTTPProxies[i].Enabled = enabled
			found = true
			break
		}
	}
	cs.mu.Unlock()
	if !found {
		return fmt.Errorf("proxy not found")
	}
	return cs.save()
}

// UpdateHTTPProxy updates an existing HTTP proxy configuration.
// The Enabled field is preserved from the existing config when the caller
// doesn't explicitly set it to true — use the toggle API for enable/disable.
func (cs *ConfigStore) UpdateHTTPProxy(id string, updated HTTPProxyConfig) error {
	cs.mu.Lock()
	found := false
	for i, p := range cs.cfg.HTTPProxies {
		if p.ID == id {
			// Preserve ID, CreatedAt, and Enabled (when not explicitly set)
			updated.ID = p.ID
			updated.CreatedAt = p.CreatedAt
			if !updated.Enabled {
				updated.Enabled = p.Enabled
			}
			cs.cfg.HTTPProxies[i] = updated
			found = true
			break
		}
	}
	cs.mu.Unlock()
	if !found {
		return fmt.Errorf("proxy not found")
	}
	return cs.save()
}

// CopyHTTPProxy duplicates an HTTP proxy configuration with a new ID.
func (cs *ConfigStore) CopyHTTPProxy(id string) (HTTPProxyConfig, error) {
	cs.mu.Lock()
	var original *HTTPProxyConfig
	for i, p := range cs.cfg.HTTPProxies {
		if p.ID == id {
			original = &cs.cfg.HTTPProxies[i]
			break
		}
	}
	if original == nil {
		cs.mu.Unlock()
		return HTTPProxyConfig{}, fmt.Errorf("proxy not found")
	}
	// Create a copy with new ID and timestamp
	newProxy := *original
	newProxy.ID = termID()[:8]
	newProxy.CreatedAt = time.Now().Unix()
	// Store a neutral name (no language-specific suffix) and mark it as a copy;
	// the UI appends the localized "forward.copy_suffix" at render time.
	newProxy.Name = original.Name
	newProxy.IsCopy = true
	cs.cfg.HTTPProxies = append(cs.cfg.HTTPProxies, newProxy)
	cs.mu.Unlock()
	if err := cs.save(); err != nil {
		return HTTPProxyConfig{}, err
	}
	return newProxy, nil
}

// ---- Forward Rules (TCP port forwarding) ----

func (cs *ConfigStore) ListForwardRules() []PersistedForwardRule {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return append([]PersistedForwardRule{}, cs.cfg.ForwardRules...)
}

func (cs *ConfigStore) AddForwardRule(r PersistedForwardRule) error {
	cs.mu.Lock()
	cs.cfg.ForwardRules = append(cs.cfg.ForwardRules, r)
	cs.mu.Unlock()
	return cs.save()
}

func (cs *ConfigStore) DeleteForwardRule(id string) error {
	cs.mu.Lock()
	kept := cs.cfg.ForwardRules[:0]
	for _, r := range cs.cfg.ForwardRules {
		if r.ID != id {
			kept = append(kept, r)
		}
	}
	cs.cfg.ForwardRules = kept
	cs.mu.Unlock()
	return cs.save()
}

func (cs *ConfigStore) ToggleForwardRule(id string, enabled bool) error {
	cs.mu.Lock()
	found := false
	for i, r := range cs.cfg.ForwardRules {
		if r.ID == id {
			cs.cfg.ForwardRules[i].Enabled = enabled
			found = true
			break
		}
	}
	cs.mu.Unlock()
	if !found {
		return fmt.Errorf("forward rule not found")
	}
	return cs.save()
}

func (cs *ConfigStore) UpdateForwardRule(id string, updated PersistedForwardRule) error {
	cs.mu.Lock()
	found := false
	for i, r := range cs.cfg.ForwardRules {
		if r.ID == id {
			updated.ID = r.ID
			updated.CreatedAt = r.CreatedAt
			cs.cfg.ForwardRules[i] = updated
			found = true
			break
		}
	}
	cs.mu.Unlock()
	if !found {
		return fmt.Errorf("forward rule not found")
	}
	return cs.save()
}
