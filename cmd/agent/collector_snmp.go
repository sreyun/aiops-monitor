package main

// SNMP 轮询采集器（agent 侧）。主动 GET 系统组 + GETBULK 遍历 IF-MIB 接口表，
// 在两次轮询间算好速率/利用率当 gauge 上报。风格对齐 collector_redfish.go：
// 每 target 一个 goroutine、独立 ticker、连续失败退避、口令 env 优先。

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"aiops-monitor/shared"
)

// ----------------------------------------------------------------------------
// 配置（独立配置段，不塞进 NetFlowConfig.ActiveTarget）
// ----------------------------------------------------------------------------

// SNMPConfig 是 agent config.json 的 "snmp" 段。
type SNMPConfig struct {
	Targets            []SNMPTarget `json:"targets,omitempty"`
	DefaultIntervalSec int          `json:"default_interval_sec,omitempty"` // 默认 60
	// Trap 接收
	TrapEnabled     bool           `json:"trap_enabled,omitempty"`
	TrapListen      string         `json:"trap_listen,omitempty"`      // ":162"
	TrapCommunities []string       `json:"trap_communities,omitempty"` // v1/v2c：空=全收
	TrapUsers       []SNMPTrapUser `json:"trap_users,omitempty"`       // v3 trap/inform：按 userName 匹配来信做验签/解密
}

// SNMPTrapUser 是接收 v3 trap/inform 时用于验签/解密的 USM 用户。发送方(被管设备)是
// authoritative engine，密钥按来信里的 engineID 本地化，故这里只需口令与协议、无需 engineID。
type SNMPTrapUser struct {
	User        string `json:"user"`
	SecLevel    string `json:"sec_level,omitempty"` // noAuthNoPriv|authNoPriv|authPriv
	AuthProto   string `json:"auth_proto,omitempty"`
	AuthPass    string `json:"auth_pass,omitempty"`
	AuthPassEnv string `json:"auth_pass_env,omitempty"`
	PrivProto   string `json:"priv_proto,omitempty"`
	PrivPass    string `json:"priv_pass,omitempty"`
	PrivPassEnv string `json:"priv_pass_env,omitempty"`
}

func (u SNMPTrapUser) resolveAuthPass() string { return envOr(u.AuthPass, u.AuthPassEnv) }
func (u SNMPTrapUser) resolvePrivPass() string { return envOr(u.PrivPass, u.PrivPassEnv) }

// toUSMUser 把配置的 trap 用户转成 USM 运行时用户（secLevel 由显式配置或口令存在性推断）。
func (u SNMPTrapUser) toUSMUser() *usmUser {
	usr := &usmUser{
		name:      u.User,
		authProto: normalizeAuthProto(u.AuthProto),
		authPass:  []byte(u.resolveAuthPass()),
		privProto: normalizePrivProto(u.PrivProto),
		privPass:  []byte(u.resolvePrivPass()),
	}
	usr.secLevel = deriveSecLevel(u.SecLevel, usr)
	return usr
}

// SNMPTarget 是一个被轮询设备。
type SNMPTarget struct {
	Name    string `json:"name"`
	IP      string `json:"ip"`
	Port    int    `json:"port,omitempty"` // 默认 161
	Version string `json:"version"`        // "2c" | "3"

	// v2c
	Community    string `json:"community,omitempty"`
	CommunityEnv string `json:"community_env,omitempty"` // 优先，密文不落盘

	// v3 USM
	SecLevel    string `json:"sec_level,omitempty"` // noAuthNoPriv|authNoPriv|authPriv
	User        string `json:"user,omitempty"`
	AuthProto   string `json:"auth_proto,omitempty"` // MD5|SHA|SHA256
	AuthPass    string `json:"auth_pass,omitempty"`
	AuthPassEnv string `json:"auth_pass_env,omitempty"`
	PrivProto   string `json:"priv_proto,omitempty"` // DES|AES
	PrivPass    string `json:"priv_pass,omitempty"`
	PrivPassEnv string `json:"priv_pass_env,omitempty"`
	ContextName string `json:"context_name,omitempty"`

	// 轮询与采集范围
	IntervalSec    int   `json:"interval_sec,omitempty"`    // 默认取 DefaultIntervalSec
	TimeoutSec     int   `json:"timeout_sec,omitempty"`     // 默认 3
	Retries        int   `json:"retries,omitempty"`         // 默认 2
	MaxRepetitions int   `json:"max_repetitions,omitempty"` // GETBULK，默认 10
	Interfaces     []int `json:"interfaces,omitempty"`      // 指定 ifIndex，空=全部
	IncludeDown    bool  `json:"include_down,omitempty"`    // 是否采 admin-down 口（默认否，控噪+控基数）
	MaxInterfaces  int   `json:"max_interfaces,omitempty"`  // 上限，默认 200（呼应基数封顶哲学）
}

