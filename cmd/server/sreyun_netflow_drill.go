package main

import (
	"fmt"
	"strings"
	"time"
)

// sreyunProtoName 把 IP 协议号转可读名（NetFlow 明细展示用）。
func sreyunProtoName(p int) string {
	switch p {
	case 1:
		return "ICMP"
	case 6:
		return "TCP"
	case 17:
		return "UDP"
	default:
		return fmt.Sprintf("proto%d", p)
	}
}

// sreyunFlowTS 格式化 Flow 明细里的时间字段（any→time.Time）。
func sreyunFlowTS(v any) string {
	if t, ok := v.(time.Time); ok && !t.IsZero() {
		return t.Format("01-02 15:04:05")
	}
	return "-"
}

// execQueryNetFlowFlows 钻取主机的原始 Flow 明细（单条连接），可按 src_ip/dst_ip/src_port/
// dst_port/protocol 过滤。配合 query_netflow（Top-N 聚合）实现「先看谁占带宽/异常外联，再下钻到
// 具体是哪些连接」的追查闭环。只读平台自有 flow_records，不触达被控主机。
func (h *SreyunCore) execQueryNetFlowFlows(args map[string]any) (string, error) {
	ref, _ := args["host_id"].(string)
	if strings.TrimSpace(ref) == "" {
		return "请指定 host_id", nil
	}
	hostID, name := ref, ref
	if hst := h.resolveHostRef(ref); hst != nil {
		hostID, name = hst.ID, hst.Hostname
	}
	if h.s.pg == nil {
		return "未配置 PostgreSQL，无法查询流量明细。", nil
	}
	filter, _ := args["filter"].(string)
	filter = strings.TrimSpace(filter)
	limit := 50
	if n, ok := args["limit"].(float64); ok && n > 0 {
		limit = int(n)
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := h.s.pg.getFlowRecords(hostID, filter, limit)
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		if filter != "" {
			return fmt.Sprintf("主机 %s 没有匹配过滤条件 %q 的流记录。", name, filter), nil
		}
		return fmt.Sprintf("主机 %s 暂无流记录（未配置 NetFlow/抓包采集，或该时段无流量）。", name), nil
	}
	var b strings.Builder
	if filter != "" {
		fmt.Fprintf(&b, "主机 %s 匹配 %q 的流明细（%d 条）:\n", name, filter, len(rows))
	} else {
		fmt.Fprintf(&b, "主机 %s 最近 %d 条流明细:\n", name, len(rows))
	}
	for i, r := range rows {
		proto := 0
		if p, ok := r["protocol"].(int); ok {
			proto = p
		}
		fmt.Fprintf(&b, "  %d. %v:%v → %v:%v %s %s（%v 包，末次 %s）\n",
			i+1, r["src_ip"], r["src_port"], r["dst_ip"], r["dst_port"],
			sreyunProtoName(proto), humanBytes(toInt64(r["bytes"])), r["packets"], sreyunFlowTS(r["last_seen"]))
	}
	return b.String(), nil
}
