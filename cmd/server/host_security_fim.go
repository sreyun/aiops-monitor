package main

import (
	"sort"
	"strings"
)

const (
	fimMaxStoredInv  = 80
	fimMaxStoredDiff = 48 << 10
	fimMaxStoredChg  = 80
)

// fimNormalizePath keeps agent-reported paths stable across server GOOS.
// Never use filepath.Clean here: a Windows server would turn "/etc/hosts" into "\etc\hosts".
func fimNormalizePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	p = strings.ReplaceAll(p, "\\", "/")
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	if len(p) > 1 && strings.HasSuffix(p, "/") {
		p = strings.TrimRight(p, "/")
	}
	return p
}

func trimHostFileInventory(inv []hsAgentFileInv) []HostFileHash {
	if len(inv) == 0 {
		return nil
	}
	out := make([]HostFileHash, 0, len(inv))
	seen := map[string]bool{}
	for _, it := range inv {
		p := fimNormalizePath(it.Path)
		if p == "" || it.SHA256 == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, HostFileHash{
			Path: p, SHA256: strings.ToLower(strings.TrimSpace(it.SHA256)),
			Size: it.Size, Mtime: it.Mtime, Kind: it.Kind,
		})
		if len(out) >= fimMaxStoredInv {
			break
		}
	}
	return out
}

func sanitizeFileChanges(changes []HostFileChange) []HostFileChange {
	if len(changes) == 0 {
		return nil
	}
	if len(changes) > fimMaxStoredChg {
		changes = changes[:fimMaxStoredChg]
	}
	out := make([]HostFileChange, 0, len(changes))
	for _, ch := range changes {
		ch.Path = fimNormalizePath(ch.Path)
		if ch.Path == "" {
			continue
		}
		switch ch.Change {
		case "added", "removed", "modified":
		default:
			continue
		}
		if len(ch.Diff) > fimMaxStoredDiff {
			ch.Diff = ch.Diff[:fimMaxStoredDiff]
			ch.Truncated = true
		}
		out = append(out, ch)
	}
	return out
}

func indexHostFileInventory(inv []HostFileHash) map[string]HostFileHash {
	m := make(map[string]HostFileHash, len(inv))
	for _, it := range inv {
		m[it.Path] = it
	}
	return m
}

func indexAgentTextDiffs(diffs []hsAgentTextDiff) map[string]hsAgentTextDiff {
	m := make(map[string]hsAgentTextDiff, len(diffs))
	for _, d := range diffs {
		p := fimNormalizePath(d.Path)
		if p == "" {
			continue
		}
		if len(d.Diff) > fimMaxStoredDiff {
			d.Diff = d.Diff[:fimMaxStoredDiff]
			d.Truncated = true
		}
		m[p] = d
	}
	return m
}

// pickFIMInventoryToStore chooses which inventory to persist on a completed scan.
// Never replace a nonempty baseline with an empty cur (would force perpetual re-baseline).
func pickFIMInventoryToStore(cur, livePrev, hostPrev []HostFileHash, fimOn bool) []HostFileHash {
	if len(cur) > 0 {
		return cur
	}
	if !fimOn {
		if len(livePrev) > 0 {
			return livePrev
		}
		return hostPrev
	}
	if len(livePrev) > 0 {
		return livePrev
	}
	if len(hostPrev) > 0 {
		return hostPrev
	}
	return cur
}