// envOr 返回 env 变量值（存在且非空时优先），否则回退直配值。
func envOr(direct, envName string) string {
	if envName != "" {
		if v := os.Getenv(envName); v != "" {
			return v
		}
	}
	return direct
}

func (t SNMPTarget) resolveCommunity() string {
	c := envOr(t.Community, t.CommunityEnv)
	if c == "" {
		c = "public"
	}
	return c
}
func (t SNMPTarget) resolveAuthPass() string { return envOr(t.AuthPass, t.AuthPassEnv) }
func (t SNMPTarget) resolvePrivPass() string { return envOr(t.PrivPass, t.PrivPassEnv) }

// ----------------------------------------------------------------------------
// OID 常量（已逐条校对）
// ----------------------------------------------------------------------------

var (
	oidSysDescr    = []uint32{1, 3, 6, 1, 2, 1, 1, 1, 0}
	oidSysObjectID = []uint32{1, 3, 6, 1, 2, 1, 1, 2, 0}
	oidSysUpTime   = []uint32{1, 3, 6, 1, 2, 1, 1, 3, 0}
	oidSysName     = []uint32{1, 3, 6, 1, 2, 1, 1, 5, 0}
	oidSysLocation = []uint32{1, 3, 6, 1, 2, 1, 1, 6, 0}

	// ifTable 列（1.3.6.1.2.1.2.2.1.C，尾 subid=ifIndex）
	colIfDescr       = []uint32{1, 3, 6, 1, 2, 1, 2, 2, 1, 2}
	colIfType        = []uint32{1, 3, 6, 1, 2, 1, 2, 2, 1, 3}
	colIfSpeed       = []uint32{1, 3, 6, 1, 2, 1, 2, 2, 1, 5}
	colIfPhysAddr    = []uint32{1, 3, 6, 1, 2, 1, 2, 2, 1, 6}
	colIfAdminStatus = []uint32{1, 3, 6, 1, 2, 1, 2, 2, 1, 7}
	colIfOperStatus  = []uint32{1, 3, 6, 1, 2, 1, 2, 2, 1, 8}
	colIfInOctets    = []uint32{1, 3, 6, 1, 2, 1, 2, 2, 1, 10}
	colIfInDiscards  = []uint32{1, 3, 6, 1, 2, 1, 2, 2, 1, 13}
	colIfInErrors    = []uint32{1, 3, 6, 1, 2, 1, 2, 2, 1, 14}
	colIfOutOctets   = []uint32{1, 3, 6, 1, 2, 1, 2, 2, 1, 16}
	colIfOutDiscards = []uint32{1, 3, 6, 1, 2, 1, 2, 2, 1, 19}
	colIfOutErrors   = []uint32{1, 3, 6, 1, 2, 1, 2, 2, 1, 20}

	// ifXTable 列（1.3.6.1.2.1.31.1.1.1.C，首选）
	colIfName          = []uint32{1, 3, 6, 1, 2, 1, 31, 1, 1, 1, 1}
	colIfHCInOctets    = []uint32{1, 3, 6, 1, 2, 1, 31, 1, 1, 1, 6}
	colIfHCInUcast     = []uint32{1, 3, 6, 1, 2, 1, 31, 1, 1, 1, 7}
	colIfHCOutOctets   = []uint32{1, 3, 6, 1, 2, 1, 31, 1, 1, 1, 10}
	colIfHCOutUcast    = []uint32{1, 3, 6, 1, 2, 1, 31, 1, 1, 1, 11}
	colIfHighSpeed     = []uint32{1, 3, 6, 1, 2, 1, 31, 1, 1, 1, 15}
	colIfAlias         = []uint32{1, 3, 6, 1, 2, 1, 31, 1, 1, 1, 18}
	colIfDiscontinuity = []uint32{1, 3, 6, 1, 2, 1, 31, 1, 1, 1, 19}
)

