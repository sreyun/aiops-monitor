package main

import (
	"strings"
	"time"
)

// ============================================================================
// 告警治理四件套
//
// 在通知下发前（notify.go pushChannels）插入一层治理决策：
//   ① 静默规则  SilenceRule —— 命中的告警不推送（仍记录、仍在 UI 展示）
//   ② 生效时段  —— 静默规则可带时间窗（如 22:00-08:00 夜间静默）+ 星期
//   ③ 抑制规则  InhibitRule —— 某源告警活跃时，抑制同主机的目标告警通知（如主机离线→抑制其 CPU/内存告警）
//   ④ 通知路由  NotifyRoute —— 按 级别/主机/类型 决定发往哪些渠道（未命中任何路由=默认全部启用渠道）
//
// 只作用于「触发(firing)」通知；「恢复」通知一律照发，避免规则导致"永远告警"的错觉。
// ============================================================================

// AlertMatch 是三类规则共用的匹配条件（各字段留空=不限）。
type AlertMatch struct {
	HostPattern string   `json:"host_pattern,omitempty"` // 主机名/IP 子串匹配（大小写不敏感）
	Types       []string `json:"types,omitempty"`        // 告警类型集合：cpu/memory/disk/offline/load/gpu/check/api
	Levels      []string `json:"levels,omitempty"`       // 级别集合：warning/critical
}

func containsFold(list []string, v string) bool {
	for _, s := range list {
		if strings.EqualFold(strings.TrimSpace(s), v) {
			return true
		}
	}
	return false
}

// matches 判断一条告警是否满足匹配条件。
func (m AlertMatch) matches(a Alert) bool {
	if p := strings.TrimSpace(m.HostPattern); p != "" {
		p = strings.ToLower(p)
		if !strings.Contains(strings.ToLower(a.Hostname), p) && !strings.Contains(strings.ToLower(a.IP), p) {
			return false
		}
	}
	if len(m.Types) > 0 && !containsFold(m.Types, a.Type) {
		return false
	}
	if len(m.Levels) > 0 && !containsFold(m.Levels, a.Level) {
		return false
	}
	return true
}

// SilenceRule 静默规则：命中的告警不推送通知，可选生效时段（夜间静默）与星期。
type SilenceRule struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Enabled   bool       `json:"enabled"`
	Match     AlertMatch `json:"match"`
	TimeStart string     `json:"time_start,omitempty"` // HH:MM，留空=全天
	TimeEnd   string     `json:"time_end,omitempty"`   // HH:MM，支持跨天（22:00-08:00）
	Weekdays  []int      `json:"weekdays,omitempty"`   // 0=周日..6=周六，留空=每天
}

// InhibitRule 抑制规则：当有匹配 Source 的告警活跃时，抑制匹配 Target 的告警通知。
type InhibitRule struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	Enabled  bool       `json:"enabled"`
	Source   AlertMatch `json:"source"`
	Target   AlertMatch `json:"target"`
	SameHost bool       `json:"same_host"` // 是否要求源与目标同主机（常见：主机离线抑制其自身指标告警）
}

// NotifyRoute 通知路由：命中的告警仅发往 Channels 指定的渠道。
type NotifyRoute struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	Enabled  bool       `json:"enabled"`
	Match    AlertMatch `json:"match"`
	Channels []string   `json:"channels"` // feishu/dingtalk/smtp/webhook
	Continue bool       `json:"continue"` // 命中后是否继续匹配后续路由（默认 false=命中即停）
}

// AlertGovernance 是告警治理配置总集（持久化在 ServerConfig 内）。
type AlertGovernance struct {
	SilenceRules []SilenceRule `json:"silence_rules,omitempty"`
	InhibitRules []InhibitRule `json:"inhibit_rules,omitempty"`
	Routes       []NotifyRoute `json:"routes,omitempty"`
}

// govHHMM 把 "HH:MM" 解析为当天分钟数（0-1439）；非法返回 -1。
func govHHMM(s string) int {
	s = strings.TrimSpace(s)
	i := strings.IndexByte(s, ':')
	if i <= 0 {
		return -1
	}
	h, e1 := atoiSafe(s[:i])
	m, e2 := atoiSafe(s[i+1:])
	if !e1 || !e2 || h < 0 || h > 23 || m < 0 || m > 59 {
		return -1
	}
	return h*60 + m
}

func atoiSafe(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	v := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
		v = v*10 + int(c-'0')
	}
	return v, true
}

// activeNow 判断静默规则的生效时段/星期此刻是否命中。
func (r SilenceRule) activeNow(now time.Time) bool {
	if len(r.Weekdays) > 0 {
		wd := int(now.Weekday()) // 0=Sunday
		hit := false
		for _, d := range r.Weekdays {
			if d == wd {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	s, e := govHHMM(r.TimeStart), govHHMM(r.TimeEnd)
	if s < 0 || e < 0 {
		return true // 无有效时段=全天生效
	}
	cur := now.Hour()*60 + now.Minute()
	if s <= e {
		return cur >= s && cur < e
	}
	return cur >= s || cur < e // 跨天，如 22:00-08:00
}

// govSilenced 判断告警是否被某条静默规则命中（含生效时段）。返回命中的规则名。
func govSilenced(g AlertGovernance, a Alert, now time.Time) (bool, string) {
	for _, r := range g.SilenceRules {
		if r.Enabled && r.Match.matches(a) && r.activeNow(now) {
			return true, r.Name
		}
	}
	return false, ""
}

// govInhibited 判断告警是否被某条抑制规则命中：存在匹配 Source 的其它活跃告警时抑制之。
func govInhibited(g AlertGovernance, a Alert, active []Alert) (bool, string) {
	for _, r := range g.InhibitRules {
		if !r.Enabled || !r.Target.matches(a) {
			continue
		}
		for _, x := range active {
			if alertKey(x) == alertKey(a) { // 不被自己抑制
				continue
			}
			if r.SameHost && x.HostID != a.HostID {
				continue
			}
			if r.Source.matches(x) {
				return true, r.Name
			}
		}
	}
	return false, ""
}

// govRouteChannels 按路由规则决定告警发往哪些渠道。routed=false 表示无路由命中（调用方回退默认全部启用渠道）。
func govRouteChannels(g AlertGovernance, a Alert) (sel map[string]bool, routed bool) {
	sel = map[string]bool{}
	for _, r := range g.Routes {
		if !r.Enabled || !r.Match.matches(a) {
			continue
		}
		routed = true
		for _, ch := range r.Channels {
			sel[strings.ToLower(strings.TrimSpace(ch))] = true
		}
		if !r.Continue {
			break
		}
	}
	return sel, routed
}

// ---- ConfigStore 存取 ----

func (cs *ConfigStore) Governance() AlertGovernance {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.cfg.Governance
}

func (cs *ConfigStore) SetGovernance(g AlertGovernance) error {
	cs.mu.Lock()
	// 为缺 ID 的规则补稳定 ID
	for i := range g.SilenceRules {
		if strings.TrimSpace(g.SilenceRules[i].ID) == "" {
			g.SilenceRules[i].ID = genToken()[:8]
		}
	}
	for i := range g.InhibitRules {
		if strings.TrimSpace(g.InhibitRules[i].ID) == "" {
			g.InhibitRules[i].ID = genToken()[:8]
		}
	}
	for i := range g.Routes {
		if strings.TrimSpace(g.Routes[i].ID) == "" {
			g.Routes[i].ID = genToken()[:8]
		}
	}
	cs.cfg.Governance = g
	cs.mu.Unlock()
	return cs.save()
}