// diffHostFileInventory compares current inventory to previous baseline.
// When prev is empty/nil, returns nil changes and baselineEstablished=true (first scan).
func diffHostFileInventory(prev, cur []HostFileHash, textDiffs []hsAgentTextDiff) (changes []HostFileChange, baselineEstablished bool) {
	if len(prev) == 0 {
		if len(cur) > 0 {
			return nil, true
		}
		return nil, false
	}
	if len(cur) == 0 {
		// Old agent or FIM disabled mid-flight: no noisy removed spam.
		return nil, false
	}
	prevMap := indexHostFileInventory(prev)
	curMap := indexHostFileInventory(cur)
	diffMap := indexAgentTextDiffs(textDiffs)

	for path, c := range curMap {
		p, ok := prevMap[path]
		if !ok {
			changes = append(changes, HostFileChange{
				Path: path, Change: "added", NewSHA: c.SHA256, NewMtime: c.Mtime,
			})
			continue
		}
		if !strings.EqualFold(p.SHA256, c.SHA256) {
			ch := HostFileChange{
				Path: path, Change: "modified",
				OldSHA: p.SHA256, NewSHA: c.SHA256,
				OldMtime: p.Mtime, NewMtime: c.Mtime,
			}
			if d, ok := diffMap[path]; ok {
				ch.Diff = d.Diff
				ch.Truncated = d.Truncated
				if ch.OldSHA == "" {
					ch.OldSHA = d.OldSHA
				}
			}
			changes = append(changes, ch)
		}
	}
	for path, p := range prevMap {
		if _, ok := curMap[path]; !ok {
			changes = append(changes, HostFileChange{
				Path: path, Change: "removed", OldSHA: p.SHA256, OldMtime: p.Mtime,
			})
		}
	}
	// Stable-ish order: critical paths first, then path.
	sortHostFileChanges(changes)
	return sanitizeFileChanges(changes), false
}

func sortHostFileChanges(changes []HostFileChange) {
	rank := func(c HostFileChange) int {
		switch fimPathSeverity(c.Path) {
		case "crit":
			return 0
		case "high":
			return 1
		case "medium":
			return 2
		default:
			return 3
		}
	}
	sort.SliceStable(changes, func(i, j int) bool {
		ri, rj := rank(changes[i]), rank(changes[j])
		if ri != rj {
			return ri < rj
		}
		return changes[i].Path < changes[j].Path
	})
}

func fimBaseName(path string) string {
	p := fimNormalizePath(path)
	if i := strings.LastIndex(p, "/"); i >= 0 && i+1 < len(p) {
		return p[i+1:]
	}
	return p
}

func fimPathSeverity(path string) string {
	base := strings.ToLower(fimBaseName(path))
	switch base {
	case "shadow", "sshd_config", "authorized_keys", "sudoers":
		return "high"
	case "passwd", "group", "crontab", "hosts", "resolv.conf", "rc.local":
		return "medium"
	}
	lower := strings.ToLower(fimNormalizePath(path))
	if strings.Contains(lower, "authorized_keys") || strings.Contains(lower, "sudoers") {
		return "high"
	}
	if strings.Contains(lower, "cron.d") || strings.Contains(lower, "startup") {
		return "medium"
	}
	if strings.HasSuffix(base, ".exe") || strings.Contains(lower, "/usr/local/bin/") {
		return "medium"
	}
	return "low"
}

func fimFindingsFromChanges(changes []HostFileChange) []HostFinding {
	var out []HostFinding
	for _, ch := range changes {
		level := fimPathSeverity(ch.Path)
		// Critical auth files that change → bump.
		base := strings.ToLower(fimBaseName(ch.Path))
		if (base == "shadow" || base == "authorized_keys" || base == "sudoers") && ch.Change != "added" {
			if level == "high" {
				level = "crit"
			}
		}
		title := "文件完整性变更"
		switch ch.Change {
		case "added":
			title = "监控文件新增"
		case "removed":
			title = "监控文件删除"
		case "modified":
			title = "监控文件内容变更"
		}
		detail := ch.Path
		if ch.Change == "modified" {
			detail += " (sha " + shortSHA(ch.OldSHA) + " → " + shortSHA(ch.NewSHA) + ")"
		} else if ch.Change == "added" {
			detail += " (sha " + shortSHA(ch.NewSHA) + ")"
		} else if ch.Change == "removed" {
			detail += " (sha " + shortSHA(ch.OldSHA) + ")"
		}
		suggest := "核对是否为预期运维变更；非预期则回滚并排查入侵痕迹"
		if ch.Diff != "" {
			suggest = "已附带脱敏内容差异，请核对变更行是否符合变更单"
		}
		out = append(out, HostFinding{
			Level: level, Category: "fim",
			ID: "fim." + ch.Change + "." + shortDiscHash(ch.Path),
			Title: title + " — " + fimBaseName(ch.Path),
			Detail: detail, Suggest: suggest,
		})
	}
	return out
}

func shortSHA(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= 12 {
		return s
	}
	return s[:12]
}