// interfaceColumns 是每轮 GETBULK 要遍历的全部列。
func interfaceColumns() [][]uint32 {
	return [][]uint32{
		colIfDescr, colIfType, colIfSpeed, colIfPhysAddr, colIfAdminStatus, colIfOperStatus,
		colIfName, colIfAlias, colIfHighSpeed, colIfDiscontinuity,
		colIfInOctets, colIfInErrors, colIfInDiscards,
		colIfOutOctets, colIfOutErrors, colIfOutDiscards,
		colIfHCInOctets, colIfHCInUcast, colIfHCOutOctets, colIfHCOutUcast,
	}
}

// ----------------------------------------------------------------------------
// exchanger：抽象 v2c/v3 的"发请求 PDU → 拿响应 pdu"，让表遍历复用两版本。
// ----------------------------------------------------------------------------

type exchanger interface {
	get(oids [][]uint32) (pdu, error)
	getBulk(nonRep, maxRep int, oids [][]uint32) (pdu, error)
	close()
}

// snmpExchange 发一个数据报并读一个响应，带重试。用 connected UDP，只收对端回包。
func snmpExchange(conn net.Conn, timeout time.Duration, retries int, req []byte) ([]byte, error) {
	buf := make([]byte, 65535)
	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		if _, err := conn.Write(req); err != nil {
			lastErr = err
			continue
		}
		_ = conn.SetReadDeadline(time.Now().Add(timeout))
		n, err := conn.Read(buf)
		if err != nil {
			lastErr = err
			continue
		}
		out := make([]byte, n)
		copy(out, buf[:n])
		return out, nil
	}
	if lastErr == nil {
		lastErr = errors.New("snmp: 无响应")
	}
	return nil, lastErr
}

// v2cExchanger 在一条 UDP conn 上实现 SNMPv2c 的 exchanger。
type v2cExchanger struct {
	conn      net.Conn
	community string
	timeout   time.Duration
	retries   int
}

func (e *v2cExchanger) do(pduBytes []byte, reqID int32) (pdu, error) {
	msg := buildV2CMessage(e.community, pduBytes)
	resp, err := snmpExchange(e.conn, e.timeout, e.retries, msg)
	if err != nil {
		return pdu{}, err
	}
	_, _, p, err := parseV2CMessage(resp)
	if err != nil {
		return pdu{}, err
	}
	if p.requestID != reqID {
		return pdu{}, fmt.Errorf("snmp: request-id 不匹配(want %d got %d)", reqID, p.requestID)
	}
	return p, nil
}

func (e *v2cExchanger) get(oids [][]uint32) (pdu, error) {
	reqID := nextRequestID()
	return e.do(buildGet(reqID, oids), reqID)
}

func (e *v2cExchanger) getBulk(nonRep, maxRep int, oids [][]uint32) (pdu, error) {
	reqID := nextRequestID()
	return e.do(buildGetBulk(reqID, nonRep, maxRep, oids), reqID)
}

func (e *v2cExchanger) close() {
	if e.conn != nil {
		_ = e.conn.Close()
	}
}

// ----------------------------------------------------------------------------
// GETBULK 表遍历
// ----------------------------------------------------------------------------

// walkColumns 用反复 GETBULK 遍历一组表列，返回 ifIndex → (列基址字符串 → 值)。
// 每列在返回 OID 离开该列子树 / endOfMibView / 无进展时停止。
func walkColumns(x exchanger, cols [][]uint32, maxRep int) (map[uint32]map[string]snmpValue, error) {
	result := map[uint32]map[string]snmpValue{}
	if maxRep <= 0 {
		maxRep = 10
	}
	starts := make([][]uint32, len(cols))
	copy(starts, cols)
	active := make([]bool, len(cols))
	for i := range cols {
		active[i] = true
	}

	const maxRounds = 512 // 防死循环兜底
	for round := 0; round < maxRounds && anyTrue(active); round++ {
		var reqOIDs [][]uint32
		var idxMap []int
		for i := range cols {
			if active[i] {
				reqOIDs = append(reqOIDs, starts[i])
				idxMap = append(idxMap, i)
			}
		}
		ncols := len(reqOIDs)
		if ncols == 0 {
			break
		}
		p, err := x.getBulk(0, maxRep, reqOIDs)
		if err != nil {
			return result, err
		}
		if p.errStatus == 1 { // tooBig：减小 maxRep 重试本轮
			if maxRep > 1 {
				maxRep /= 2
				continue
			}
			return result, errors.New("snmp: GETBULK tooBig 且 maxRep 已至 1")
		}
		if p.errStatus != 0 {
			return result, fmt.Errorf("snmp: GETBULK error-status %d", p.errStatus)
		}
		// GETBULK 响应按 repetition-major 排布：位置 vi 的列 = vi % ncols。
		progressed := make([]bool, len(cols))
		for vi, vb := range p.varbinds {
			ci := idxMap[vi%ncols]
			if !active[ci] {
				continue
			}
			if vb.value.Exc == tagEndOfMibView || !oidHasPrefix(vb.oid, cols[ci]) {
				active[ci] = false
				continue
			}
			ifIndex := vb.oid[len(vb.oid)-1] // IF-MIB 单索引：尾 subid=ifIndex
			if result[ifIndex] == nil {
				result[ifIndex] = map[string]snmpValue{}
			}
			result[ifIndex][oidToString(cols[ci])] = vb.value
			starts[ci] = vb.oid
			progressed[ci] = true
		}
		for i := range cols {
			if active[i] && !progressed[i] {
				active[i] = false // 本轮零进展 → 停，防死循环
			}
		}
	}
	return result, nil
}

func anyTrue(b []bool) bool {
	for _, v := range b {
		if v {
			return true
		}
	}
	return false
}

// ----------------------------------------------------------------------------
// 速率计算
// ----------------------------------------------------------------------------

type ifCounterSample struct {
	ts            time.Time
	inOctets      uint64
	outOctets     uint64
	inUcast       uint64
	outUcast      uint64
	inErrors      uint64
	outErrors     uint64
	inDiscards    uint64
	outDiscards   uint64
	is64          bool
	discontinuity uint64
}

type ifRates struct {
	inBps, outBps         float64
	inPps, outPps         float64
	inErrPps, outErrPps   float64
	inDiscPps, outDiscPps float64
	inUtil, outUtil       float64
	valid                 bool
}

// counterDelta 返回两次计数器读数之差，处理 32 位回绕。ok=false 表示疑似计数器复位
// （本轮 delta 不可信，应丢弃）。speedBps>0 时对回绕做速率合理性校验。
func counterDelta(prev, cur uint64, is64 bool, elapsed float64, speedBps uint64) (uint64, bool) {
	if cur >= prev {
		return cur - prev, true
	}
	// cur < prev
	if is64 {
		return 0, false // 64 位计数器在任何现实间隔内不会回绕 → 判复位
	}
	d := (uint64(1) << 32) - prev + cur // 疑似 32 位回绕
	if speedBps > 0 && elapsed > 0 {
		// 换算 bps 超过链路速率的 1.5 倍 → 不是回绕而是复位
		if float64(d)*8/elapsed > float64(speedBps)*1.5 {
			return 0, false
		}
	}
	return d, true
}

// computeRates 用两次采样算出各项速率与利用率。检测到复位/间隔非法时 valid=false。
func computeRates(prev, cur ifCounterSample, speedBps uint64) ifRates {
	var r ifRates
	elapsed := cur.ts.Sub(prev.ts).Seconds()
	if elapsed <= 0 {
		return r
	}
	if cur.discontinuity != prev.discontinuity { // 计数器不连续（设备复位/热插拔）
		return r
	}
	inB, ok1 := counterDelta(prev.inOctets, cur.inOctets, cur.is64, elapsed, speedBps)
	outB, ok2 := counterDelta(prev.outOctets, cur.outOctets, cur.is64, elapsed, speedBps)
	if !ok1 || !ok2 {
		return r
	}
	r.inBps = float64(inB) * 8 / elapsed
	r.outBps = float64(outB) * 8 / elapsed
	if d, ok := counterDelta(prev.inUcast, cur.inUcast, cur.is64, elapsed, 0); ok {
		r.inPps = float64(d) / elapsed
	}
	if d, ok := counterDelta(prev.outUcast, cur.outUcast, cur.is64, elapsed, 0); ok {
		r.outPps = float64(d) / elapsed
	}
	if d, ok := counterDelta(prev.inErrors, cur.inErrors, false, elapsed, 0); ok {
		r.inErrPps = float64(d) / elapsed
	}
	if d, ok := counterDelta(prev.outErrors, cur.outErrors, false, elapsed, 0); ok {
		r.outErrPps = float64(d) / elapsed
	}
	if d, ok := counterDelta(prev.inDiscards, cur.inDiscards, false, elapsed, 0); ok {
		r.inDiscPps = float64(d) / elapsed
	}
	if d, ok := counterDelta(prev.outDiscards, cur.outDiscards, false, elapsed, 0); ok {
		r.outDiscPps = float64(d) / elapsed
	}
	if speedBps > 0 {
		r.inUtil = r.inBps / float64(speedBps) * 100
		r.outUtil = r.outBps / float64(speedBps) * 100
	}
	r.valid = true
	return r
}

// ----------------------------------------------------------------------------
// 采集器
// ----------------------------------------------------------------------------

type snmpCollector struct {
	cfg    SNMPConfig
	hostID string
	fp     string

	mu   sync.Mutex
	prev map[string]map[uint32]ifCounterSample // targetName → ifIndex → 上次采样
}

func newSNMPCollector(cfg SNMPConfig, hostID, fp string) *snmpCollector {
	return &snmpCollector{
		cfg:    cfg,
		hostID: hostID,
		fp:     fp,
		prev:   map[string]map[uint32]ifCounterSample{},
	}
}

// run 每 target 一个 goroutine，仿 redfishCollector.run。
func (sc *snmpCollector) run(reporter func(shared.SNMPReport)) {
	for _, t := range sc.cfg.Targets {
		go sc.pollLoop(t, reporter)
	}
}

func (sc *snmpCollector) resolveInterval(t SNMPTarget) time.Duration {
	sec := t.IntervalSec
	if sec <= 0 {
		sec = sc.cfg.DefaultIntervalSec
	}
	if sec <= 0 {
		sec = 60
	}
	iv := time.Duration(sec) * time.Second
	if iv < 10*time.Second { // 下界：SNMP 轮询太密无意义且压设备
		iv = 10 * time.Second
	}
	return iv
}

func (sc *snmpCollector) pollLoop(t SNMPTarget, reporter func(shared.SNMPReport)) {
	interval := sc.resolveInterval(t)
	slog.Info("SNMP 轮询采集器启动", "target", t.Name, "ip", t.IP, "version", t.Version, "interval", interval)
	failCount := 0
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	sc.pollAndReport(t, reporter, &failCount)
	for range ticker.C {
		sc.pollAndReport(t, reporter, &failCount)
	}
}

func (sc *snmpCollector) pollAndReport(t SNMPTarget, reporter func(shared.SNMPReport), failCount *int) {
	snap := sc.collectOne(t)
	if snap.Error != "" {
		*failCount++
		slog.Warn("SNMP 采集失败", "target", t.Name, "err", snap.Error, "consecutive", *failCount)
	} else {
		*failCount = 0
		slog.Info("SNMP 采集成功", "target", t.Name, "sys", snap.System.Name, "if", len(snap.Interfaces))
	}
	reporter(shared.SNMPReport{
		HostID:      sc.hostID,
		Fingerprint: sc.fp,
		Timestamp:   time.Now().Unix(),
		Snapshots:   []shared.SNMPSnapshot{snap},
	})
	if *failCount >= 3 { // 连续失败退避 5 分钟（仿 Redfish）
		slog.Error("SNMP 连续失败，退避 5 分钟", "target", t.Name)
		time.Sleep(5 * time.Minute)
		*failCount = 0
	}
}

// collectOne 采集一台设备一轮：系统组 + 接口表。
func (sc *snmpCollector) collectOne(t SNMPTarget) shared.SNMPSnapshot {
	now := time.Now()
	snap := shared.SNMPSnapshot{
		TargetName:  t.Name,
		TargetIP:    t.IP,
		Timestamp:   now.Unix(),
		Version:     t.Version,
		IntervalSec: int(sc.resolveInterval(t) / time.Second),
	}
	x, err := sc.newExchanger(t)
	if err != nil {
		snap.Error = err.Error()
		return snap
	}
	defer x.close()

	sys, err := sc.pollSystem(x)
	if err != nil {
		snap.Error = "系统组采集失败: " + err.Error()
		return snap
	}
	snap.System = sys

	ifs, err := sc.pollInterfaces(t, x, now)
	if err != nil {
		snap.Error = "接口表采集失败: " + err.Error()
		return snap
	}
	snap.Interfaces = ifs
	snap.Reachable = true
	return snap
}

func (sc *snmpCollector) newExchanger(t SNMPTarget) (exchanger, error) {
	port := t.Port
	if port == 0 {
		port = 161
	}
	addr := net.JoinHostPort(t.IP, strconv.Itoa(port))
	conn, err := net.Dial("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	timeout := time.Duration(t.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	retries := t.Retries
	if retries <= 0 {
		retries = 2
	}
	switch t.Version {
	case "3":
		return newV3Exchanger(conn, t, timeout, retries)
	default: // "2c" 及其它
		return &v2cExchanger{conn: conn, community: t.resolveCommunity(), timeout: timeout, retries: retries}, nil
	}
}

func (sc *snmpCollector) pollSystem(x exchanger) (shared.SNMPSystem, error) {
	oids := [][]uint32{oidSysDescr, oidSysObjectID, oidSysUpTime, oidSysName, oidSysLocation}
	p, err := x.get(oids)
	if err != nil {
		return shared.SNMPSystem{}, err
	}
	var sys shared.SNMPSystem
	for _, vb := range p.varbinds {
		if vb.value.Exc != 0 {
			continue
		}
		switch {
		case oidEqual(vb.oid, oidSysDescr):
			sys.Descr = vb.value.String()
		case oidEqual(vb.oid, oidSysObjectID):
			sys.ObjectID = vb.value.String()
		case oidEqual(vb.oid, oidSysUpTime):
			sys.UptimeSec = float64(vb.value.Uint) / 100
		case oidEqual(vb.oid, oidSysName):
			sys.Name = vb.value.String()
		case oidEqual(vb.oid, oidSysLocation):
			sys.Location = vb.value.String()
		}
	}
	return sys, nil
}

func (sc *snmpCollector) pollInterfaces(t SNMPTarget, x exchanger, now time.Time) ([]shared.SNMPInterface, error) {
	maxRep := t.MaxRepetitions
	if maxRep <= 0 {
		maxRep = 10
	}
	table, err := walkColumns(x, interfaceColumns(), maxRep)
	if err != nil {
		return nil, err
	}
	indices := make([]uint32, 0, len(table))
	for idx := range table {
		indices = append(indices, idx)
	}
	sort.Slice(indices, func(i, j int) bool { return indices[i] < indices[j] })

	maxIf := t.MaxInterfaces
	if maxIf <= 0 {
		maxIf = 200
	}

	var out []shared.SNMPInterface
	for _, idx := range indices {
		if len(t.Interfaces) > 0 && !containsInt(t.Interfaces, int(idx)) {
			continue
		}
		iface := sc.buildInterface(t, idx, table[idx], now)
		if !t.IncludeDown && iface.AdminStatus == 2 { // admin-down 默认不采（控噪+控基数）
			continue
		}
		out = append(out, iface)
		if len(out) >= maxIf {
			slog.Warn("SNMP 接口数超过上限，截断", "target", t.Name, "max", maxIf)
			break
		}
	}
	sc.prunePrev(t.Name, table)
	return out, nil
}

// pickCounter 优先取 HC(64 位)计数器，无则回退 32 位 ifTable 计数器。
func pickCounter(row map[string]snmpValue, hcCol, stdCol []uint32) (val uint64, is64 bool) {
	if hcCol != nil {
		if v, ok := row[oidToString(hcCol)]; ok && v.Exc == 0 && v.Tag == tagCounter64 {
			return v.Uint, true
		}
	}
	if stdCol != nil {
		if v, ok := row[oidToString(stdCol)]; ok && v.Exc == 0 {
			return v.Uint, false
		}
	}
	return 0, false
}

func (sc *snmpCollector) buildInterface(t SNMPTarget, idx uint32, row map[string]snmpValue, now time.Time) shared.SNMPInterface {
	get := func(col []uint32) (snmpValue, bool) {
		v, ok := row[oidToString(col)]
		return v, ok && v.Exc == 0
	}
	iface := shared.SNMPInterface{Index: idx}
	if v, ok := get(colIfName); ok {
		iface.Name = v.String()
	}
	if v, ok := get(colIfDescr); ok {
		iface.Descr = v.String()
	}
	if iface.Name == "" {
		iface.Name = iface.Descr
	}
	if iface.Name == "" {
		iface.Name = fmt.Sprintf("if%d", idx)
	}
	if v, ok := get(colIfAlias); ok {
		iface.Alias = v.String()
	}
	if v, ok := get(colIfType); ok {
		iface.Type = int(v.Int)
	}
	if v, ok := get(colIfPhysAddr); ok && len(v.Bytes) == 6 {
		iface.MAC = fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x",
			v.Bytes[0], v.Bytes[1], v.Bytes[2], v.Bytes[3], v.Bytes[4], v.Bytes[5])
	}
	if v, ok := get(colIfAdminStatus); ok {
		iface.AdminStatus = int(v.Int)
	}
	if v, ok := get(colIfOperStatus); ok {
		iface.OperStatus = int(v.Int)
		iface.OperUp = v.Int == 1
	}
	// 速率：ifHighSpeed(Mbps) 优先，否则 ifSpeed(bps)
	var speedBps uint64
	if v, ok := get(colIfHighSpeed); ok && v.Uint > 0 {
		speedBps = v.Uint * 1_000_000
	} else if v, ok := get(colIfSpeed); ok {
		speedBps = v.Uint
	}
	iface.SpeedBps = speedBps

	// 计数器：优先 HC 64 位
	sample := ifCounterSample{ts: now}
	inO, in64 := pickCounter(row, colIfHCInOctets, colIfInOctets)
	outO, out64 := pickCounter(row, colIfHCOutOctets, colIfOutOctets)
	sample.inOctets, sample.outOctets = inO, outO
	sample.is64 = in64 && out64
	iface.Counter64 = sample.is64
	iface.InOctets, iface.OutOctets = inO, outO
	sample.inUcast, _ = pickCounter(row, colIfHCInUcast, nil)
	sample.outUcast, _ = pickCounter(row, colIfHCOutUcast, nil)
	if v, ok := get(colIfInErrors); ok {
		sample.inErrors = v.Uint
		iface.InErrors = v.Uint
	}
	if v, ok := get(colIfOutErrors); ok {
		sample.outErrors = v.Uint
		iface.OutErrors = v.Uint
	}
	if v, ok := get(colIfInDiscards); ok {
		sample.inDiscards = v.Uint
	}
	if v, ok := get(colIfOutDiscards); ok {
		sample.outDiscards = v.Uint
	}
	if v, ok := get(colIfDiscontinuity); ok {
		sample.discontinuity = v.Uint
	}

	// 与上次采样算速率
	sc.mu.Lock()
	if sc.prev[t.Name] == nil {
		sc.prev[t.Name] = map[uint32]ifCounterSample{}
	}
	prev, hasPrev := sc.prev[t.Name][idx]
	sc.prev[t.Name][idx] = sample
	sc.mu.Unlock()

	if hasPrev {
		r := computeRates(prev, sample, speedBps)
		if r.valid {
			iface.InBps, iface.OutBps = r.inBps, r.outBps
			iface.InPps, iface.OutPps = r.inPps, r.outPps
			iface.InErrPps, iface.OutErrPps = r.inErrPps, r.outErrPps
			iface.InDiscardPps, iface.OutDiscardPps = r.inDiscPps, r.outDiscPps
			iface.InUtilPercent, iface.OutUtilPercent = r.inUtil, r.outUtil
			iface.RateValid = true
		}
	}
	return iface
}

// prunePrev 清掉本轮已不存在的接口的上次采样，避免内存无界增长。
func (sc *snmpCollector) prunePrev(targetName string, table map[uint32]map[string]snmpValue) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	m := sc.prev[targetName]
	if m == nil {
		return
	}
	for idx := range m {
		if _, ok := table[idx]; !ok {
			delete(m, idx)
		}
	}
}

func containsInt(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
