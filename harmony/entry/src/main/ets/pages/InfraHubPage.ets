import { pasteboard } from '@kit.BasicServicesKit';
import { Api } from '../data/Api';
import { ApiClient } from '../data/ApiClient';
import { FormatUtil } from '../common/FormatUtil';
import {
  ApiEndpoint,
  ApiMonitorSystem,
  CheckPoint,
  CheckUpsertRequest,
  ContentAuditEvent,
  DataHost,
  FanReading,
  GpuInfo,
  HardwareEvent,
  HardwareSnapshot,
  HardwareSnapshotWrap,
  Host,
  HyperVGuest,
  HyperVInventory,
  MemoryDIMM,
  NetFlowDrillItem,
  NetFlowHistoryPoint,
  NetFlowIPHistoryResponse,
  NetFlowItem,
  NetFlowSummaryItem,
  Probe,
  PromRangeResult,
  PSUReading,
  RedfishCPU,
  RedfishGPU,
  RedfishRAID,
  RedfishStorage,
  SensorReading,
  SNMPDevice,
  SNMPInterface,
  SNMPInterfaceHistoryResponse,
  SNMPTrap,
  SNMPVarbind
} from '../data/models/Models';
import {
  ConfirmDialogHost,
  EmptyHint,
  ErrorBanner,
  FilterChips,
  HostCard,
  KeyValueRow,
  MetricBar,
  PageHeader,
  SearchBar,
  StatusDot
} from '../components/CommonUI';
import { ChartPoint, HistoryChart } from '../components/HistoryChart';
import { AiAssistSheet, DetailSheet, ToastController, ToastHost } from '../components/InteractionUI';
import { AppSymbol } from '../components/AppIcons';
import { AppTheme } from '../common/Routes';


interface GpuRankRow {
  hostId: string;
  hostname: string;
  gpuName: string;
  util: number;
  mem: number;
  temp: number;
}

interface BadPartRow {
  kind: string;
  name: string;
  health: string;
}

@Component
export struct InfraHubTab {
  @StorageLink('aiops_theme_rev') themeRev: number = 0;

  @StorageLink('aiops_bg') tBg: string = '#080C14';
  @StorageLink('aiops_card') tCard: string = '#111722';
  @StorageLink('aiops_elevated') tElevated: string = '#192130';
  @StorageLink('aiops_text') tText: string = '#E6E8F0';
  @StorageLink('aiops_muted') tMuted: string = '#9AA3B8';
  @StorageLink('aiops_accent') tAccent: string = '#4F7FFF';
  @StorageLink('aiops_accent_soft') tAccentSoft: string = '#1E3166';
  @StorageLink('aiops_accent_on') tAccentOn: string = '#D6E2FF';
  @StorageLink('aiops_success') tSuccess: string = '#00A86B';
  @StorageLink('aiops_success_soft') tSuccessSoft: string = '#0F3A2C';
  @StorageLink('aiops_warning') tWarning: string = '#F59E0B';
  @StorageLink('aiops_warning_soft') tWarningSoft: string = '#3A2A10';
  @StorageLink('aiops_danger') tDanger: string = '#EF4444';
  @StorageLink('aiops_danger_soft') tDangerSoft: string = '#3A1614';
  @StorageLink('aiops_border') tBorder: string = '#39445A';
  @StorageLink('aiops_border_soft') tBorderSoft: string = '#222C3D';
  @StorageLink('aiops_on_accent') tOnAccent: string = '#FFFFFF';
  @StorageLink('aiops_nav_bar') tNavBar: string = '#111722';
  @StorageLink('aiops_scrim') tScrim: string = '#99000000';
  @StorageLink('aiops_shadow') tShadow: string = '#40000000';

  @Prop @Watch('onInitialTab') initialTab: number = 0;
  @Prop @Watch('onInitialRes') initialRes: number = 0;
  onOpenHost: (hostId: string) => void = (_hostId: string) => {};

  @State tab: number = 0;
  @State resSub: number = 0;
  @State netSub: number = 0;
  @State hostQuickFilter: number = 0; // 0 全部 / 1 仅在线
  @State selectedCategory: string = '';
  @State probeTypeIdx: number = 0;
  @State hostQuery: string = '';
  @State hwQuery: string = '';
  @State hvQuery: string = '';
  @State hvFilter: number = 1; // 0 全部 / 1 运行中 / 2 非运行（Android 默认运行中）
  @State selectedHost: string = '';
  @State hosts: Host[] = [];
  @State probes: Probe[] = [];
  @State hvInventories: HyperVInventory[] = [];
  @State snmpDevices: SNMPDevice[] = [];
  @State netflowItems: NetFlowSummaryItem[] = [];
  @State netflowFlows: NetFlowItem[] = [];
  @State traps: SNMPTrap[] = [];
  @State audits: ContentAuditEvent[] = [];
  @State hwSnapshots: HardwareSnapshotWrap[] = [];
  @State hwFocus: HardwareSnapshotWrap | null = null;
  @State hwSheetVisible: boolean = false;
  @State netDimIdx: number = 0;
  @State netRangeIdx: number = 0;
  @State dataHostsSnmp: DataHost[] = [];
  @State dataHostsNetflow: DataHost[] = [];
  @State dataHostsAudit: DataHost[] = [];
  @State showAllHosts: boolean = false;
  @State auditQuery: string = '';
  @State probeSeg: number = 0;
  @State apiSystems: ApiMonitorSystem[] = [];
  @State probeOverviewIdx: number = 0;
  @State snmpExpandKey: string = '';
  @State ifaceFilter: number = 1; // Android 默认只看在线接口
  @State snmpHistVisible: boolean = false;
  @State snmpHistTitle: string = '';
  @State snmpHistText: string = '';
  @State snmpHistResp: SNMPInterfaceHistoryResponse | null = null;
  @State snmpHistInPts: ChartPoint[] = [];
  @State snmpHistOutPts: ChartPoint[] = [];
  @State snmpHistUtilInPts: ChartPoint[] = [];
  @State snmpHistUtilOutPts: ChartPoint[] = [];
  @State snmpFocusDevice: string = '';
  @State snmpFocusIface: string = '';
  @State snmpHistDevice: SNMPDevice | null = null;
  @State snmpHistIface: SNMPInterface | null = null;
  @State snmpHistRangeIdx: number = 0;
  @State ipHistPts: ChartPoint[] = [];
  @State ipHistVisible: boolean = false;
  @State ipHistTitle: string = '';
  @State ipHist: NetFlowIPHistoryResponse | null = null;
  @State aiAssistVisible: boolean = false;
  @State aiTask: string = 'assist';
  @State aiContext: string = '';
  @State aiTitle: string = 'AI 助手';
  @State aiSeed: string = '';
  @State vmSheetVisible: boolean = false;
  @State vmFocus: HyperVGuest | null = null;
  @State vmFocusHost: string = '';
  @State hvExportVisible: boolean = false;
  @State probeFormVisible: boolean = false;
  @State probeEditId: string = '';
  @State probeName: string = '';
  @State probeTarget: string = '';
  @State probeTypeSel: number = 0;
  @State probeInterval: number = 60;
  @State probeDnsType: string = 'A';
  @State probeBusy: boolean = false;
  @State probeDeleteVisible: boolean = false;
  @State probeDeleteId: string = '';
  @State apiDeleteVisible: boolean = false;
  @State apiDeleteId: string = '';
  @State histVisible: boolean = false;
  @State histTitle: string = '';
  @State histPoints: CheckPoint[] = [];
  @State histRangeIdx: number = 2; // default 3h ~ 180min aligned with prior load
  @State histSourceKind: string = ''; // probe | api
  @State histSourceId: string = '';
  @State probeExpect: string = '';
  @State probeTimeout: number = 5;
  @State probeEnabled: boolean = true;
  @State probeLevel: string = 'critical';
  @State isAdmin: boolean = false;
  @State dnsThVisible: boolean = false;
  @State dnsWarnMs: string = '500';
  @State dnsCritMs: string = '2000';
  @State dnsThBusy: boolean = false;
  @State ipHistIp: string = '';
  @State testingProbeIds: string[] = [];
  @State testingApiIds: string[] = [];
  @State toastMsg: string = '';
  @State toastError: boolean = false;
  @State error: string = '';
  private pollId: number = -1;

  private tabLabels: string[] = ['主机', '资源', '网络', '拨测'];
  private tabSubtitles: string[] = [
    'Agent 指标 · 资源利用率',
    '物理机 · 虚拟机',
    '网络设备 · 流量 · Trap · 内容审计',
    '拨测 · API · 证书'
  ];
  private resLabels: string[] = ['物理机', '虚拟机'];
  private netLabels: string[] = ['设备', '流量', 'Trap', '内容审计'];
  private probeTypesAll: string[] = ['http', 'tcp', 'ping', 'udp', 'dns', 'process'];
  private probeOverviewLabels: string[] = ['全部对象', '正常', '异常', '已探测', 'HTTPS 证书'];
  private ifaceFilterLabels: string[] = ['全部', '在线', '离线'];
  private hvFilterLabels: string[] = ['全部', '运行中', '非运行'];
  private nfDims: string[] = ['dst_ip', 'src_ip', 'dst_port', 'src_port', 'protocol'];
  private nfDimLabels: string[] = ['目的IP', '源IP', '目的端口', '源端口', '协议'];
  private nfRanges: string[] = ['1h', '3h', '6h', '12h', '24h', '3d', '7d', '14d'];
  private nfRangeLabels: string[] = ['1小时', '3小时', '6小时', '12小时', '1天', '3天', '7天', '14天'];
  private snmpHistRanges: string[] = ['1h', '3h', '6h', '12h', '24h'];
  private snmpHistRangeLabels: string[] = ['1小时', '3小时', '6小时', '12小时', '1天'];
  private histRangesMin: number[] = [60, 180, 360, 720, 1440];
  private histRangeLabels: string[] = ['1小时', '3小时', '6小时', '12小时', '1天'];
  private dnsTypes: string[] = ['A', 'AAAA', 'CNAME', 'MX', 'TXT', 'NS'];
  @State loadingHosts: boolean = false;

  onInitialTab(): void {
    const t = Number(this.initialTab);
    if (t >= 0 && t <= 3) {
      this.tab = t;
    }
    void this.refresh();
  }

  onInitialRes(): void {
    const r = Number(this.initialRes);
    if (r === 0 || r === 1) {
      this.resSub = r;
    }
    void this.refresh();
  }

  aboutToAppear(): void {
    const t = Number(this.initialTab);
    if (t >= 0 && t <= 3) {
      this.tab = t;
    }
    const r = Number(this.initialRes);
    if (r === 0 || r === 1) {
      this.resSub = r;
    }
    void this.refreshMe();
    void this.refresh();
    this.pollId = setInterval(() => {
      if (ApiClient.isSessionAlive()) {
        void this.refresh();
      }
    }, 10000);
  }

  aboutToDisappear(): void {
    if (this.pollId >= 0) {
      clearInterval(this.pollId);
    }
  }

  private hostCategories(): string[] {
    const out: string[] = [];
    for (let i = 0; i < this.hosts.length; i++) {
      const c = (this.hosts[i].category || '').trim();
      if (c && out.indexOf(c) < 0) {
        out.push(c);
      }
    }
    return out.sort();
  }

  private categoryCount(cat: string): number {
    let n = 0;
    for (let i = 0; i < this.hosts.length; i++) {
      if ((this.hosts[i].category || '') === cat) {
        n++;
      }
    }
    return n;
  }

  private onlineCount(): number {
    let n = 0;
    for (let i = 0; i < this.hosts.length; i++) {
      if (this.hosts[i].online) {
        n++;
      }
    }
    return n;
  }

  private clearHostFilters(): void {
    this.hostQuery = '';
    this.selectedCategory = '';
    this.hostQuickFilter = 0;
  }

  private filteredHosts(): Host[] {
    let list = this.hosts;
    if (this.hostQuickFilter === 1) {
      list = list.filter((h: Host) => !!h.online);
    }
    if (this.selectedCategory) {
      const want = this.selectedCategory;
      list = list.filter((h: Host) => (h.category || '') === want);
    }
    const q = this.hostQuery.trim().toLowerCase();
    if (q) {
      list = list.filter((h: Host) => {
        const name = (h.hostname || '').toLowerCase();
        const ip = (h.ip || '').toLowerCase();
        return name.indexOf(q) >= 0 || ip.indexOf(q) >= 0 || h.id.toLowerCase().indexOf(q) >= 0;
      });
    }
    return list.slice().sort((a: Host, b: Host) => {
      const ao = a.online ? 0 : 1;
      const bo = b.online ? 0 : 1;
      if (ao !== bo) {
        return ao - bo;
      }
      return (a.hostname || a.id).localeCompare(b.hostname || b.id);
    });
  }

  private dataHostId(dh: DataHost): string {
    return (dh.host_id || dh.id || '').trim();
  }

  private currentDataHosts(): DataHost[] {
    if (this.netSub === 0 || this.netSub === 2) {
      return this.dataHostsSnmp;
    }
    if (this.netSub === 1) {
      return this.dataHostsNetflow;
    }
    return this.dataHostsAudit;
  }

  private netPickerHosts(): Host[] {
    if (this.showAllHosts) {
      return this.hosts;
    }
    const dh = this.currentDataHosts();
    if (dh.length === 0) {
      return this.hosts;
    }
    const ids: string[] = [];
    for (let i = 0; i < dh.length; i++) {
      const id = this.dataHostId(dh[i]);
      if (id && ids.indexOf(id) < 0) {
        ids.push(id);
      }
    }
    const filtered = this.hosts.filter((h: Host) => ids.indexOf(h.id) >= 0);
    return filtered.length > 0 ? filtered : this.hosts;
  }

  private preferSelectedFromDataHosts(): void {
    if (this.showAllHosts) {
      return;
    }
    const pool = this.currentDataHosts();
    if (pool.length === 0) {
      return;
    }
    const ids: string[] = [];
    for (let i = 0; i < pool.length; i++) {
      const id = this.dataHostId(pool[i]);
      if (id) {
        ids.push(id);
      }
    }
    if (ids.length === 0) {
      return;
    }
    if (ids.indexOf(this.selectedHost) < 0) {
      this.selectedHost = ids[0];
    }
  }

  private probeLatency(p: Probe): number {
    return Number(p.latency_ms ?? p.last_latency_ms ?? 0);
  }

  private probeStatusText(p: Probe): string {
    if (p.enabled === false) {
      return '已停用';
    }
    if (!(p.checked_at && p.checked_at > 0) && p.ok === undefined) {
      return '未探测';
    }
    if ((p.checked_at ?? 0) <= 0 && p.ok !== true && p.ok !== false) {
      return '未探测';
    }
    if (p.ok === true) {
      return '正常';
    }
    if (p.ok === false) {
      return p.message || '异常';
    }
    return FormatUtil.healthZh(p.last_status || p.message || '—');
  }

  private probeOk(p: Probe): boolean {
    if ((p.checked_at ?? 0) <= 0 && p.ok !== true) {
      return false;
    }
    if (p.ok === true) {
      return true;
    }
    if (p.ok === false) {
      return false;
    }
    const s = (p.last_status || '').toLowerCase();
    return s === 'up' || s === 'ok' || s === 'success';
  }

  private probeTypes(): string[] {
    const set: string[] = [];
    for (let i = 0; i < this.probes.length; i++) {
      const t = (this.probes[i].type || '').trim().toUpperCase();
      if (t && set.indexOf(t) < 0) {
        set.push(t);
      }
    }
    set.sort((a: string, b: string) => a.localeCompare(b));
    return set;
  }

  private probeTypeLabels(): string[] {
    return ['全部'].concat(this.probeTypes());
  }

  private endpointOk(ep: ApiEndpoint): boolean {
    return (ep.checked_at ?? 0) > 0 && ep.ok === true;
  }

  private endpointFail(ep: ApiEndpoint): boolean {
    return (ep.checked_at ?? 0) > 0 && ep.ok === false;
  }

  private filteredProbes(): Probe[] {
    let list = this.probes;
    if (this.probeOverviewIdx === 1) {
      list = list.filter((p: Probe) => (p.checked_at ?? 0) > 0 && p.ok === true);
    } else if (this.probeOverviewIdx === 2) {
      list = list.filter((p: Probe) => (p.checked_at ?? 0) > 0 && p.ok === false);
    } else if (this.probeOverviewIdx === 3) {
      list = list.filter((p: Probe) => (p.checked_at ?? 0) > 0);
    } else if (this.probeOverviewIdx === 4) {
      list = list.filter((p: Probe) => this.isHttpsTarget(p.target));
    }
    if (this.probeTypeIdx > 0) {
      const types = this.probeTypes();
      const want = types[this.probeTypeIdx - 1] || '';
      list = list.filter((p: Probe) => (p.type || '').toUpperCase() === want);
    }
    return list;
  }

  /** Android visibleSystems：按快捷筛选过滤端点；无端点且非「全部」时隐藏系统 */
  private filteredApiSystems(): ApiMonitorSystem[] {
    const out: ApiMonitorSystem[] = [];
    for (let i = 0; i < this.apiSystems.length; i++) {
      const sys = this.apiSystems[i];
      const eps = sys.endpoints ?? [];
      let filtered: ApiEndpoint[] = eps;
      if (this.probeOverviewIdx === 1) {
        filtered = eps.filter((e: ApiEndpoint) => this.endpointOk(e));
      } else if (this.probeOverviewIdx === 2) {
        filtered = eps.filter((e: ApiEndpoint) => this.endpointFail(e));
      } else if (this.probeOverviewIdx === 3) {
        filtered = eps.filter((e: ApiEndpoint) => (e.checked_at ?? 0) > 0);
      } else if (this.probeOverviewIdx === 4) {
        filtered = eps.filter((e: ApiEndpoint) => this.isHttpsTarget(e.url));
      }
      if (filtered.length > 0 || this.probeOverviewIdx === 0) {
        const copy: ApiMonitorSystem = {
          id: sys.id,
          name: sys.name,
          interval_sec: sys.interval_sec,
          level: sys.level,
          enabled: sys.enabled,
          created_at: sys.created_at,
          endpoints: this.probeOverviewIdx === 0 ? eps : filtered
        };
        out.push(copy);
      }
    }
    return out;
  }

  private onProbeOverviewSelect(idx: number): void {
    this.probeOverviewIdx = idx;
    // Android openMetric：优先切到有匹配结果的 Tab
    let probeMatch = false;
    let epMatch = false;
    for (let i = 0; i < this.probes.length; i++) {
      const p = this.probes[i];
      if (idx === 0) {
        probeMatch = true;
        break;
      }
      if (idx === 1 && (p.checked_at ?? 0) > 0 && p.ok === true) {
        probeMatch = true;
        break;
      }
      if (idx === 2 && (p.checked_at ?? 0) > 0 && p.ok === false) {
        probeMatch = true;
        break;
      }
      if (idx === 3 && (p.checked_at ?? 0) > 0) {
        probeMatch = true;
        break;
      }
      if (idx === 4 && this.isHttpsTarget(p.target)) {
        probeMatch = true;
        break;
      }
    }
    for (let s = 0; s < this.apiSystems.length; s++) {
      const eps = this.apiSystems[s].endpoints ?? [];
      for (let e = 0; e < eps.length; e++) {
        const ep = eps[e];
        if (idx === 0) {
          epMatch = true;
          break;
        }
        if (idx === 1 && this.endpointOk(ep)) {
          epMatch = true;
          break;
        }
        if (idx === 2 && this.endpointFail(ep)) {
          epMatch = true;
          break;
        }
        if (idx === 3 && (ep.checked_at ?? 0) > 0) {
          epMatch = true;
          break;
        }
        if (idx === 4 && this.isHttpsTarget(ep.url)) {
          epMatch = true;
          break;
        }
      }
      if (epMatch) {
        break;
      }
    }
    if (probeMatch) {
      this.probeSeg = 0;
    } else if (epMatch) {
      this.probeSeg = 1;
    }
  }

  private probeSegChipLabels(): string[] {
    return [
      `拨测监控  ${this.filteredProbes().length}`,
      `API 业务监控  ${this.filteredApiSystems().length}`
    ];
  }

  private gpuHostCount(): number {
    let n = 0;
    for (let i = 0; i < this.hosts.length; i++) {
      const gpus = this.hosts[i].latest?.gpus ?? [];
      if (gpus.length > 0) {
        n++;
      }
    }
    return n;
  }

  private gpuPeakUtil(): number {
    const rows = this.gpuRanking(999);
    let peak = 0;
    for (let i = 0; i < rows.length; i++) {
      if (rows[i].util > peak) {
        peak = rows[i].util;
      }
    }
    return peak;
  }

  private gpuRanking(limit: number = 8): GpuRankRow[] {
    const rows: GpuRankRow[] = [];
    for (let i = 0; i < this.hosts.length; i++) {
      const h = this.hosts[i];
      const gpus = h.latest?.gpus ?? [];
      for (let g = 0; g < gpus.length; g++) {
        const gpu: GpuInfo = gpus[g];
        rows.push({
          hostId: h.id,
          hostname: h.hostname || h.id,
          gpuName: gpu.name || `GPU${g}`,
          util: Number(gpu.util_percent ?? 0),
          mem: Number(gpu.mem_percent ?? 0),
          temp: Number(gpu.temp ?? 0)
        });
      }
    }
    rows.sort((a: GpuRankRow, b: GpuRankRow) => b.util - a.util);
    return rows.slice(0, limit);
  }

  private hostGpuCount(h: Host): number {
    return (h.latest?.gpus ?? []).length;
  }

  private hostGpuMax(h: Host, field: string): number {
    const gpus = h.latest?.gpus ?? [];
    let maxv = 0;
    for (let i = 0; i < gpus.length; i++) {
      let v = 0;
      if (field === 'util') {
        v = Number(gpus[i].util_percent ?? 0);
      } else if (field === 'mem') {
        v = Number(gpus[i].mem_percent ?? 0);
      } else {
        v = Number(gpus[i].temp ?? 0);
      }
      if (v > maxv) {
        maxv = v;
      }
    }
    return maxv;
  }

  /** Android HostCard: 单卡且有显存总量时展示 name · used / total */
  private hostGpuDetail(h: Host): string {
    const gpus = h.latest?.gpus ?? [];
    if (gpus.length !== 1) {
      return '';
    }
    const g = gpus[0];
    if (!(g.mem_total && g.mem_total > 0)) {
      return '';
    }
    return `${g.name || 'GPU'} · ${FormatUtil.bytes(g.mem_used)} / ${FormatUtil.bytes(g.mem_total)}`;
  }

  private pct(v?: number): string {
    if (v === undefined || v === null) {
      return '';
    }
    return `${Math.round(v)}%`;
  }

  private statusColor(st: string): string {
    const s = (st || '').toLowerCase();
    if (s === 'up' || s === 'ok' || s === 'success' || s === 'running' || s === 'online') {
      return this.tSuccess;
    }
    if (s === 'down' || s === 'fail' || s === 'critical' || s === 'error') {
      return this.tDanger;
    }
    if (s.indexOf('warn') >= 0 || s === 'degraded') {
      return this.tWarning;
    }
    return this.tMuted;
  }

  private healthRank(h?: string): number {
    const s = (h || '').toLowerCase();
    if (s.indexOf('crit') >= 0 || s === 'error' || s === 'down' || s === 'failed') {
      return 0;
    }
    if (s.indexOf('warn') >= 0 || s === 'degraded') {
      return 1;
    }
    if (s === 'ok' || s === 'good' || s === 'healthy' || s === 'up') {
      return 3;
    }
    return 2;
  }

  private isBadHealth(h?: string): boolean {
    const s = (h || '').toLowerCase();
    if (!s || s === 'ok' || s === 'good' || s === 'healthy' || s === 'up' || s === 'enabled') {
      return false;
    }
    return s.indexOf('crit') >= 0 || s.indexOf('warn') >= 0 || s === 'error' ||
      s === 'down' || s === 'failed' || s === 'degraded' || s === 'critical';
  }

  private snapCpuCount(snap?: HardwareSnapshot): number {
    return (snap?.cpus ?? []).length;
  }

  private snapMemGb(snap?: HardwareSnapshot): number {
    return Number(snap?.memory?.total_gb ?? 0);
  }

  private snapDiskCount(snap?: HardwareSnapshot): number {
    return (snap?.storage ?? []).length;
  }

  private snapFanCount(snap?: HardwareSnapshot): number {
    return (snap?.fans ?? []).length;
  }

  private snapTempCount(snap?: HardwareSnapshot): number {
    return (snap?.temps ?? []).length;
  }

  private snapPowerW(snap?: HardwareSnapshot): number {
    return Number(snap?.power?.total_watts ?? 0);
  }

  private snapMaxTemp(snap?: HardwareSnapshot): number {
    let maxv = 0;
    const temps = snap?.temps ?? [];
    for (let i = 0; i < temps.length; i++) {
      const v = Number(temps[i].reading ?? 0);
      if (v > maxv) {
        maxv = v;
      }
    }
    return maxv;
  }

  private badParts(snap?: HardwareSnapshot): BadPartRow[] {
    const out: BadPartRow[] = [];
    if (!snap) {
      return out;
    }
    const cpus = snap.cpus ?? [];
    for (let i = 0; i < cpus.length; i++) {
      const c: RedfishCPU = cpus[i];
      if (this.isBadHealth(c.health)) {
        out.push({ kind: 'CPU', name: c.name || c.model || `CPU${i}`, health: c.health || '' });
      }
    }
    const dims = snap.memory?.dimms ?? [];
    for (let i = 0; i < dims.length; i++) {
      const d: MemoryDIMM = dims[i];
      if (this.isBadHealth(d.health)) {
        out.push({ kind: '内存', name: d.name || d.slot || `DIMM${i}`, health: d.health || '' });
      }
    }
    const disks = snap.storage ?? [];
    for (let i = 0; i < disks.length; i++) {
      const d: RedfishStorage = disks[i];
      if (this.isBadHealth(d.health) || d.smart_warn === true) {
        out.push({ kind: '磁盘', name: d.name || d.model || `Disk${i}`, health: d.health || 'SMART' });
      }
    }
    const fans = snap.fans ?? [];
    for (let i = 0; i < fans.length; i++) {
      const f: FanReading = fans[i];
      if (this.isBadHealth(f.health) || this.isBadHealth(f.status)) {
        out.push({ kind: '风扇', name: f.name || `Fan${i}`, health: f.health || f.status || '' });
      }
    }
    const temps = snap.temps ?? [];
    for (let i = 0; i < temps.length; i++) {
      const t: SensorReading = temps[i];
      if (this.isBadHealth(t.status)) {
        out.push({ kind: '温度', name: t.name || `Temp${i}`, health: t.status || '' });
      }
    }
    const psus = snap.power?.psus ?? [];
    for (let i = 0; i < psus.length; i++) {
      const p: PSUReading = psus[i];
      if (this.isBadHealth(p.health)) {
        out.push({ kind: '电源', name: p.name || `PSU${i}`, health: p.health || '' });
      }
    }
    return out;
  }

  private manufacturerModel(wrap: HardwareSnapshotWrap): string {
    const sys = wrap.snapshot?.system;
    const mfr = sys?.manufacturer || '';
    const model = sys?.model || '';
    if (mfr && model) {
      return `${mfr} / ${model}`;
    }
    return mfr || model || '—';
  }

  private hwMatches(wrap: HardwareSnapshotWrap, q: string): boolean {
    const qq = (q || '').trim().toLowerCase();
    if (!qq) {
      return true;
    }
    const sys = wrap.snapshot?.system;
    const hay = [
      wrap.target_name || '',
      wrap.target_url || '',
      sys?.manufacturer || '',
      sys?.model || '',
      sys?.serial_number || '',
      sys?.sku || '',
      sys?.host_name || ''
    ].join(' ').toLowerCase();
    return hay.indexOf(qq) >= 0;
  }

  private filteredHw(): HardwareSnapshotWrap[] {
    return this.hwSnapshots.filter((w: HardwareSnapshotWrap) => this.hwMatches(w, this.hwQuery));
  }

  private hvStateText(s?: string): string {
    const st = s || '';
    if (st === 'Running') {
      return '运行中';
    }
    if (st === 'Off') {
      return '已关机';
    }
    if (st === 'Paused') {
      return '已暂停';
    }
    if (st === 'Saved') {
      return '已保存';
    }
    return st || '—';
  }

  private hvUptime(sec?: number): string {
    const s = Number(sec ?? 0);
    if (s <= 0) {
      return '—';
    }
    const d = Math.floor(s / 86400);
    const h = Math.floor((s % 86400) / 3600);
    const m = Math.floor((s % 3600) / 60);
    if (d > 0) {
      return `${d}天 ${h}时`;
    }
    if (h > 0) {
      return `${h}时 ${m}分`;
    }
    return `${m}分`;
  }

  private hvGuestMatches(g: HyperVGuest, q: string): boolean {
    const qq = (q || '').trim().toLowerCase();
    if (!qq) {
      return true;
    }
    const ips = (g.ip_addresses ?? []).join(' ');
    const hay = `${g.name || ''} ${g.state || ''} ${ips} ${g.linked_host_name || ''}`.toLowerCase();
    return hay.indexOf(qq) >= 0;
  }

  private hvGuestVisible(g: HyperVGuest): boolean {
    if (!this.hvGuestMatches(g, this.hvQuery)) {
      return false;
    }
    if (this.hvFilter === 1) {
      return g.state === 'Running';
    }
    if (this.hvFilter === 2) {
      return g.state !== 'Running';
    }
    return true;
  }

  private visibleHvInventories(): HyperVInventory[] {
    const out: HyperVInventory[] = [];
    for (let i = 0; i < this.hvInventories.length; i++) {
      const inv = this.hvInventories[i];
      const guests = (inv.guests ?? []).filter((g: HyperVGuest) => this.hvGuestVisible(g));
      if (guests.length > 0) {
        out.push({
          host_id: inv.host_id,
          host_name: inv.host_name,
          guest_count: guests.length,
          guests: guests,
          updated_at: inv.updated_at
        });
      }
    }
    return out;
  }

  private hvRunningCount(inv: HyperVInventory): number {
    const guests = inv.guests ?? [];
    let n = 0;
    for (let i = 0; i < guests.length; i++) {
      if (guests[i].state === 'Running') {
        n++;
      }
    }
    return n;
  }

  private hvCsvCell(v: string): string {
    const s = v || '';
    if (s.indexOf(',') >= 0 || s.indexOf('"') >= 0 || s.indexOf('\n') >= 0) {
      return `"${s.replace(/"/g, '""')}"`;
    }
    return s;
  }

  private buildHvCsv(): string {
    const lines: string[] = [];
    lines.push('\uFEFF# Hyper-V 虚拟机资产清单');
    lines.push(`# 宿主机数,${this.hvInventories.length}`);
    let total = 0;
    for (let i = 0; i < this.hvInventories.length; i++) {
      total += (this.hvInventories[i].guests ?? []).length;
    }
    lines.push(`# 客户机数,${total}`);
    lines.push('');
    lines.push('宿主机名称,Host ID,客户机名,电源状态,健康,vCPU,占用CPU%,内存已分配(MB),内存需求(MB),运行时间(秒),IP地址,关联Agent主机');
    for (let i = 0; i < this.hvInventories.length; i++) {
      const inv = this.hvInventories[i];
      const guests = inv.guests ?? [];
      for (let j = 0; j < guests.length; j++) {
        const g = guests[j];
        const cols: string[] = [
          inv.host_name || inv.host_id || '',
          inv.host_id || '',
          g.name || '',
          g.state || '',
          g.health || '',
          `${g.processor_count ?? 0}`,
          `${g.cpu_usage ?? 0}`,
          `${g.mem_assigned_mb ?? 0}`,
          `${g.mem_demand_mb ?? 0}`,
          `${g.uptime_sec ?? 0}`,
          (g.ip_addresses ?? []).join(';'),
          g.linked_host_name || g.linked_host_id || ''
        ];
        lines.push(cols.map((c: string) => this.hvCsvCell(c)).join(','));
      }
    }
    return lines.join('\n');
  }

  private exportHvCsv(): void {
    try {
      const text = this.buildHvCsv();
      const data = pasteboard.createData(pasteboard.MIMETYPE_TEXT_PLAIN, text);
      pasteboard.getSystemPasteboard().setData(data);
      this.hvExportVisible = false;
      this.toast('已复制 Hyper-V CSV 到剪贴板');
    } catch (e) {
      this.toast(ApiClient.formatError(e as Object), true);
    }
  }

  private probeDisplayTarget(p: Probe): string {
    const t = p.target || '';
    if ((p.type || '').toLowerCase() === 'process' && t.indexOf('/') > 0) {
      const slash = t.indexOf('/');
      const hostId = t.substring(0, slash);
      const proc = t.substring(slash + 1);
      const host = this.hosts.find((h: Host) => h.id === hostId);
      if (host?.hostname) {
        return `${host.hostname}/${proc}`;
      }
    }
    return t;
  }

  private probeProcessName(p: Probe): string {
    const t = p.target || '';
    const slash = t.lastIndexOf('/');
    if (slash >= 0 && slash < t.length - 1) {
      return t.substring(slash + 1);
    }
    return t || '—';
  }

  private isHttpsTarget(target?: string): boolean {
    const t = (target || '').toLowerCase();
    return t.indexOf('https://') === 0;
  }

  private certLabel(days?: number, checkedAt?: number): string {
    if ((checkedAt ?? 0) <= 0) {
      return '等待首次 HTTPS 握手';
    }
    const d = days ?? -1;
    if (d < 0) {
      return '证书有效期暂不可用';
    }
    if (d === 0) {
      return '证书今天到期';
    }
    return `证书剩余 ${d} 天`;
  }

  private certColor(days?: number): string {
    const d = days ?? -1;
    if (d < 0) {
      return this.tMuted;
    }
    if (d <= 7) {
      return this.tDanger;
    }
    if (d <= 30) {
      return this.tWarning;
    }
    return this.tSuccess;
  }

  private probeStateColor(p: Probe): string {
    if (p.enabled === false || (p.checked_at ?? 0) <= 0) {
      return this.tMuted;
    }
    return this.probeOk(p) ? this.tSuccess : this.tDanger;
  }

  private isProbeTesting(id: string): boolean {
    return this.testingProbeIds.indexOf(id) >= 0;
  }

  private isApiTesting(id: string): boolean {
    return this.testingApiIds.indexOf(id) >= 0;
  }

  private probeOverviewCount(idx: number): number {
    const probes = this.probes;
    const endpoints: ApiEndpoint[] = [];
    for (let i = 0; i < this.apiSystems.length; i++) {
      const eps = this.apiSystems[i].endpoints ?? [];
      for (let j = 0; j < eps.length; j++) {
        endpoints.push(eps[j]);
      }
    }
    if (idx === 0) {
      return probes.length + endpoints.length;
    }
    if (idx === 1) {
      let n = 0;
      for (let i = 0; i < probes.length; i++) {
        if ((probes[i].checked_at ?? 0) > 0 && probes[i].ok === true) {
          n++;
        }
      }
      for (let i = 0; i < endpoints.length; i++) {
        if (this.endpointOk(endpoints[i])) {
          n++;
        }
      }
      return n;
    }
    if (idx === 2) {
      let n = 0;
      for (let i = 0; i < probes.length; i++) {
        if ((probes[i].checked_at ?? 0) > 0 && probes[i].ok === false) {
          n++;
        }
      }
      for (let i = 0; i < endpoints.length; i++) {
        if (this.endpointFail(endpoints[i])) {
          n++;
        }
      }
      return n;
    }
    if (idx === 3) {
      let n = 0;
      for (let i = 0; i < probes.length; i++) {
        if ((probes[i].checked_at ?? 0) > 0) {
          n++;
        }
      }
      for (let i = 0; i < endpoints.length; i++) {
        if ((endpoints[i].checked_at ?? 0) > 0) {
          n++;
        }
      }
      return n;
    }
    // HTTPS 证书：仅计有有效 cert_days 的
    let n = 0;
    for (let i = 0; i < probes.length; i++) {
      if (this.isHttpsTarget(probes[i].target) && (probes[i].cert_days ?? -1) >= 0) {
        n++;
      }
    }
    for (let i = 0; i < endpoints.length; i++) {
      if (this.isHttpsTarget(endpoints[i].url) && (endpoints[i].cert_days ?? -1) >= 0) {
        n++;
      }
    }
    return n;
  }

  private nearestCertDays(): number {
    let nearest = -1;
    for (let i = 0; i < this.probes.length; i++) {
      const p = this.probes[i];
      if (this.isHttpsTarget(p.target) && (p.cert_days ?? -1) >= 0) {
        const d = p.cert_days ?? 0;
        if (nearest < 0 || d < nearest) {
          nearest = d;
        }
      }
    }
    for (let s = 0; s < this.apiSystems.length; s++) {
      const eps = this.apiSystems[s].endpoints ?? [];
      for (let e = 0; e < eps.length; e++) {
        const ep = eps[e];
        if (this.isHttpsTarget(ep.url) && (ep.cert_days ?? -1) >= 0) {
          const d = ep.cert_days ?? 0;
          if (nearest < 0 || d < nearest) {
            nearest = d;
          }
        }
      }
    }
    return nearest;
  }

  private probeOverviewChipLabels(): string[] {
    return [
      `全部对象 ${this.probeOverviewCount(0)}`,
      `正常 ${this.probeOverviewCount(1)}`,
      `异常 ${this.probeOverviewCount(2)}`,
      `已探测 ${this.probeOverviewCount(3)}`,
      `HTTPS 证书 ${this.probeOverviewCount(4)}`
    ];
  }

  private snmpName(d: SNMPDevice): string {
    return d.device_name || d.name || d.device_ip || d.ip || '设备';
  }

  private snmpIp(d: SNMPDevice): string {
    return d.device_ip || d.ip || '';
  }

  private snmpKey(d: SNMPDevice): string {
    return this.snmpIp(d) || this.snmpName(d);
  }

  private snmpIfaces(d: SNMPDevice): SNMPInterface[] {
    const list = (d.snapshot?.interfaces ?? []).slice();
    list.sort((a: SNMPInterface, b: SNMPInterface) => this.ifBad(b) - this.ifBad(a));
    if (this.ifaceFilter === 1) {
      return list.filter((x: SNMPInterface) => x.oper_up === true || x.oper_status === 1);
    }
    if (this.ifaceFilter === 2) {
      return list.filter((x: SNMPInterface) => !(x.oper_up === true || x.oper_status === 1));
    }
    return list;
  }

  private ifBad(i: SNMPInterface): number {
    let score = 0;
    if (!(i.oper_up === true || i.oper_status === 1)) {
      score += 2;
    }
    if ((i.in_err_pps ?? 0) + (i.out_err_pps ?? 0) > 0) {
      score += 1;
    }
    if ((i.in_discard_pps ?? 0) + (i.out_discard_pps ?? 0) > 0) {
      score += 1;
    }
    const util = Math.max(i.in_util_percent ?? 0, i.out_util_percent ?? 0);
    if (util >= 80) {
      score += 1;
    }
    return score;
  }

  private snmpUpCount(d: SNMPDevice): number {
    const list = d.snapshot?.interfaces ?? [];
    let n = 0;
    for (let i = 0; i < list.length; i++) {
      if (list[i].oper_up === true || list[i].oper_status === 1) {
        n++;
      }
    }
    return n;
  }

  private snmpUptimeText(d: SNMPDevice): string {
    const sec = Number(d.snapshot?.system?.uptime_sec ?? 0);
    if (sec <= 0) {
      return '—';
    }
    const days = Math.floor(sec / 86400);
    const hours = Math.floor((sec % 86400) / 3600);
    if (days > 0) {
      return `${days}天${hours}时`;
    }
    const mins = Math.floor((sec % 3600) / 60);
    if (hours > 0) {
      return `${hours}时${mins}分`;
    }
    return `${mins}分`;
  }

  private fmtIfSpeed(bps?: number): string {
    const v = Number(bps ?? 0);
    if (v <= 0) {
      return '—';
    }
    if (v >= 1e9) {
      return `${(v / 1e9).toFixed(0)}G`;
    }
    if (v >= 1e6) {
      return `${(v / 1e6).toFixed(0)}M`;
    }
    if (v >= 1e3) {
      return `${(v / 1e3).toFixed(0)}K`;
    }
    return `${Math.round(v)}`;
  }

  private ifaceUtil(i: SNMPInterface): number {
    return Math.max(Number(i.in_util_percent ?? 0), Number(i.out_util_percent ?? 0));
  }

  private ifaceErrRate(i: SNMPInterface): number {
    return Number(i.in_err_pps ?? 0) + Number(i.out_err_pps ?? 0) +
      Number(i.in_discard_pps ?? 0) + Number(i.out_discard_pps ?? 0);
  }

  private protoName(p?: number | string): string {
    const n = Number(p);
    if (n === 6) {
      return 'TCP';
    }
    if (n === 17) {
      return 'UDP';
    }
    if (n === 1) {
      return 'ICMP';
    }
    if (typeof p === 'string' && p) {
      return p;
    }
    if (!isNaN(n) && n > 0) {
      return `${n}`;
    }
    return '—';
  }

  private seriesToChartPoints(resp: SNMPInterfaceHistoryResponse | null, key: string): ChartPoint[] {
    const out: ChartPoint[] = [];
    if (!resp || !resp.series) {
      return out;
    }
    const arr: PromRangeResult[] = resp.series[key] ?? [];
    for (let i = 0; i < arr.length; i++) {
      const rawVals = arr[i].values;
      if (!rawVals) {
        continue;
      }
      for (let j = 0; j < rawVals.length; j++) {
        const pair = rawVals[j];
        if (pair && pair.length >= 2) {
          const t = Number(pair[0]);
          const v = Number(pair[1]);
          if (!isNaN(t) && !isNaN(v)) {
            out.push({ t: t, v: v });
          }
        }
      }
    }
    out.sort((a: ChartPoint, b: ChartPoint) => a.t - b.t);
    return out;
  }

  private applySnmpHistCharts(resp: SNMPInterfaceHistoryResponse | null): void {
    this.snmpHistResp = resp;
    this.snmpHistInPts = this.seriesToChartPoints(resp, 'in_bps');
    this.snmpHistOutPts = this.seriesToChartPoints(resp, 'out_bps');
    this.snmpHistUtilInPts = this.seriesToChartPoints(resp, 'in_util');
    this.snmpHistUtilOutPts = this.seriesToChartPoints(resp, 'out_util');
    const total = this.snmpHistInPts.length + this.snmpHistOutPts.length +
      this.snmpHistUtilInPts.length + this.snmpHistUtilOutPts.length;
    this.snmpHistText = total > 0 ? '' : '当前时间范围内无接口历史数据';
  }

  private ipHistoryChartPts(): ChartPoint[] {
    const pts = this.ipHist?.points ?? [];
    const out: ChartPoint[] = [];
    for (let i = 0; i < pts.length; i++) {
      const t = Number(pts[i].timestamp ?? 0);
      const v = Number(pts[i].bytes ?? 0);
      if (t > 0) {
        out.push({ t: t, v: v });
      }
    }
    return out;
  }

  private trapVarbindText(tr: SNMPTrap): string {
    const vbs = tr.varbinds ?? [];
    const parts: string[] = [];
    for (let i = 0; i < vbs.length && i < 4; i++) {
      const oid = vbs[i].oid || '';
      const short = oid.indexOf('.') >= 0 ? oid.substring(oid.lastIndexOf('.') + 1) : oid;
      parts.push(`${short}=${vbs[i].value || ''}`);
    }
    return parts.join('  ');
  }

  private nfMaxBytes(): number {
    let m = 1;
    for (let i = 0; i < this.netflowItems.length; i++) {
      const b = Number(this.netflowItems[i].bytes ?? 0);
      if (b > m) {
        m = b;
      }
    }
    return m;
  }

  private filteredAudits(): ContentAuditEvent[] {
    const q = this.auditQuery.trim().toLowerCase();
    if (!q) {
      return this.audits;
    }
    return this.audits.filter((ev: ContentAuditEvent) => {
      const blob = `${ev.method || ''} ${ev.path || ''} ${ev.src_ip || ''} ${ev.dst_ip || ''} ${ev.status ?? ''} ${ev.sensitive || ''}`.toLowerCase();
      return blob.indexOf(q) >= 0;
    });
  }

  private relIso(s?: string): string {
    if (!s) {
      return '—';
    }
    const t = Date.parse(s);
    if (isNaN(t)) {
      return s;
    }
    return FormatUtil.relTime(t);
  }

  private histAvg(): number {
    if (this.histPoints.length === 0) {
      return 0;
    }
    let sum = 0;
    for (let i = 0; i < this.histPoints.length; i++) {
      sum += Number(this.histPoints[i].latency_ms ?? 0);
    }
    return sum / this.histPoints.length;
  }

  private histP95(): number {
    return this.histPercentile(0.95);
  }

  private histP99(): number {
    return this.histPercentile(0.99);
  }

  private histPercentile(p: number): number {
    if (this.histPoints.length === 0) {
      return 0;
    }
    const arr: number[] = [];
    for (let i = 0; i < this.histPoints.length; i++) {
      const v = Number(this.histPoints[i].latency_ms ?? 0);
      if (v >= 0) {
        arr.push(v);
      }
    }
    if (arr.length === 0) {
      return 0;
    }
    arr.sort((a: number, b: number) => a - b);
    const idx = Math.min(arr.length - 1, Math.floor((arr.length - 1) * p));
    return arr[idx];
  }

  private histUptime(): number {
    if (this.histPoints.length === 0) {
      return 0;
    }
    let ok = 0;
    for (let i = 0; i < this.histPoints.length; i++) {
      if (this.histPoints[i].ok !== false) {
        ok++;
      }
    }
    return (ok / this.histPoints.length) * 100;
  }

  private histChartPts(): ChartPoint[] {
    const out: ChartPoint[] = [];
    for (let i = 0; i < this.histPoints.length; i++) {
      const pt = this.histPoints[i];
      out.push({ t: Number(pt.ts ?? pt.timestamp ?? i), v: Number(pt.latency_ms ?? 0) });
    }
    return out;
  }

  private histAvailPts(): ChartPoint[] {
    const out: ChartPoint[] = [];
    for (let i = 0; i < this.histPoints.length; i++) {
      const pt = this.histPoints[i];
      out.push({
        t: Number(pt.ts ?? pt.timestamp ?? i),
        v: pt.ok === false ? 0 : 100
      });
    }
    return out;
  }

  private histBytesPts(): ChartPoint[] {
    const out: ChartPoint[] = [];
    for (let i = 0; i < this.histPoints.length; i++) {
      const pt = this.histPoints[i];
      const b = Number(pt.resp_bytes ?? 0);
      if (b > 0) {
        out.push({ t: Number(pt.ts ?? pt.timestamp ?? i), v: b / 1024 });
      }
    }
    return out;
  }

  private histHasBytes(): boolean {
    for (let i = 0; i < this.histPoints.length; i++) {
      if ((this.histPoints[i].resp_bytes ?? 0) > 0) {
        return true;
      }
    }
    return false;
  }

  private histPhasePts(kind: string): ChartPoint[] {
    const out: ChartPoint[] = [];
    for (let i = 0; i < this.histPoints.length; i++) {
      const pt = this.histPoints[i];
      let v = 0;
      if (kind === 'dns') {
        v = Number(pt.dns_ms ?? 0);
      } else if (kind === 'tcp') {
        v = Number(pt.tcp_ms ?? 0);
      } else if (kind === 'tls') {
        v = Number(pt.tls_ms ?? 0);
      } else if (kind === 'ttfb') {
        v = Number(pt.ttfb_ms ?? 0);
      }
      if (v > 0) {
        out.push({ t: Number(pt.ts ?? pt.timestamp ?? i), v: v });
      }
    }
    return out;
  }

  private histHasPhase(): boolean {
    for (let i = 0; i < this.histPoints.length; i++) {
      const pt = this.histPoints[i];
      if ((pt.dns_ms ?? 0) > 0 || (pt.tcp_ms ?? 0) > 0 || (pt.tls_ms ?? 0) > 0 || (pt.ttfb_ms ?? 0) > 0) {
        return true;
      }
    }
    return false;
  }

  private nfRangeMinutes(range: string): number {
    if (range === '1h') {
      return 60;
    }
    if (range === '3h') {
      return 180;
    }
    if (range === '6h') {
      return 360;
    }
    if (range === '12h') {
      return 720;
    }
    if (range === '24h') {
      return 1440;
    }
    if (range === '3d') {
      return 4320;
    }
    if (range === '7d') {
      return 10080;
    }
    if (range === '14d') {
      return 20160;
    }
    return 60;
  }

  private digitsOnly(v: string, maxLen: number): string {
    let out = '';
    for (let i = 0; i < v.length && out.length < maxLen; i++) {
      const ch = v.charAt(i);
      if (ch >= '0' && ch <= '9') {
        out += ch;
      }
    }
    return out;
  }

  /** 简易延迟直方图：返回 8 桶 label/count */
  private histBuckets(): string[] {
    const vals: number[] = [];
    for (let i = 0; i < this.histPoints.length; i++) {
      const v = Number(this.histPoints[i].latency_ms ?? 0);
      if (v >= 0) {
        vals.push(v);
      }
    }
    if (vals.length === 0) {
      return [];
    }
    let minv = vals[0];
    let maxv = vals[0];
    for (let i = 1; i < vals.length; i++) {
      if (vals[i] < minv) {
        minv = vals[i];
      }
      if (vals[i] > maxv) {
        maxv = vals[i];
      }
    }
    if (maxv <= minv) {
      maxv = minv + 1;
    }
    const n = 8;
    const step = (maxv - minv) / n;
    const counts: number[] = [];
    for (let i = 0; i < n; i++) {
      counts.push(0);
    }
    for (let i = 0; i < vals.length; i++) {
      let b = Math.floor((vals[i] - minv) / step);
      if (b >= n) {
        b = n - 1;
      }
      if (b < 0) {
        b = 0;
      }
      counts[b]++;
    }
    const lines: string[] = [];
    for (let i = 0; i < n; i++) {
      const lo = minv + step * i;
      const hi = minv + step * (i + 1);
      const barLen = Math.max(1, Math.round((counts[i] / vals.length) * 20));
      let bar = '';
      for (let k = 0; k < barLen; k++) {
        bar += '█';
      }
      lines.push(`${lo.toFixed(0)}-${hi.toFixed(0)}ms  ${bar} ${counts[i]}`);
    }
    return lines;
  }

  private histSinceMin(): number {
    return this.histRangesMin[this.histRangeIdx] || 180;
  }

  private ipHistTotalBytes(): number {
    const pts = this.ipHist?.points ?? [];
    let sum = 0;
    for (let i = 0; i < pts.length; i++) {
      sum += Number(pts[i].bytes ?? 0);
    }
    return sum;
  }

  private availPct(ep: ApiEndpoint): number {
    const v = Number(ep.avail_1h ?? ep.avail_24h ?? 0);
    if (v <= 1) {
      return Math.round(v * 100);
    }
    return Math.round(v);
  }

  private openAi(task: string, title: string, context: string): void {
    this.aiTask = task;
    this.aiTitle = title;
    this.aiContext = context.length > 14000 ? context.substring(0, 14000) : context;
    this.aiSeed = '';
    this.aiAssistVisible = true;
  }

  private hwContext(wrap: HardwareSnapshotWrap): string {
    const snap = wrap.snapshot;
    const parts = this.badParts(snap);
    const lines: string[] = [];
    lines.push(`主机：${wrap.target_name || ''} BMC：${wrap.target_url || ''}`);
    lines.push(`整机健康：${wrap.health || '—'}`);
    lines.push(`型号：${this.manufacturerModel(wrap)}`);
    lines.push(
      `异常部件：${parts.length} 最高温：${Math.round(this.snapMaxTemp(snap))}°C 功耗：${Math.round(this.snapPowerW(snap))}W`
    );
    for (let i = 0; i < parts.length && i < 20; i++) {
      const p = parts[i];
      lines.push(`- ${p.kind} ${p.name} ${p.health}`);
    }
    const evs = snap?.events ?? [];
    for (let i = 0; i < evs.length && i < 10; i++) {
      const ev = evs[i];
      lines.push(`事件[${ev.severity || ''}] ${ev.message || ''}`);
    }
    return lines.join('\n');
  }

  async refresh(): Promise<void> {
    this.error = '';
    this.loadingHosts = true;
    try {
      this.hosts = await Api.hosts();
      if (!this.selectedHost && this.hosts.length > 0) {
        this.selectedHost = this.hosts[0].id;
      }

      if (this.tab === 1) {
        if (this.resSub === 0) {
          const snaps: HardwareSnapshotWrap[] = [];
          for (let i = 0; i < this.hosts.length; i++) {
            const h = this.hosts[i];
            try {
              const hw = await Api.hardwareHealth(h.id);
              const list = hw.snapshots ?? [];
              for (let j = 0; j < list.length; j++) {
                const raw = list[j];
                const wrap: HardwareSnapshotWrap = {
                  target_name: raw.target_name || h.hostname || h.id,
                  target_url: raw.target_url,
                  health: raw.health,
                  updated_at: raw.updated_at,
                  snapshot: raw.snapshot
                };
                snaps.push(wrap);
              }
            } catch (_ignore) {
              // skip unreachable host (Promise.allSettled style)
            }
          }
          snaps.sort((a: HardwareSnapshotWrap, b: HardwareSnapshotWrap) => {
            const ra = this.healthRank(a.health || a.snapshot?.state);
            const rb = this.healthRank(b.health || b.snapshot?.state);
            if (ra !== rb) {
              return ra - rb;
            }
            return (a.target_name || '').localeCompare(b.target_name || '');
          });
          this.hwSnapshots = snaps;
        } else {
          const hv = await Api.hypervList();
          this.hvInventories = hv.inventories ?? [];
        }
      }

      if (this.tab === 2) {
        try {
          if (this.netSub === 0 || this.netSub === 2) {
            const r = await Api.snmpHosts();
            this.dataHostsSnmp = r.hosts ?? [];
          } else if (this.netSub === 1) {
            const r = await Api.netflowHosts();
            this.dataHostsNetflow = r.hosts ?? [];
          } else {
            const r = await Api.contentAuditHosts();
            this.dataHostsAudit = r.hosts ?? [];
          }
        } catch (_ignore) {
          // keep previous data hosts
        }
        this.preferSelectedFromDataHosts();
        if (this.selectedHost) {
          if (this.netSub === 0) {
            const s = await Api.snmpList(this.selectedHost);
            this.snmpDevices = s.devices ?? [];
          } else if (this.netSub === 1) {
            const dim = this.nfDims[this.netDimIdx] || 'dst_ip';
            const range = this.nfRanges[this.netRangeIdx] || '1h';
            const n = await Api.netflowSummary(this.selectedHost, dim, range, 20);
            this.netflowItems = n.summary ?? n.items ?? [];
            const f = await Api.netflowFlows(this.selectedHost, 80, range);
            this.netflowFlows = f.flows ?? [];
          } else if (this.netSub === 2) {
            const t = await Api.snmpTraps(this.selectedHost);
            this.traps = t.traps ?? [];
          } else {
            const a = await Api.contentAudit(this.selectedHost, 200, this.auditQuery.trim());
            this.audits = a.events ?? [];
          }
        }
      }

      if (this.tab === 3) {
        this.probes = await Api.probes();
        if (this.probeTypeIdx >= this.probeTypeLabels().length) {
          this.probeTypeIdx = 0;
        }
        try {
          const ov = await Api.apiMonitors();
          this.apiSystems = ov.systems ?? [];
        } catch (_ignore) {
          // keep previous
        }
      }
    } catch (e) {
      this.error = ApiClient.formatError(e as Object);
    } finally {
      this.loadingHosts = false;
    }
  }

  private toast(msg: string, isError: boolean = false): void {
    this.toastMsg = msg;
    this.toastError = isError;
    ToastController.delayClear((v: string) => { this.toastMsg = v; }, msg);
  }

  private openProbeCreate(): void {
    this.probeEditId = '';
    this.probeName = '';
    this.probeTarget = '';
    this.probeTypeSel = 0;
    this.probeInterval = 60;
    this.probeDnsType = 'A';
    this.probeExpect = '';
    this.probeTimeout = 5;
    this.probeEnabled = true;
    this.probeLevel = 'critical';
    this.probeFormVisible = true;
  }

  private openProbeEdit(p: Probe): void {
    this.probeEditId = p.id;
    this.probeName = p.name || '';
    this.probeTarget = p.target || '';
    const ty = (p.type || 'http').toLowerCase();
    let idx = this.probeTypesAll.indexOf(ty);
    if (idx < 0) {
      idx = 0;
    }
    this.probeTypeSel = idx;
    this.probeInterval = Number(p.interval_sec ?? 60);
    this.probeDnsType = p.dns_type || 'A';
    this.probeExpect = p.expect_keyword || '';
    this.probeTimeout = Number(p.timeout_sec ?? 5);
    if (this.probeTimeout <= 0) {
      this.probeTimeout = 5;
    }
    this.probeEnabled = p.enabled !== false;
    this.probeLevel = (p.level || 'critical').toLowerCase() === 'warning' ? 'warning' : 'critical';
    this.probeFormVisible = true;
  }

  async refreshMe(): Promise<void> {
    try {
      const me = await Api.me();
      this.isAdmin = (me.role || '').toLowerCase() === 'admin';
    } catch (_e) {
      this.isAdmin = false;
    }
  }

  async openDnsThresholds(): Promise<void> {
    this.dnsWarnMs = '500';
    this.dnsCritMs = '2000';
    try {
      const cfg = await Api.serverConfig();
      const raw = JSON.stringify(cfg);
      const parsed = JSON.parse(raw) as Record<string, Object>;
      const thObj = parsed['thresholds'];
      if (thObj) {
        const th = thObj as Record<string, Object>;
        const w = Number(th['check_dns_timeout_warn'] ?? 0);
        const c = Number(th['check_dns_timeout_crit'] ?? 0);
        if (w > 0) {
          this.dnsWarnMs = `${Math.round(w)}`;
        }
        if (c > 0) {
          this.dnsCritMs = `${Math.round(c)}`;
        }
      }
    } catch (_e) {
      // keep defaults
    }
    this.dnsThVisible = true;
  }

  async saveDnsThresholds(): Promise<void> {
    if (this.dnsThBusy) {
      return;
    }
    if (!this.isAdmin) {
      this.toast('仅管理员可修改告警阈值', true);
      return;
    }
    const warn = Number(this.dnsWarnMs) || 500;
    const crit = Number(this.dnsCritMs) || 2000;
    this.dnsThBusy = true;
    try {
      const cfg = await Api.serverConfig();
      const parsed = JSON.parse(JSON.stringify(cfg)) as Record<string, Object>;
      const thRaw = parsed['thresholds'];
      const th = JSON.parse(thRaw ? JSON.stringify(thRaw) : '{}') as Record<string, Object>;
      th['check_dns_timeout_warn'] = warn;
      th['check_dns_timeout_crit'] = crit;
      parsed['thresholds'] = th;
      await Api.saveServerConfig(parsed);
      this.dnsThVisible = false;
      this.toast('DNS 阈值已保存');
    } catch (e) {
      this.toast(ApiClient.formatError(e as Object), true);
    } finally {
      this.dnsThBusy = false;
    }
  }

  async saveProbe(): Promise<void> {
    if (!this.probeName.trim() || !this.probeTarget.trim() || this.probeBusy) {
      if (!this.probeName.trim() || !this.probeTarget.trim()) {
        this.toast('请填写名称与目标', true);
      }
      return;
    }
    this.probeBusy = true;
    const ty = this.probeTypesAll[this.probeTypeSel] || 'http';
    const body: CheckUpsertRequest = {
      id: this.probeEditId || undefined,
      name: this.probeName.trim(),
      target: this.probeTarget.trim(),
      type: ty,
      enabled: this.probeEnabled,
      interval_sec: this.probeInterval > 0 ? this.probeInterval : 60,
      level: this.probeLevel,
      dns_type: ty === 'dns' ? this.probeDnsType : undefined,
      expect_keyword: ty === 'dns' ? this.probeExpect.trim() : undefined,
      timeout_sec: ty === 'dns' ? this.probeTimeout : undefined
    };
    try {
      await Api.upsertCheck(body);
      this.probeFormVisible = false;
      this.toast(this.probeEditId ? '已更新拨测' : '已创建拨测');
      await this.refresh();
    } catch (e) {
      this.toast(ApiClient.formatError(e as Object), true);
    } finally {
      this.probeBusy = false;
    }
  }

  async loadProbeHistory(p: Probe): Promise<void> {
    this.histTitle = p.name || p.id;
    this.histSourceKind = 'probe';
    this.histSourceId = p.id;
    this.histVisible = true;
    await this.reloadHistPoints();
  }

  async loadApiHistory(ep: ApiEndpoint): Promise<void> {
    this.histTitle = ep.name || ep.url || ep.id;
    this.histSourceKind = 'api';
    this.histSourceId = ep.id;
    this.histVisible = true;
    await this.reloadHistPoints();
  }

  async reloadHistPoints(): Promise<void> {
    if (!this.histSourceId) {
      return;
    }
    this.histPoints = [];
    try {
      const since = this.histSinceMin();
      if (this.histSourceKind === 'api') {
        this.histPoints = await Api.apiMonitorHistory(this.histSourceId, since);
      } else {
        this.histPoints = await Api.probeHistory(this.histSourceId, since);
      }
    } catch (e) {
      this.toast(ApiClient.formatError(e as Object), true);
    }
  }

  async runProbe(p: Probe): Promise<void> {
    if (p.enabled === false || this.isProbeTesting(p.id)) {
      return;
    }
    const next = this.testingProbeIds.slice();
    next.push(p.id);
    this.testingProbeIds = next;
    try {
      await Api.testProbe(p.id);
      this.toast(`已触发：${p.name || p.id}`);
      await new Promise<void>((resolve: () => void) => {
        setTimeout(() => {
          resolve();
        }, 1800);
      });
      await this.refresh();
    } catch (e) {
      this.toast(ApiClient.formatError(e as Object), true);
    } finally {
      this.testingProbeIds = this.testingProbeIds.filter((id: string) => id !== p.id);
    }
  }

  async doDeleteProbe(): Promise<void> {
    this.probeDeleteVisible = false;
    if (!this.probeDeleteId) {
      return;
    }
    try {
      await Api.deleteCheck(this.probeDeleteId);
      this.toast('已删除拨测');
      this.probeDeleteId = '';
      await this.refresh();
    } catch (e) {
      this.toast(ApiClient.formatError(e as Object), true);
    }
  }

  async doDeleteApi(): Promise<void> {
    this.apiDeleteVisible = false;
    if (!this.apiDeleteId) {
      return;
    }
    try {
      await Api.deleteApiSystem(this.apiDeleteId);
      this.toast('已删除 API 系统');
      this.apiDeleteId = '';
      await this.refresh();
    } catch (e) {
      this.toast(ApiClient.formatError(e as Object), true);
    }
  }

  async testApi(sys: ApiMonitorSystem): Promise<void> {
    if (sys.enabled === false || this.isApiTesting(sys.id)) {
      return;
    }
    const next = this.testingApiIds.slice();
    next.push(sys.id);
    this.testingApiIds = next;
    try {
      await Api.testApiSystem(sys.id);
      this.toast(`已触发：${sys.name || sys.id}`);
      await new Promise<void>((resolve: () => void) => {
        setTimeout(() => {
          resolve();
        }, 1800);
      });
      await this.refresh();
    } catch (e) {
      this.toast(ApiClient.formatError(e as Object), true);
    } finally {
      this.testingApiIds = this.testingApiIds.filter((id: string) => id !== sys.id);
    }
  }

  async loadIfaceHistory(d: SNMPDevice, iface: SNMPInterface): Promise<void> {
    const device = this.snmpIp(d) || this.snmpName(d);
    const ifindex = `${iface.index ?? ''}`;
    this.snmpHistDevice = d;
    this.snmpHistIface = iface;
    this.snmpFocusDevice = device;
    this.snmpFocusIface = iface.name || iface.descr || ifindex;
    this.snmpHistTitle = `${this.snmpFocusIface} · 接口历史`;
    this.snmpHistText = '加载中…';
    this.snmpHistResp = null;
    this.snmpHistInPts = [];
    this.snmpHistOutPts = [];
    this.snmpHistUtilInPts = [];
    this.snmpHistUtilOutPts = [];
    this.snmpHistVisible = true;
    if (!this.selectedHost || !ifindex) {
      this.snmpHistText = '缺少主机或接口索引';
      return;
    }
    await this.reloadSnmpIfaceHistory();
  }

  async reloadSnmpIfaceHistory(): Promise<void> {
    const d = this.snmpHistDevice;
    const iface = this.snmpHistIface;
    if (!d || !iface || !this.selectedHost) {
      return;
    }
    const device = this.snmpIp(d) || this.snmpName(d);
    const ifindex = `${iface.index ?? ''}`;
    const range = this.snmpHistRanges[this.snmpHistRangeIdx] || '1h';
    this.snmpHistText = '加载中…';
    try {
      const resp = await Api.snmpInterfaceHistory(this.selectedHost, device, ifindex, range);
      this.applySnmpHistCharts(resp);
    } catch (e) {
      this.snmpHistText = ApiClient.formatError(e as Object);
      this.snmpHistResp = null;
    }
  }

  async loadIpHistory(ip: string): Promise<void> {
    if (!ip || !this.selectedHost) {
      return;
    }
    const dim = this.nfDims[this.netDimIdx] || 'dst_ip';
    const range = this.nfRanges[this.netRangeIdx] || '1h';
    this.ipHistIp = ip;
    this.ipHistTitle = `${ip} · 流量历史`;
    this.ipHist = null;
    this.ipHistPts = [];
    this.ipHistVisible = true;
    try {
      this.ipHist = await Api.netflowIPHistory(this.selectedHost, ip, dim, range);
      this.ipHistPts = this.ipHistoryChartPts();
    } catch (e) {
      this.toast(ApiClient.formatError(e as Object), true);
    }
  }

  async loadRelatedFlows(): Promise<void> {
    const ip = this.ipHistIp;
    if (!ip || !this.selectedHost) {
      return;
    }
    const dim = this.nfDims[this.netDimIdx] || 'dst_ip';
    const range = this.nfRanges[this.netRangeIdx] || '1h';
    const to = Math.floor(Date.now() / 1000);
    const from = to - this.nfRangeMinutes(range) * 60;
    try {
      const f = await Api.netflowFlows(this.selectedHost, 100, range, `${dim}:${ip}`, from, to);
      this.netflowFlows = f.flows ?? [];
      this.ipHistVisible = false;
      this.toast(`已加载相关 Flow ${this.netflowFlows.length} 条`);
    } catch (e) {
      this.toast(ApiClient.formatError(e as Object), true);
    }
  }

  build() {
    Stack() {
      Column() {
        PageHeader({
          title: '基础设施',
          subtitle: this.tabSubtitles[this.tab] || '',
          trailingIcon: 'refresh',
          onTrailing: () => {
            void this.refresh();
          }
        })
        ErrorBanner({ message: this.error })

        FilterChips({
          labels: this.tabLabels,
          selected: $tab,
          variant: 'tab',
          onSelect: (_i: number) => {
            void this.refresh();
          }
        })
          .padding({ left: 8, right: 8, bottom: 4 })

        if (this.tab === 0) {
          this.hostsPane()
        } else if (this.tab === 1) {
          this.resourcePane()
        } else if (this.tab === 2) {
          this.networkPane()
        } else {
          this.probePane()
        }
      }
      .width('100%')
      .height('100%')
      .backgroundColor(this.tBg)

      DetailSheet({
        visible: this.vmSheetVisible,
        title: this.vmFocus?.name || '虚拟机详情',
        onClose: () => {
          this.vmSheetVisible = false;
          this.vmFocus = null;
          this.vmFocusHost = '';
        }
      }) {
        Column({ space: 6 }) {
          if (this.vmFocus) {
            Row() {
              Text(this.hvStateText(this.vmFocus.state))
                .fontSize(AppTheme.fontCaption)
                .fontWeight(FontWeight.Medium)
                .fontColor(this.vmFocus.state === 'Running' ? this.tSuccess : this.tMuted)
                .padding({ left: 8, right: 8, top: 3, bottom: 3 })
                .backgroundColor(this.vmFocus.state === 'Running' ? this.tSuccessSoft : this.tElevated)
                .borderRadius(AppTheme.rChip)
              Blank()
              if (this.vmFocus.health) {
                Text(FormatUtil.healthZh(this.vmFocus.health))
                  .fontSize(AppTheme.fontMicro)
                  .fontColor(this.statusColor(this.vmFocus.health))
              }
            }
            .width('100%')
            .margin({ bottom: 6 })

            KeyValueRow({ label: '状态', value: this.hvStateText(this.vmFocus.state) })
            KeyValueRow({ label: '宿主机', value: this.vmFocusHost || '—' })
            KeyValueRow({ label: '运行时间', value: this.vmFocus.state === 'Running' ? this.hvUptime(this.vmFocus.uptime_sec) : '—' })
            KeyValueRow({ label: 'vCPU', value: `${this.vmFocus.processor_count ?? 0}` })
            KeyValueRow({ label: 'CPU 占用', value: `${Number(this.vmFocus.cpu_usage ?? 0).toFixed(1)}%` })
            KeyValueRow({ label: '内存已分配', value: `${Math.round(this.vmFocus.mem_assigned_mb ?? 0)} MB` })
            KeyValueRow({ label: '内存需求', value: `${Math.round(this.vmFocus.mem_demand_mb ?? 0)} MB` })
            KeyValueRow({
              label: '内存模式',
              value: this.vmFocus.dynamic_mem_enabled ? '动态' : '静态'
            })
            KeyValueRow({ label: '代数', value: `${this.vmFocus.generation ?? '—'}` })
            KeyValueRow({ label: '配置版本', value: this.vmFocus.version || '—' })
            KeyValueRow({ label: '集成服务', value: this.vmFocus.integration_state || '—' })
            KeyValueRow({
              label: 'IP 地址',
              value: (this.vmFocus.ip_addresses ?? []).join(', ') || '—'
            })
            KeyValueRow({
              label: '虚拟交换机',
              value: (this.vmFocus.switches ?? []).join(', ') || '—'
            })
            KeyValueRow({ label: '硬盘数', value: `${this.vmFocus.vhd_count ?? (this.vmFocus.disks ?? []).length}` })
            KeyValueRow({ label: '检查点数', value: `${this.vmFocus.checkpoint_count ?? (this.vmFocus.checkpoints ?? []).length}` })
            if (this.vmFocus.linked_host_id || this.vmFocus.linked_host_name) {
              Button('打开关联主机')
                .width('100%').height(40).margin({ top: 10 })
                .backgroundColor(this.tAccent).fontColor(this.tOnAccent)
                .onClick(() => {
                  const id = this.vmFocus?.linked_host_id || '';
                  if (id) {
                    this.vmSheetVisible = false;
                    this.onOpenHost(id);
                  }
                })
            }
          }
        }
        .width('100%')
        .alignItems(HorizontalAlign.Start)
        .padding({ bottom: 12 })
      }

      DetailSheet({
        visible: this.hwSheetVisible,
        title: this.hwFocus?.target_name || '硬件详情',
        onClose: () => {
          this.hwSheetVisible = false;
          this.hwFocus = null;
        }
      }) {
        Column({ space: 8 }) {
          if (this.hwFocus) {
            if (this.hwFocus.snapshot?.error) {
              Text(`采集异常：${this.hwFocus.snapshot?.error}`)
                .fontSize(AppTheme.fontCaption).fontColor(this.tDanger).width('100%')
                .padding(10).backgroundColor(this.tDangerSoft).borderRadius(AppTheme.rXs)
            }

            Row({ space: 8 }) {
              Column() {
                Text(FormatUtil.healthZh(this.hwFocus.health || this.hwFocus.snapshot?.state || ''))
                  .fontSize(AppTheme.fontBody).fontWeight(FontWeight.Bold)
                  .fontColor(this.statusColor(this.hwFocus.health || ''))
                Text('整体健康').fontSize(AppTheme.fontTiny).fontColor(this.tMuted)
              }.layoutWeight(1)
              Column() {
                Text(`${this.snapPowerW(this.hwFocus.snapshot)} W`)
                  .fontSize(AppTheme.fontBody).fontWeight(FontWeight.Bold).fontColor(this.tText)
                Text('功耗').fontSize(AppTheme.fontTiny).fontColor(this.tMuted)
              }.layoutWeight(1)
              Column() {
                Text(`${this.badParts(this.hwFocus.snapshot).length}`)
                  .fontSize(AppTheme.fontBody).fontWeight(FontWeight.Bold)
                  .fontColor(this.badParts(this.hwFocus.snapshot).length > 0 ? this.tDanger : this.tSuccess)
                Text('异常').fontSize(AppTheme.fontTiny).fontColor(this.tMuted)
              }.layoutWeight(1)
            }
            .width('100%')
            .padding(10)
            .backgroundColor(this.tElevated)
            .borderRadius(AppTheme.rSm)

            Text('设备信息').fontSize(AppTheme.fontSmall).fontWeight(FontWeight.Bold).fontColor(this.tAccent).width('100%')
            KeyValueRow({ label: '厂商 / 型号', value: this.manufacturerModel(this.hwFocus) })
            KeyValueRow({ label: '序列号', value: this.hwFocus.snapshot?.system?.serial_number || '—' })
            KeyValueRow({ label: 'SKU', value: this.hwFocus.snapshot?.system?.sku || '—' })
            KeyValueRow({ label: '主机名', value: this.hwFocus.snapshot?.system?.host_name || '—' })
            KeyValueRow({ label: 'BIOS', value: this.hwFocus.snapshot?.system?.bios_version || '—' })
            KeyValueRow({ label: '电源状态', value: this.hwFocus.snapshot?.system?.power_state || '—' })
            KeyValueRow({
              label: 'BMC',
              value: `${this.hwFocus.snapshot?.system?.bmc_model || '—'} / ${this.hwFocus.snapshot?.system?.bmc_firmware || '—'}`
            })

            Text(`需要关注（${this.badParts(this.hwFocus.snapshot).length}）`)
              .fontSize(AppTheme.fontSmall).fontWeight(FontWeight.Bold).fontColor(this.tAccent).width('100%').margin({ top: 4 })
            if (this.badParts(this.hwFocus.snapshot).length === 0) {
              Text('无异常部件').fontSize(AppTheme.fontCaption).fontColor(this.tSuccess)
            } else {
              ForEach(this.badParts(this.hwFocus.snapshot), (bp: BadPartRow, i: number) => {
                Text(`${bp.kind} · ${bp.name} · ${bp.health}`)
                  .fontSize(AppTheme.fontCaption).fontColor(this.tDanger).width('100%')
              }, (bp: BadPartRow, i: number) => `bp-${i}-${bp.name}`)
            }

            if ((this.hwFocus.snapshot?.cpus ?? []).length > 0) {
              Text(`CPU（${(this.hwFocus.snapshot?.cpus ?? []).length}）`)
                .fontSize(AppTheme.fontSmall).fontWeight(FontWeight.Bold).fontColor(this.tAccent).width('100%').margin({ top: 4 })
              ForEach(this.hwFocus.snapshot?.cpus ?? [], (c: RedfishCPU, i: number) => {
                KeyValueRow({
                  label: `${c.name || `CPU${i}`} ${c.cores ?? 0}C/${c.threads ?? 0}T`,
                  value: `${(c.model || '').substring(0, 28)} · ${FormatUtil.healthZh(c.health || '')}`
                })
              }, (c: RedfishCPU, i: number) => `cpu-${i}-${c.name ?? ''}`)
            }

            if ((this.hwFocus.snapshot?.gpus ?? []).length > 0) {
              Text(`GPU / 加速卡（${(this.hwFocus.snapshot?.gpus ?? []).length}）`)
                .fontSize(AppTheme.fontSmall).fontWeight(FontWeight.Bold).fontColor(this.tAccent).width('100%').margin({ top: 4 })
              ForEach(this.hwFocus.snapshot?.gpus ?? [], (g: RedfishGPU, i: number) => {
                KeyValueRow({
                  label: g.name || `GPU${i}`,
                  value: `${g.model || '—'} · ${FormatUtil.healthZh(g.health || '')}`
                })
              }, (g: RedfishGPU, i: number) => `rgpu-${i}-${g.name ?? ''}`)
            }

            if ((this.hwFocus.snapshot?.memory?.dimms ?? []).length > 0) {
              Text(`内存（${Math.round(this.hwFocus.snapshot?.memory?.total_gb ?? 0)} GB）`)
                .fontSize(AppTheme.fontSmall).fontWeight(FontWeight.Bold).fontColor(this.tAccent).width('100%').margin({ top: 4 })
              ForEach((this.hwFocus.snapshot?.memory?.dimms ?? []).slice(0, 16), (d: MemoryDIMM, i: number) => {
                KeyValueRow({
                  label: `${d.slot || d.name || `DIMM${i}`} ${Math.round(d.capacity_gb ?? 0)}GB`,
                  value: `${d.type || ''} · ${FormatUtil.healthZh(d.health || '')}`
                })
              }, (d: MemoryDIMM, i: number) => `dimm-${i}-${d.slot ?? d.name ?? ''}`)
            }

            if ((this.hwFocus.snapshot?.storage ?? []).length > 0) {
              Text(`存储（${(this.hwFocus.snapshot?.storage ?? []).length}）`)
                .fontSize(AppTheme.fontSmall).fontWeight(FontWeight.Bold).fontColor(this.tAccent).width('100%').margin({ top: 4 })
              ForEach(this.hwFocus.snapshot?.storage ?? [], (d: RedfishStorage, i: number) => {
                KeyValueRow({
                  label: `${d.location || d.name || `盘${i}`} ${Math.round(d.capacity_gb ?? 0)}GB`,
                  value: `${d.media_type || ''}${d.smart_warn ? ' · SMART⚠' : ''} · ${FormatUtil.healthZh(d.health || '')}`
                })
              }, (d: RedfishStorage, i: number) => `sto-${i}-${d.name ?? ''}`)
            }

            if ((this.hwFocus.snapshot?.raid ?? []).length > 0) {
              Text(`RAID（${(this.hwFocus.snapshot?.raid ?? []).length}）`)
                .fontSize(AppTheme.fontSmall).fontWeight(FontWeight.Bold).fontColor(this.tAccent).width('100%').margin({ top: 4 })
              ForEach(this.hwFocus.snapshot?.raid ?? [], (r: RedfishRAID, i: number) => {
                KeyValueRow({
                  label: r.name || `RAID${i}`,
                  value: `${r.model || ''} · 盘 ${r.drive_count ?? 0} · ${FormatUtil.healthZh(r.health || '')}`
                })
              }, (r: RedfishRAID, i: number) => `raid-${i}-${r.name ?? ''}`)
            }

            if ((this.hwFocus.snapshot?.fans ?? []).length > 0) {
              Text(`风扇（${(this.hwFocus.snapshot?.fans ?? []).length}）`)
                .fontSize(AppTheme.fontSmall).fontWeight(FontWeight.Bold).fontColor(this.tAccent).width('100%').margin({ top: 4 })
              ForEach(this.hwFocus.snapshot?.fans ?? [], (f: FanReading, i: number) => {
                KeyValueRow({
                  label: f.name || `Fan${i}`,
                  value: `${f.rpm ?? 0} RPM · ${FormatUtil.healthZh(f.health || f.status || '')}`
                })
              }, (f: FanReading, i: number) => `fan-${i}-${f.name ?? ''}`)
            }

            if ((this.hwFocus.snapshot?.temps ?? []).length > 0) {
              Text(`温度（${(this.hwFocus.snapshot?.temps ?? []).length}）`)
                .fontSize(AppTheme.fontSmall).fontWeight(FontWeight.Bold).fontColor(this.tAccent).width('100%').margin({ top: 4 })
              ForEach(this.hwFocus.snapshot?.temps ?? [], (t: SensorReading, i: number) => {
                KeyValueRow({
                  label: t.name || `Temp${i}`,
                  value: `${Number(t.reading ?? 0).toFixed(0)}°C · ${FormatUtil.healthZh(t.status || '')}`
                })
              }, (t: SensorReading, i: number) => `tmp-${i}-${t.name ?? ''}`)
            }

            Text(`事件日志（${(this.hwFocus.snapshot?.events ?? []).length}）`)
              .fontSize(AppTheme.fontSmall).fontWeight(FontWeight.Bold).fontColor(this.tAccent).width('100%').margin({ top: 4 })
            ForEach((this.hwFocus.snapshot?.events ?? []).slice(0, 12), (ev: HardwareEvent, i: number) => {
              Text(`${ev.severity || 'info'} · ${ev.component || ''} · ${ev.message || ev.message_id || '—'}`)
                .fontSize(AppTheme.fontMicro).fontColor(this.tMuted).width('100%')
            }, (ev: HardwareEvent, i: number) => `hev-${i}-${ev.id ?? 0}`)

            Button('AI 诊断')
              .width('100%').height(40).margin({ top: 12 })
              .backgroundColor(this.tAccent).fontColor(this.tOnAccent)
              .onClick(() => {
                if (this.hwFocus) {
                  this.openAi('hardware_diagnosis', '硬件 AI 诊断', this.hwContext(this.hwFocus));
                }
              })
          }
        }
        .width('100%')
        .alignItems(HorizontalAlign.Start)
        .padding({ bottom: 12 })
      }

      DetailSheet({
        visible: this.snmpHistVisible,
        title: this.snmpHistTitle || '接口历史',
        onClose: () => {
          this.snmpHistVisible = false;
          this.snmpHistDevice = null;
          this.snmpHistIface = null;
        }
      }) {
        Column({ space: 8 }) {
          Text(`${this.snmpFocusDevice} · ifIndex ${this.snmpHistIface?.index ?? '—'}`)
            .fontSize(AppTheme.fontCaption).fontColor(this.tMuted).width('100%')

          FilterChips({
            labels: this.snmpHistRangeLabels,
            selected: $snmpHistRangeIdx,
            onSelect: (_i: number) => { void this.reloadSnmpIfaceHistory(); }
          })

          if (this.snmpHistText) {
            Text(this.snmpHistText)
              .fontSize(AppTheme.fontCaption).fontColor(this.tMuted).width('100%')
          }
          if (this.snmpHistInPts.length > 0) {
            HistoryChart({
              title: '入站速率',
              unit: 'bps',
              color: this.tAccent,
              points: this.snmpHistInPts,
              heightVp: 100
            })
          }
          if (this.snmpHistOutPts.length > 0) {
            HistoryChart({
              title: '出站速率',
              unit: 'bps',
              color: this.tSuccess,
              points: this.snmpHistOutPts,
              heightVp: 100
            })
          }
          if (this.snmpHistUtilInPts.length > 0) {
            HistoryChart({
              title: '入站利用率',
              unit: '%',
              color: '#8B5CF6',
              points: this.snmpHistUtilInPts,
              heightVp: 90
            })
          }
          if (this.snmpHistUtilOutPts.length > 0) {
            HistoryChart({
              title: '出站利用率',
              unit: '%',
              color: this.tWarning,
              points: this.snmpHistUtilOutPts,
              heightVp: 90
            })
          }
        }
        .width('100%')
        .alignItems(HorizontalAlign.Start)
        .padding({ bottom: 12 })
      }

      DetailSheet({
        visible: this.ipHistVisible,
        title: this.ipHistTitle || 'IP 历史',
        onClose: () => {
          this.ipHistVisible = false;
          this.ipHist = null;
          this.ipHistPts = [];
          this.ipHistIp = '';
        }
      }) {
        Column({ space: 8 }) {
          Row() {
            Text(this.nfDims[this.netDimIdx] === 'src_ip' ? '作为源 IP' : '作为目的 IP')
              .fontSize(AppTheme.fontMicro).fontColor(this.tMuted).layoutWeight(1)
            Text('相关 Flow')
              .fontSize(AppTheme.fontCaption)
              .fontColor(this.tAccent)
              .padding({ left: 10, right: 10, top: 4, bottom: 4 })
              .border({ width: 1, color: this.tAccent })
              .borderRadius(AppTheme.rXs)
              .onClick(() => { void this.loadRelatedFlows(); })
          }
          .width('100%')

          Row({ space: 12 }) {
            Column() {
              Text(FormatUtil.bytes(this.ipHistTotalBytes())).fontSize(AppTheme.fontBody).fontWeight(FontWeight.Bold).fontColor(this.tAccent)
              Text('总流量').fontSize(AppTheme.fontTiny).fontColor(this.tMuted)
            }.layoutWeight(1)
            Column() {
              Text(`${(this.ipHist?.peers ?? []).length}`).fontSize(AppTheme.fontBody).fontWeight(FontWeight.Bold).fontColor(this.tText)
              Text('对端').fontSize(AppTheme.fontTiny).fontColor(this.tMuted)
            }.layoutWeight(1)
            Column() {
              Text(`${(this.ipHist?.protocols ?? []).length}`).fontSize(AppTheme.fontBody).fontWeight(FontWeight.Bold).fontColor(this.tText)
              Text('协议').fontSize(AppTheme.fontTiny).fontColor(this.tMuted)
            }.layoutWeight(1)
            Column() {
              Text(`${(this.ipHist?.points ?? []).length}`).fontSize(AppTheme.fontBody).fontWeight(FontWeight.Bold).fontColor(this.tText)
              Text('采样点').fontSize(AppTheme.fontTiny).fontColor(this.tMuted)
            }.layoutWeight(1)
          }
          .width('100%')

          if (this.ipHistPts.length > 0) {
            HistoryChart({
              title: '流量趋势',
              unit: 'B',
              color: this.tAccent,
              points: this.ipHistPts,
              heightVp: 110
            })
          } else if (this.ipHist) {
            Text('当前时间范围内无该 IP 流量历史')
              .fontSize(AppTheme.fontCaption).fontColor(this.tMuted).width('100%')
          }

          if ((this.ipHist?.peers ?? []).length > 0) {
            Text('对端 Top').fontSize(AppTheme.fontCaption).fontWeight(FontWeight.Bold).fontColor(this.tText).width('100%')
            ForEach((this.ipHist?.peers ?? []).slice(0, 12), (row: NetFlowDrillItem, i: number) => {
              Row() {
                Text(row.key || '—').fontSize(AppTheme.fontCaption).fontColor(this.tAccent).layoutWeight(1).maxLines(1)
                Text(FormatUtil.bytes(row.bytes ?? 0)).fontSize(AppTheme.fontMicro).fontColor(this.tAccent)
              }.width('100%').padding({ top: 4, bottom: 4 })
            }, (row: NetFlowDrillItem, i: number) => `peer-${i}-${row.key ?? ''}`)
          }
          if ((this.ipHist?.protocols ?? []).length > 0) {
            Text('协议').fontSize(AppTheme.fontCaption).fontWeight(FontWeight.Bold).fontColor(this.tText).width('100%')
            ForEach((this.ipHist?.protocols ?? []).slice(0, 8), (row: NetFlowDrillItem, i: number) => {
              Row() {
                Text(this.protoName(row.key)).fontSize(AppTheme.fontCaption).fontColor(this.tText).layoutWeight(1)
                Text(FormatUtil.bytes(row.bytes ?? 0)).fontSize(AppTheme.fontMicro).fontColor(this.tAccent)
              }.width('100%').padding({ top: 4, bottom: 4 })
            }, (row: NetFlowDrillItem, i: number) => `proto-${i}-${row.key ?? ''}`)
          }
          if ((this.ipHist?.dst_ports ?? []).length > 0) {
            Text('目的端口').fontSize(AppTheme.fontCaption).fontWeight(FontWeight.Bold).fontColor(this.tText).width('100%')
            ForEach((this.ipHist?.dst_ports ?? []).slice(0, 8), (row: NetFlowDrillItem, i: number) => {
              Row() {
                Text(row.key || '—').fontSize(AppTheme.fontCaption).fontColor(this.tText).layoutWeight(1)
                Text(FormatUtil.bytes(row.bytes ?? 0)).fontSize(AppTheme.fontMicro).fontColor(this.tAccent)
              }.width('100%').padding({ top: 4, bottom: 4 })
            }, (row: NetFlowDrillItem, i: number) => `dport-${i}-${row.key ?? ''}`)
          }
        }
        .width('100%')
        .alignItems(HorizontalAlign.Start)
        .padding({ bottom: 12 })
      }

      DetailSheet({
        visible: this.histVisible,
        title: `历史 · ${this.histTitle}`,
        onClose: () => {
          this.histVisible = false;
          this.histSourceId = '';
          this.histSourceKind = '';
        }
      }) {
        Column({ space: 8 }) {
          Text('延迟 · 可用率 · 分布 · 分位')
            .fontSize(AppTheme.fontMicro).fontColor(this.tMuted).width('100%')

          FilterChips({
            labels: this.histRangeLabels,
            selected: $histRangeIdx,
            onSelect: (_i: number) => { void this.reloadHistPoints(); }
          })

          if (this.histPoints.length === 0) {
            Text('暂无历史点').fontSize(AppTheme.fontCaption).fontColor(this.tMuted)
          } else {
            Row({ space: 8 }) {
              Column() {
                Text(`${Math.round(this.histAvg())} ms`).fontSize(AppTheme.fontSmall).fontWeight(FontWeight.Bold).fontColor(this.tAccent)
                Text('平均').fontSize(AppTheme.fontTiny).fontColor(this.tMuted)
              }.layoutWeight(1)
              Column() {
                Text(`${Math.round(this.histP95())} ms`).fontSize(AppTheme.fontSmall).fontWeight(FontWeight.Bold).fontColor(this.tWarning)
                Text('P95').fontSize(AppTheme.fontTiny).fontColor(this.tMuted)
              }.layoutWeight(1)
              Column() {
                Text(`${Math.round(this.histP99())} ms`).fontSize(AppTheme.fontSmall).fontWeight(FontWeight.Bold).fontColor(this.tDanger)
                Text('P99').fontSize(AppTheme.fontTiny).fontColor(this.tMuted)
              }.layoutWeight(1)
              Column() {
                Text(`${this.histUptime().toFixed(1)}%`)
                  .fontSize(AppTheme.fontSmall).fontWeight(FontWeight.Bold)
                  .fontColor(this.histUptime() >= 99 ? this.tSuccess : this.tDanger)
                Text('可用率').fontSize(AppTheme.fontTiny).fontColor(this.tMuted)
              }.layoutWeight(1)
            }
            .width('100%')

            HistoryChart({
              title: '响应延迟',
              unit: 'ms',
              color: this.tAccent,
              points: this.histChartPts(),
              heightVp: 110
            })

            HistoryChart({
              title: '可用性（成功=100 / 失败=0）',
              unit: '%',
              color: this.tSuccess,
              points: this.histAvailPts(),
              heightVp: 90
            })

            Text('延迟分布直方图（长尾 / 双峰）')
              .fontSize(AppTheme.fontSmall).fontWeight(FontWeight.Bold).fontColor(this.tText).width('100%')
            if (this.histBuckets().length === 0) {
              Text('暂无延迟数据').fontSize(AppTheme.fontCaption).fontColor(this.tMuted)
            } else {
              ForEach(this.histBuckets(), (line: string, i: number) => {
                Text(line)
                  .fontSize(AppTheme.fontMicro)
                  .fontColor(this.tMuted)
                  .fontFamily('monospace')
                  .width('100%')
              }, (line: string, i: number) => `hb-${i}`)
            }

            if (this.histHasBytes()) {
              HistoryChart({
                title: '响应体大小（KB）',
                unit: 'KB',
                color: '#8B5CF6',
                points: this.histBytesPts(),
                heightVp: 90
              })
            }

            if (this.histHasPhase()) {
              Text('分阶段耗时')
                .fontSize(AppTheme.fontSmall).fontWeight(FontWeight.Bold).fontColor(this.tText).width('100%').margin({ top: 4 })
              if (this.histPhasePts('dns').length > 0) {
                HistoryChart({
                  title: 'DNS 解析',
                  unit: 'ms',
                  color: this.tAccent,
                  points: this.histPhasePts('dns'),
                  heightVp: 80
                })
              }
              if (this.histPhasePts('tcp').length > 0) {
                HistoryChart({
                  title: 'TCP 建连',
                  unit: 'ms',
                  color: this.tSuccess,
                  points: this.histPhasePts('tcp'),
                  heightVp: 80
                })
              }
              if (this.histPhasePts('tls').length > 0) {
                HistoryChart({
                  title: 'TLS 握手',
                  unit: 'ms',
                  color: this.tWarning,
                  points: this.histPhasePts('tls'),
                  heightVp: 80
                })
              }
              if (this.histPhasePts('ttfb').length > 0) {
                HistoryChart({
                  title: 'TTFB',
                  unit: 'ms',
                  color: '#8B5CF6',
                  points: this.histPhasePts('ttfb'),
                  heightVp: 80
                })
              }
            }

            Text('采样明细').fontSize(AppTheme.fontCaption).fontWeight(FontWeight.Bold).fontColor(this.tText).width('100%').margin({ top: 4 })
            ForEach(this.histPoints.slice(0, 40), (pt: CheckPoint, i: number) => {
              Row() {
                Text(pt.ok === false ? '失败' : '成功')
                  .fontSize(AppTheme.fontMicro)
                  .fontColor(pt.ok === false ? this.tDanger : this.tSuccess)
                  .width(40)
                Text(`${Math.round(pt.latency_ms ?? 0)} ms`)
                  .fontSize(AppTheme.fontCaption)
                  .fontColor(FormatUtil.latencyColor(pt.latency_ms ?? 0))
                  .layoutWeight(1)
                if ((pt.status_code ?? 0) > 0) {
                  Text(`HTTP ${pt.status_code}`).fontSize(AppTheme.fontTiny).fontColor(this.tMuted).margin({ right: 8 })
                }
                Text(FormatUtil.relTime(pt.ts ?? pt.timestamp ?? 0))
                  .fontSize(AppTheme.fontMicro)
                  .fontColor(this.tMuted)
              }
              .width('100%')
              .padding({ top: 6, bottom: 6 })
            }, (pt: CheckPoint, i: number) => `hp-${i}-${pt.ts ?? pt.timestamp ?? 0}`)
          }
        }
        .width('100%')
        .alignItems(HorizontalAlign.Start)
        .padding({ bottom: 12 })
      }

      if (this.probeFormVisible) {
        Column() {
          Blank().layoutWeight(1).onClick(() => {
            if (!this.probeBusy) {
              this.probeFormVisible = false;
            }
          })
          Column() {
            Text(this.probeEditId ? '编辑拨测' : '新建拨测')
              .fontSize(AppTheme.fontLead).fontWeight(FontWeight.Bold).fontColor(this.tText).margin({ bottom: 12 })
            TextInput({ text: this.probeName, placeholder: '名称' })
              .height(40).margin({ bottom: 8 }).backgroundColor(this.tElevated).fontColor(this.tText)
              .onChange((v: string) => { this.probeName = v; })
            TextInput({ text: this.probeTarget, placeholder: '目标 URL / 主机:端口' })
              .height(40).margin({ bottom: 8 }).backgroundColor(this.tElevated).fontColor(this.tText)
              .onChange((v: string) => { this.probeTarget = v; })
            Text(`间隔（秒）：${this.probeInterval}`)
              .fontSize(AppTheme.fontCaption).fontColor(this.tMuted).margin({ bottom: 4 })
            Row({ space: 8 }) {
              ForEach([30, 60, 120, 300], (sec: number) => {
                Text(`${sec}s`)
                  .fontSize(AppTheme.fontMicro)
                  .fontColor(this.probeInterval === sec ? this.tOnAccent : this.tMuted)
                  .padding({ left: 8, right: 8, top: 5, bottom: 5 })
                  .backgroundColor(this.probeInterval === sec ? this.tAccent : this.tElevated)
                  .borderRadius(AppTheme.rXs)
                  .onClick(() => { this.probeInterval = sec; })
              }, (sec: number) => `iv-${sec}`)
            }
            .margin({ bottom: 10 })
            Row({ space: 8 }) {
              ForEach(this.probeTypesAll, (ty: string, i: number) => {
                Text(ty.toUpperCase())
                  .fontSize(AppTheme.fontMicro)
                  .fontColor(this.probeTypeSel === i ? this.tOnAccent : this.tMuted)
                  .padding({ left: 8, right: 8, top: 5, bottom: 5 })
                  .backgroundColor(this.probeTypeSel === i ? this.tAccent : this.tElevated)
                  .borderRadius(AppTheme.rXs)
                  .onClick(() => { this.probeTypeSel = i; })
              }, (ty: string, i: number) => `pt-${ty}`)
            }
            .margin({ bottom: 10 })
            if ((this.probeTypesAll[this.probeTypeSel] || '') === 'dns') {
              Row({ space: 8 }) {
                ForEach(this.dnsTypes, (dt: string) => {
                  Text(dt)
                    .fontSize(AppTheme.fontMicro)
                    .fontColor(this.probeDnsType === dt ? this.tOnAccent : this.tMuted)
                    .padding({ left: 8, right: 8, top: 5, bottom: 5 })
                    .backgroundColor(this.probeDnsType === dt ? this.tAccent : this.tElevated)
                    .borderRadius(AppTheme.rXs)
                    .onClick(() => { this.probeDnsType = dt; })
                }, (dt: string) => `dns-${dt}`)
              }
              .margin({ bottom: 8 })
              TextInput({ text: this.probeExpect, placeholder: '期望关键字(可选)，如某 IP' })
                .height(40).margin({ bottom: 8 }).backgroundColor(this.tElevated).fontColor(this.tText)
                .onChange((v: string) => { this.probeExpect = v; })
              Text(`超时（秒）：${this.probeTimeout}`)
                .fontSize(AppTheme.fontCaption).fontColor(this.tMuted).margin({ bottom: 4 })
              Row({ space: 8 }) {
                ForEach([3, 5, 10, 15], (sec: number) => {
                  Text(`${sec}s`)
                    .fontSize(AppTheme.fontMicro)
                    .fontColor(this.probeTimeout === sec ? this.tOnAccent : this.tMuted)
                    .padding({ left: 8, right: 8, top: 5, bottom: 5 })
                    .backgroundColor(this.probeTimeout === sec ? this.tAccent : this.tElevated)
                    .borderRadius(AppTheme.rXs)
                    .onClick(() => { this.probeTimeout = sec; })
                }, (sec: number) => `to-${sec}`)
              }
              .margin({ bottom: 12 })
            }
            Row({ space: 8 }) {
              Text('级别').fontSize(AppTheme.fontCaption).fontColor(this.tMuted)
              Text('警告')
                .fontSize(AppTheme.fontMicro)
                .fontColor(this.probeLevel === 'warning' ? this.tOnAccent : this.tMuted)
                .padding({ left: 8, right: 8, top: 5, bottom: 5 })
                .backgroundColor(this.probeLevel === 'warning' ? this.tWarning : this.tElevated)
                .borderRadius(AppTheme.rXs)
                .onClick(() => { this.probeLevel = 'warning'; })
              Text('严重')
                .fontSize(AppTheme.fontMicro)
                .fontColor(this.probeLevel === 'critical' ? this.tOnAccent : this.tMuted)
                .padding({ left: 8, right: 8, top: 5, bottom: 5 })
                .backgroundColor(this.probeLevel === 'critical' ? this.tDanger : this.tElevated)
                .borderRadius(AppTheme.rXs)
                .onClick(() => { this.probeLevel = 'critical'; })
            }
            .width('100%')
            .margin({ bottom: 10 })
            Row() {
              Text('启用').fontSize(AppTheme.fontSmall).fontColor(this.tText).layoutWeight(1)
              Toggle({ type: ToggleType.Switch, isOn: this.probeEnabled })
                .selectedColor(this.tAccent)
                .onChange((on: boolean) => { this.probeEnabled = on; })
            }
            .width('100%')
            .margin({ bottom: 12 })
            Row({ space: 12 }) {
              Button('取消').layoutWeight(1).height(40).backgroundColor(this.tElevated).fontColor(this.tText)
                .onClick(() => { this.probeFormVisible = false; })
              Button(this.probeBusy ? '保存中…' : '保存').layoutWeight(1).height(40).enabled(!this.probeBusy)
                .backgroundColor(this.tAccent).fontColor(this.tOnAccent)
                .onClick(() => { void this.saveProbe(); })
            }
            .width('100%')
          }
          .width('88%')
          .padding(AppTheme.pagePadH)
          .backgroundColor(this.tCard)
          .borderRadius(AppTheme.rSm)
          .border({ width: 1, color: this.tBorderSoft })
          Blank().layoutWeight(1)
        }
        .width('100%').height('100%').justifyContent(FlexAlign.Center)
        .backgroundColor(this.tScrim).zIndex(96)
      }

      if (this.dnsThVisible) {
        Column() {
          Blank().layoutWeight(1).onClick(() => {
            if (!this.dnsThBusy) {
              this.dnsThVisible = false;
            }
          })
          Column() {
            Text('DNS 解析延迟阈值')
              .fontSize(AppTheme.fontLead).fontWeight(FontWeight.Bold).fontColor(this.tText).margin({ bottom: 8 })
            Text('DNS 拨测的解析延迟达到阈值即产生对应级别的延迟告警（全局，作用于所有 DNS 拨测）。')
              .fontSize(AppTheme.fontCaption).fontColor(this.tMuted).margin({ bottom: 12 }).width('100%')
            TextInput({ text: this.dnsWarnMs, placeholder: '警告阈值 (ms)' })
              .height(40).margin({ bottom: 8 }).backgroundColor(this.tElevated).fontColor(this.tText)
              .onChange((v: string) => { this.dnsWarnMs = this.digitsOnly(v, 6); })
            TextInput({ text: this.dnsCritMs, placeholder: '严重阈值 (ms)' })
              .height(40).margin({ bottom: 12 }).backgroundColor(this.tElevated).fontColor(this.tText)
              .onChange((v: string) => { this.dnsCritMs = this.digitsOnly(v, 6); })
            Row({ space: 12 }) {
              Button('取消').layoutWeight(1).height(40).backgroundColor(this.tElevated).fontColor(this.tText)
                .enabled(!this.dnsThBusy)
                .onClick(() => { this.dnsThVisible = false; })
              Button(this.dnsThBusy ? '保存中…' : '保存').layoutWeight(1).height(40)
                .enabled(!this.dnsThBusy && this.dnsWarnMs.length > 0 && this.dnsCritMs.length > 0)
                .backgroundColor(this.tAccent).fontColor(this.tOnAccent)
                .onClick(() => { void this.saveDnsThresholds(); })
            }
            .width('100%')
          }
          .width('88%')
          .padding(AppTheme.pagePadH)
          .backgroundColor(this.tCard)
          .borderRadius(AppTheme.rSm)
          .border({ width: 1, color: this.tBorderSoft })
          Blank().layoutWeight(1)
        }
        .width('100%').height('100%').justifyContent(FlexAlign.Center)
        .backgroundColor(this.tScrim).zIndex(97)
      }

      ConfirmDialogHost({
        visible: this.hvExportVisible,
        title: '导出 Hyper-V 清单',
        message: '将生成 CSV（Excel 可打开）并复制到剪贴板',
        confirmText: '复制 CSV',
        onConfirm: () => { this.exportHvCsv(); },
        onCancel: () => { this.hvExportVisible = false; }
      })

      ConfirmDialogHost({
        visible: this.probeDeleteVisible,
        title: '删除拨测？',
        message: '删除后不可恢复',
        confirmText: '删除',
        danger: true,
        onConfirm: () => { void this.doDeleteProbe(); },
        onCancel: () => { this.probeDeleteVisible = false; }
      })

      ConfirmDialogHost({
        visible: this.apiDeleteVisible,
        title: '删除 API 系统？',
        message: '将删除该系统及其端点配置',
        confirmText: '删除',
        danger: true,
        onConfirm: () => { void this.doDeleteApi(); },
        onCancel: () => { this.apiDeleteVisible = false; }
      })

      AiAssistSheet({
        visible: this.aiAssistVisible,
        title: this.aiTitle,
        task: this.aiTask,
        context: this.aiContext,
        onClose: () => { this.aiAssistVisible = false; }
      })

      ToastHost({ message: this.toastMsg, isError: this.toastError })
    }
    .width('100%')
    .height('100%')
  }

  @Builder
  hostPicker() {
    Scroll() {
      Row({ space: 8 }) {
        ForEach(this.hosts, (h: Host) => {
          Text(h.hostname || h.id)
            .fontSize(AppTheme.fontMicro)
            .fontColor(this.selectedHost === h.id ? this.tOnAccent : this.tMuted)
            .padding({ left: 10, right: 10, top: 5, bottom: 5 })
            .backgroundColor(this.selectedHost === h.id ? this.tAccent : this.tElevated)
            .borderRadius(AppTheme.rSm)
            .onClick(() => {
              this.selectedHost = h.id;
              void this.refresh();
            })
        }, (h: Host) => `hp-${h.id}`)
      }
    }
    .scrollable(ScrollDirection.Horizontal)
    .scrollBar(BarState.Off)
    .height(36)
    .padding({ left: 12, right: 12, bottom: 8 })
  }

  @Builder
  netHostPicker() {
    Column() {
      Row() {
        Text(this.showAllHosts ? '显示全部' : '仅显示有数据')
          .fontSize(AppTheme.fontMicro)
          .fontColor(this.tAccent)
          .onClick(() => {
            this.showAllHosts = !this.showAllHosts;
            void this.refresh();
          })
        Blank()
        if (this.netSub === 0 || this.netSub === 1) {
          Text('AI 诊断')
            .fontSize(AppTheme.fontMicro)
            .fontColor(this.tOnAccent)
            .padding({ left: 10, right: 10, top: 4, bottom: 4 })
            .backgroundColor(this.tAccent)
            .borderRadius(AppTheme.rXs)
            .onClick(() => {
              if (this.netSub === 0) {
                const ctx = this.snmpDevices.map((d: SNMPDevice) =>
                  `${this.snmpName(d)}/${this.snmpIp(d)} reachable=${d.reachable}`).join('\n');
                this.openAi('snmp_diagnosis', 'SNMP AI 诊断', ctx || `host=${this.selectedHost}`);
              } else {
                const ctx = this.netflowItems.map((it: NetFlowSummaryItem) =>
                  `${it.key}: ${it.bytes ?? 0}B`).join('\n');
                this.openAi('netflow_diagnosis', '流量 AI 诊断', ctx || `host=${this.selectedHost}`);
              }
            })
        }
      }
      .width('100%')
      .padding({ left: 12, right: 12, bottom: 6 })

      Scroll() {
        Row({ space: 8 }) {
          ForEach(this.netPickerHosts(), (h: Host) => {
            Text(h.hostname || h.id)
              .fontSize(AppTheme.fontMicro)
              .fontColor(this.selectedHost === h.id ? this.tOnAccent : this.tMuted)
              .padding({ left: 10, right: 10, top: 5, bottom: 5 })
              .backgroundColor(this.selectedHost === h.id ? this.tAccent : this.tElevated)
              .borderRadius(AppTheme.rSm)
              .onClick(() => {
                this.selectedHost = h.id;
                void this.refresh();
              })
          }, (h: Host) => `nhp-${h.id}`)
        }
      }
      .scrollable(ScrollDirection.Horizontal)
      .scrollBar(BarState.Off)
      .height(36)
      .padding({ left: 12, right: 12, bottom: 8 })
    }
  }

  @Builder
  hostsPane() {
    // Android HostListContent：单列 LazyColumn（筛选/GPU 概览/主机卡一体滚动），避免嵌套 List 裁切
    Column() {
      SearchBar({
        text: $hostQuery,
        placeholder: '搜索主机名或 IP'
      })
        .padding({ left: 16, right: 16, bottom: 8 })

      Scroll() {
        Column() {
          Scroll() {
            Row({ space: 8 }) {
              Text(`全部 ${this.hosts.length}`)
                .fontSize(AppTheme.fontCaption)
                .fontWeight(this.selectedCategory.length === 0 && this.hostQuickFilter === 0 ? FontWeight.Medium : FontWeight.Regular)
                .fontColor(this.selectedCategory.length === 0 && this.hostQuickFilter === 0 ? this.tAccent : this.tMuted)
                .padding({ left: 12, right: 12, top: 7, bottom: 7 })
                .backgroundColor(this.selectedCategory.length === 0 && this.hostQuickFilter === 0 ? '#334F7FFF' : Color.Transparent)
                .borderRadius(AppTheme.rMd)
                .onClick(() => {
                  this.selectedCategory = '';
                  this.hostQuickFilter = 0;
                })
              Text(`仅在线 ${this.onlineCount()}`)
                .fontSize(AppTheme.fontCaption)
                .fontWeight(this.hostQuickFilter === 1 ? FontWeight.Medium : FontWeight.Regular)
                .fontColor(this.hostQuickFilter === 1 ? this.tAccent : this.tMuted)
                .padding({ left: 12, right: 12, top: 7, bottom: 7 })
                .backgroundColor(this.hostQuickFilter === 1 ? '#334F7FFF' : Color.Transparent)
                .borderRadius(AppTheme.rMd)
                .onClick(() => {
                  this.hostQuickFilter = this.hostQuickFilter === 1 ? 0 : 1;
                })
              ForEach(this.hostCategories(), (cat: string) => {
                Text(`${cat} ${this.categoryCount(cat)}`)
                  .fontSize(AppTheme.fontCaption)
                  .fontWeight(this.selectedCategory === cat ? FontWeight.Medium : FontWeight.Regular)
                  .fontColor(this.selectedCategory === cat ? this.tAccent : this.tMuted)
                  .padding({ left: 12, right: 12, top: 7, bottom: 7 })
                  .backgroundColor(this.selectedCategory === cat ? '#334F7FFF' : Color.Transparent)
                  .borderRadius(AppTheme.rMd)
                  .onClick(() => {
                    this.selectedCategory = this.selectedCategory === cat ? '' : cat;
                  })
              }, (cat: string) => `cat-${cat}`)
            }
          }
          .scrollable(ScrollDirection.Horizontal)
          .scrollBar(BarState.Off)
          .width('100%')
          .padding({ left: 16, right: 16, bottom: 10 })

          if (this.gpuRanking().length > 0) {
            Column({ space: 10 }) {
              Row() {
                Column() {
                  Text('GPU 资源概览')
                    .fontSize(AppTheme.fontBody)
                    .fontWeight(FontWeight.Bold)
                    .fontColor(this.tText)
                  Text(`${this.gpuHostCount()} 台加速主机 · ${this.gpuRanking(999).length} 块设备`)
                    .fontSize(AppTheme.fontMicro)
                    .fontColor(this.tMuted)
                    .margin({ top: 2 })
                }
                .alignItems(HorizontalAlign.Start)
                .layoutWeight(1)
                Text(`峰值 ${Math.round(this.gpuPeakUtil())}%`)
                  .fontSize(AppTheme.fontMicro)
                  .fontWeight(FontWeight.Bold)
                  .fontColor(FormatUtil.thresholdColor(this.gpuPeakUtil()))
                  .padding({ left: 8, right: 8, top: 3, bottom: 3 })
                  .backgroundColor(this.tElevated)
                  .borderRadius(AppTheme.rChip)
              }
              .width('100%')

              Divider().color(this.tBorderSoft).strokeWidth(0.5)

              // Android GpuOverviewCard：内部限高 255dp 可滚，避免挤掉主机列表
              Scroll() {
                Column({ space: 0 }) {
                  ForEach(this.gpuRanking(999), (row: GpuRankRow, i: number) => {
                    Column({ space: 6 }) {
                      Row() {
                        Text(row.hostname)
                          .fontSize(AppTheme.fontCaption)
                          .fontWeight(FontWeight.Medium)
                          .fontColor(this.tText)
                          .layoutWeight(1)
                          .maxLines(1)
                          .textOverflow({ overflow: TextOverflow.Ellipsis })
                        Text(row.gpuName)
                          .fontSize(AppTheme.fontTiny)
                          .fontColor(this.tMuted)
                          .maxLines(1)
                      }
                      .width('100%')
                      Row({ space: 12 }) {
                        MetricBar({ label: 'GPU', value: row.util, compact: true }).layoutWeight(1)
                        MetricBar({ label: '显存', value: row.mem, compact: true }).layoutWeight(1)
                        MetricBar({
                          label: '温度',
                          value: row.temp > 0 ? row.temp : 0,
                          valueText: row.temp > 0 ? `${Math.round(row.temp)}°C` : '—',
                          compact: true
                        }).layoutWeight(1)
                      }
                      .width('100%')
                      if (i < this.gpuRanking(999).length - 1) {
                        Divider().color(this.tBorderSoft).strokeWidth(0.5).margin({ top: 4, bottom: 4 })
                      }
                    }
                    .width('100%')
                    .onClick(() => this.onOpenHost(row.hostId))
                  }, (row: GpuRankRow, i: number) => `gpu-${row.hostId}-${i}-${row.gpuName}`)
                }
                .width('100%')
              }
              .scrollBar(BarState.Off)
              .constraintSize({ maxHeight: 255 })
              .nestedScroll({
                scrollForward: NestedScrollMode.PARENT_FIRST,
                scrollBackward: NestedScrollMode.SELF_FIRST
              })
            }
            .width('100%')
            .padding(AppTheme.cardPad)
            .margin({ left: 16, right: 16, bottom: 10 })
            .backgroundColor(this.tCard)
            .borderRadius(AppTheme.rSm)
          }

          Row() {
            Text(`主机 ${this.filteredHosts().length}/${this.hosts.length}`)
              .fontSize(AppTheme.fontLead)
              .fontWeight(FontWeight.Bold)
              .fontColor(this.tText)
            Blank()
            if (this.loadingHosts) {
              LoadingProgress().width(16).height(16).color(this.tAccent)
            }
          }
          .width('100%')
          .padding({ left: 16, right: 16, bottom: 8 })

          if (this.filteredHosts().length === 0) {
            Column({ space: 12 }) {
              AppSymbol({ kind: 'logs', tint: this.tMuted, iconSize: 36 })
              Text('没有匹配的主机')
                .fontSize(AppTheme.fontBody)
                .fontWeight(FontWeight.Bold)
                .fontColor(this.tText)
              Text('清除筛选')
                .fontSize(AppTheme.fontSmall)
                .fontColor(this.tAccent)
                .onClick(() => this.clearHostFilters())
            }
            .width('100%')
            .padding(32)
            .margin({ left: 16, right: 16, bottom: 16 })
            .backgroundColor(this.tCard)
            .borderRadius(AppTheme.rSm)
            .alignItems(HorizontalAlign.Center)
          } else {
            ForEach(this.filteredHosts(), (h: Host) => {
              HostCard({
                hostname: h.hostname || h.id,
                ip: h.ip || '',
                online: !!h.online,
                cpu: this.pct(h.latest?.cpu_percent),
                mem: this.pct(h.latest?.mem_percent),
                disk: this.pct(h.latest?.disk_percent),
                cpuPct: h.latest?.cpu_percent ?? -1,
                memPct: h.latest?.mem_percent ?? -1,
                diskPct: h.latest?.disk_percent ?? -1,
                category: h.category || '',
                gpuCount: this.hostGpuCount(h),
                gpuUtil: this.hostGpuMax(h, 'util'),
                gpuMem: this.hostGpuMax(h, 'mem'),
                gpuTemp: this.hostGpuMax(h, 'temp'),
                gpuDetail: this.hostGpuDetail(h),
                hasLatest: !!h.latest,
                onTap: () => this.onOpenHost(h.id)
              })
                .width('100%')
                .padding({ left: 16, right: 16, bottom: 12 })
            }, (h: Host) => h.id)
          }

          Blank().height(8)
        }
        .width('100%')
      }
      .layoutWeight(1)
      .scrollBar(BarState.Off)
      .align(Alignment.Top)
      .edgeEffect(EdgeEffect.Spring)
    }
    .width('100%')
    .layoutWeight(1)
  }

  @Builder
  resourcePane() {
    Column() {
      FilterChips({
        labels: this.resLabels,
        selected: $resSub,
        variant: 'subtab',
        onSelect: (_i: number) => {
          void this.refresh();
        }
      })
        .padding({ left: 8, right: 8, bottom: 6 })

      if (this.resSub === 0) {
        SearchBar({
          text: $hwQuery,
          placeholder: '搜索目标名 / 型号 / 序列号'
        })
          .padding({ left: 12, right: 12, bottom: 8 })

        if (this.hwSnapshots.length === 0) {
          EmptyHint({ text: '暂无硬件数据（需在 Agent 配置 Redfish/OceanStor 目标）' })
        } else if (this.filteredHw().length === 0) {
          EmptyHint({ text: '没有匹配的硬件目标' })
        } else {
          Scroll() {
            Column() {
              ForEach(this.filteredHw(), (hw: HardwareSnapshotWrap, i: number) => {
                Column() {
                  Row() {
                    Text(hw.target_name || `节点 ${i + 1}`)
                      .fontSize(AppTheme.fontBody)
                      .fontWeight(FontWeight.Bold)
                      .fontColor(this.tText)
                      .layoutWeight(1)
                      .maxLines(1)
                      .textOverflow({ overflow: TextOverflow.Ellipsis })
                    Text(FormatUtil.healthZh(hw.health || hw.snapshot?.state || ''))
                      .fontSize(AppTheme.fontMicro)
                      .fontColor(this.statusColor(hw.health || hw.snapshot?.state || ''))
                      .padding({ left: 8, right: 8, top: 2, bottom: 2 })
                      .backgroundColor(this.tElevated)
                      .borderRadius(AppTheme.rChip)
                  }
                  .width('100%')
                  Text(this.manufacturerModel(hw))
                    .fontSize(AppTheme.fontCaption).fontColor(this.tMuted).margin({ top: 6 })
                  if (hw.snapshot?.system?.serial_number) {
                    Text(`序列号 ${hw.snapshot?.system?.serial_number}`)
                      .fontSize(AppTheme.fontMicro).fontColor(this.tMuted).margin({ top: 2 })
                  }
                  Text(`CPU ${this.snapCpuCount(hw.snapshot)} · 内存 ${this.snapMemGb(hw.snapshot)} GB · 磁盘 ${this.snapDiskCount(hw.snapshot)}`)
                    .fontSize(AppTheme.fontCaption).fontColor(this.tText).margin({ top: 4 })
                  Flex({ wrap: FlexWrap.Wrap }) {
                    Text(`风扇 ${this.snapFanCount(hw.snapshot)}`)
                      .fontSize(AppTheme.fontTiny).fontColor(this.tMuted)
                      .padding({ left: 8, right: 8, top: 3, bottom: 3 })
                      .backgroundColor(this.tElevated).borderRadius(AppTheme.rChip)
                      .margin({ right: 6, top: 6 })
                    Text(`温度 ${this.snapTempCount(hw.snapshot)}`)
                      .fontSize(AppTheme.fontTiny).fontColor(this.tMuted)
                      .padding({ left: 8, right: 8, top: 3, bottom: 3 })
                      .backgroundColor(this.tElevated).borderRadius(AppTheme.rChip)
                      .margin({ right: 6, top: 6 })
                    Text(`功耗 ${this.snapPowerW(hw.snapshot)} W`)
                      .fontSize(AppTheme.fontTiny).fontColor(this.tMuted)
                      .padding({ left: 8, right: 8, top: 3, bottom: 3 })
                      .backgroundColor(this.tElevated).borderRadius(AppTheme.rChip)
                      .margin({ right: 6, top: 6 })
                    Text(`异常 ${this.badParts(hw.snapshot).length}`)
                      .fontSize(AppTheme.fontTiny)
                      .fontColor(this.badParts(hw.snapshot).length > 0 ? this.tDanger : this.tSuccess)
                      .padding({ left: 8, right: 8, top: 3, bottom: 3 })
                      .backgroundColor(this.badParts(hw.snapshot).length > 0 ? this.tDangerSoft : this.tSuccessSoft)
                      .borderRadius(AppTheme.rChip)
                      .margin({ top: 6 })
                  }
                  .width('100%')
                  Text('点击查看详情 ›')
                    .fontSize(AppTheme.fontMicro)
                    .fontColor(this.tAccent)
                    .margin({ top: 8 })
                }
                .width('100%')
                .alignItems(HorizontalAlign.Start)
                .padding(AppTheme.spaceMd)
                .margin({ left: 12, right: 12, bottom: 8 })
                .backgroundColor(this.tCard)
                .borderRadius(AppTheme.rSm)
                .border({ width: 1, color: this.tBorderSoft })
                .onClick(() => {
                  this.hwFocus = hw;
                  this.hwSheetVisible = true;
                })
              }, (hw: HardwareSnapshotWrap, i: number) => `hws-${i}-${hw.target_name ?? ''}`)
              Blank().height(12)
            }
            .width('100%')
          }
          .layoutWeight(1)
          .scrollBar(BarState.Off)
          .align(Alignment.Top)
        }
      } else {
        SearchBar({
          text: $hvQuery,
          placeholder: '搜索 VM 名称 / IP / 状态'
        })
          .padding({ left: 12, right: 12, bottom: 6 })

        Row({ space: 8 }) {
          FilterChips({
            labels: this.hvFilterLabels,
            selected: $hvFilter
          })
            .layoutWeight(1)
          Text('导出')
            .fontSize(AppTheme.fontCaption)
            .fontColor(this.tAccent)
            .padding({ left: 12, right: 12, top: 7, bottom: 7 })
            .border({ width: 1, color: this.tAccent })
            .borderRadius(AppTheme.rMd)
            .onClick(() => {
              this.hvExportVisible = true;
            })
        }
        .width('100%')
        .padding({ left: 8, right: 12, bottom: 8 })

        if (this.hvInventories.length === 0) {
          EmptyHint({ text: '暂无 Hyper-V 宿主机数据\n请确认物理机已安装并更新 Agent' })
        } else if (this.visibleHvInventories().length === 0) {
          EmptyHint({ text: '没有匹配的虚拟机\n调整上方筛选标签可恢复全部' })
        } else {
          Scroll() {
            Column() {
              ForEach(this.visibleHvInventories(), (inv: HyperVInventory, ii: number) => {
                Column({ space: 8 }) {
                  Column() {
                    Text(inv.host_name || inv.host_id || `宿主机 ${ii + 1}`)
                      .fontSize(AppTheme.fontSubtitle)
                      .fontWeight(FontWeight.Bold)
                      .fontColor(this.tText)
                    Text(`${(inv.guests ?? []).length} 台客户机 · ${this.hvRunningCount(inv)} 运行中`)
                      .fontSize(AppTheme.fontMicro)
                      .fontColor(this.tMuted)
                      .margin({ top: 2 })
                  }
                  .width('100%')
                  .alignItems(HorizontalAlign.Start)
                  .padding({ left: 16, right: 16, top: 4, bottom: 2 })

                  ForEach(inv.guests ?? [], (vm: HyperVGuest, i: number) => {
                    Column({ space: 8 }) {
                      Row() {
                        Text(vm.name || vm.id || `VM ${i + 1}`)
                          .fontSize(AppTheme.fontBody)
                          .fontWeight(FontWeight.Bold)
                          .fontColor(this.tText)
                          .layoutWeight(1)
                          .maxLines(1)
                          .textOverflow({ overflow: TextOverflow.Ellipsis })
                        Text(this.hvStateText(vm.state))
                          .fontSize(AppTheme.fontMicro)
                          .fontWeight(FontWeight.Medium)
                          .fontColor(vm.state === 'Running' ? this.tSuccess : this.tMuted)
                          .padding({ left: 8, right: 8, top: 2, bottom: 2 })
                          .backgroundColor(vm.state === 'Running' ? this.tSuccessSoft : this.tElevated)
                          .borderRadius(AppTheme.rChip)
                      }
                      .width('100%')
                      Row({ space: 8 }) {
                        Column() {
                          Text(vm.state === 'Running' ? this.hvUptime(vm.uptime_sec) : '—')
                            .fontSize(AppTheme.fontSmall).fontWeight(FontWeight.Bold).fontColor(this.tText)
                          Text('运行').fontSize(AppTheme.fontTiny).fontColor(this.tMuted)
                        }.layoutWeight(1)
                        Column() {
                          Text(`${Math.round(vm.cpu_usage ?? 0)}%`)
                            .fontSize(AppTheme.fontSmall).fontWeight(FontWeight.Bold).fontColor(this.tText)
                          Text(`vCPU ${vm.processor_count ?? 0}`).fontSize(AppTheme.fontTiny).fontColor(this.tMuted)
                        }.layoutWeight(1)
                        Column() {
                          Text(`${Math.round(vm.mem_assigned_mb ?? 0)}`)
                            .fontSize(AppTheme.fontSmall).fontWeight(FontWeight.Bold).fontColor(this.tText)
                          Text('内存 MB').fontSize(AppTheme.fontTiny).fontColor(this.tMuted)
                        }.layoutWeight(1)
                      }
                      .width('100%')
                      if ((vm.ip_addresses ?? []).length > 0) {
                        Text((vm.ip_addresses ?? []).join(' · '))
                          .fontSize(AppTheme.fontMicro).fontColor(this.tMuted).width('100%')
                      }
                      if (vm.linked_host_name || vm.linked_host_id) {
                        Text(`关联 Agent：${vm.linked_host_name || vm.linked_host_id}`)
                          .fontSize(AppTheme.fontMicro).fontColor(this.tAccent).width('100%')
                      }
                    }
                    .width('100%')
                    .alignItems(HorizontalAlign.Start)
                    .padding(AppTheme.spaceMd)
                    .margin({ left: 12, right: 12, bottom: 8 })
                    .backgroundColor(this.tCard)
                    .borderRadius(AppTheme.rSm)
                    .border({ width: 1, color: this.tBorderSoft })
                    .onClick(() => {
                      this.vmFocus = vm;
                      this.vmFocusHost = inv.host_name || inv.host_id || '';
                      this.vmSheetVisible = true;
                    })
                  }, (vm: HyperVGuest, i: number) => `vm-${inv.host_id ?? ii}-${vm.id ?? i}-${vm.name ?? ''}`)
                }
                .width('100%')
                .margin({ bottom: 8 })
              }, (inv: HyperVInventory, ii: number) => `inv-${inv.host_id ?? ii}`)
              Blank().height(12)
            }
            .width('100%')
          }
          .layoutWeight(1)
          .scrollBar(BarState.Off)
          .align(Alignment.Top)
        }
      }
    }
    .width('100%')
    .layoutWeight(1)
  }

  @Builder
  networkPane() {
    Column() {
      this.netHostPicker()
      FilterChips({
        labels: this.netLabels,
        selected: $netSub,
        variant: 'subtab',
        onSelect: (_i: number) => {
          void this.refresh();
        }
      })
        .padding({ left: 8, right: 8, bottom: 8 })

      if (this.netSub === 0) {
        this.snmpDevicesPane()
      } else if (this.netSub === 1) {
        this.netflowPane()
      } else if (this.netSub === 2) {
        this.trapsPane()
      } else {
        this.auditPane()
      }
    }
    .width('100%')
    .layoutWeight(1)
  }

  @Builder
  snmpDevicesPane() {
    Column() {
      if (this.snmpDevices.length === 0) {
        EmptyHint({ text: '暂无 SNMP 监控设备数据\n请在本地端配置 SNMP 查询目标（交换机/路由器）并上报' })
      } else {
        FilterChips({
          labels: this.ifaceFilterLabels,
          selected: $ifaceFilter
        })
          .padding({ left: 12, right: 12, bottom: 6 })

        Scroll() {
          Column() {
            ForEach(this.snmpDevices, (d: SNMPDevice, i: number) => {
              Column() {
                Row() {
                  StatusDot({ online: d.reachable === true }).margin({ right: 8 })
                  Column() {
                    Text(this.snmpName(d))
                      .fontSize(AppTheme.fontBody).fontColor(this.tText).fontWeight(FontWeight.Bold)
                      .maxLines(1).textOverflow({ overflow: TextOverflow.Ellipsis })
                    Text([this.snmpIp(d), d.snapshot?.system?.name || ''].filter((s: string) => !!s).join(' · '))
                      .fontSize(AppTheme.fontMicro).fontColor(this.tMuted).margin({ top: 2 })
                  }
                  .alignItems(HorizontalAlign.Start)
                  .layoutWeight(1)
                  if (d.reachable === false) {
                    Text('不可达')
                      .fontSize(AppTheme.fontMicro).fontColor(this.tDanger)
                      .padding({ left: 8, right: 8, top: 2, bottom: 2 })
                      .backgroundColor(this.tDangerSoft).borderRadius(AppTheme.rChip)
                  }
                }
                .width('100%')

                Scroll() {
                  Row({ space: 8 }) {
                    Text(`接口 ${(d.snapshot?.interfaces ?? []).length}`)
                      .fontSize(AppTheme.fontMicro)
                      .fontColor(this.ifaceFilter === 0 ? this.tAccent : this.tMuted)
                      .padding({ left: 10, right: 10, top: 5, bottom: 5 })
                      .backgroundColor(this.ifaceFilter === 0 ? '#334F7FFF' : this.tElevated)
                      .borderRadius(AppTheme.rXs)
                      .onClick(() => { this.ifaceFilter = 0; })
                    Text(`UP ${this.snmpUpCount(d)}`)
                      .fontSize(AppTheme.fontMicro)
                      .fontColor(this.ifaceFilter === 1 ? this.tSuccess : this.tMuted)
                      .padding({ left: 10, right: 10, top: 5, bottom: 5 })
                      .backgroundColor(this.ifaceFilter === 1 ? this.tSuccessSoft : this.tElevated)
                      .borderRadius(AppTheme.rXs)
                      .onClick(() => { this.ifaceFilter = 1; })
                    Text(`DOWN ${(d.snapshot?.interfaces ?? []).length - this.snmpUpCount(d)}`)
                      .fontSize(AppTheme.fontMicro)
                      .fontColor(this.ifaceFilter === 2 ? this.tDanger : this.tMuted)
                      .padding({ left: 10, right: 10, top: 5, bottom: 5 })
                      .backgroundColor(this.ifaceFilter === 2 ? this.tDangerSoft : this.tElevated)
                      .borderRadius(AppTheme.rXs)
                      .onClick(() => { this.ifaceFilter = 2; })
                    Text(`运行 ${this.snmpUptimeText(d)}`)
                      .fontSize(AppTheme.fontMicro).fontColor(this.tMuted)
                      .padding({ left: 6, right: 6, top: 5, bottom: 5 })
                  }
                }
                .scrollable(ScrollDirection.Horizontal)
                .scrollBar(BarState.Off)
                .width('100%')
                .margin({ top: 8 })

                if ((d.snapshot?.interfaces ?? []).length > 0) {
                  Divider().color(this.tBorderSoft).strokeWidth(0.5).margin({ top: 8, bottom: 4 })
                  if (this.snmpIfaces(d).length === 0) {
                    Text('无匹配接口').fontSize(AppTheme.fontCaption).fontColor(this.tMuted).padding({ top: 8, bottom: 4 })
                  } else {
                    ForEach(this.snmpIfaces(d), (iface: SNMPInterface, j: number) => {
                      Column({ space: 4 }) {
                        Row() {
                          Text(iface.name || iface.descr || `if${iface.index ?? j}`)
                            .fontSize(AppTheme.fontCaption).fontWeight(FontWeight.Medium).fontColor(this.tText)
                            .layoutWeight(1).maxLines(1)
                          Text(this.fmtIfSpeed(iface.speed_bps))
                            .fontSize(AppTheme.fontMicro).fontColor(this.tMuted)
                          Text('历史 ›')
                            .fontSize(AppTheme.fontTiny).fontColor(this.tAccent).margin({ left: 8 })
                            .onClick(() => { void this.loadIfaceHistory(d, iface); })
                        }
                        .width('100%')
                        Row({ space: 10 }) {
                          Text(`↓ ${FormatUtil.bytesRate(iface.in_bps)}`)
                            .fontSize(AppTheme.fontMicro).fontColor(this.tAccent)
                          Text(`↑ ${FormatUtil.bytesRate(iface.out_bps)}`)
                            .fontSize(AppTheme.fontMicro).fontColor(this.tSuccess)
                          if (iface.rate_valid) {
                            Text(`利用率 ${Math.round(this.ifaceUtil(iface))}%`)
                              .fontSize(AppTheme.fontMicro)
                              .fontColor(this.ifaceUtil(iface) >= 80 ? this.tWarning : this.tMuted)
                          }
                          if (this.ifaceErrRate(iface) > 0) {
                            Text(`错误/丢弃 ${this.ifaceErrRate(iface).toFixed(1)}/s`)
                              .fontSize(AppTheme.fontMicro).fontColor(this.tDanger)
                          }
                        }
                        .width('100%')
                        if (iface.alias) {
                          Text(iface.alias).fontSize(AppTheme.fontTiny).fontColor(this.tMuted).maxLines(1)
                            .textOverflow({ overflow: TextOverflow.Ellipsis }).width('100%')
                        }
                      }
                      .width('100%')
                      .alignItems(HorizontalAlign.Start)
                      .padding({ top: 8, bottom: 8 })
                      .border({ width: { bottom: 0.5 }, color: this.tBorderSoft })
                      .onClick(() => { void this.loadIfaceHistory(d, iface); })
                    }, (iface: SNMPInterface, j: number) => `if-${this.snmpKey(d)}-${iface.index ?? j}`)
                  }
                }
              }
              .width('100%')
              .alignItems(HorizontalAlign.Start)
              .padding(AppTheme.cardPad)
              .margin({ left: 12, right: 12, bottom: 8 })
              .backgroundColor(this.tCard)
              .borderRadius(AppTheme.rSm)
              .borderWidth({ left: 4, right: 1, top: 1, bottom: 1 })
              .borderColor({
                left: d.reachable === false ? this.tDanger : this.tSuccess,
                right: this.tBorderSoft,
                top: this.tBorderSoft,
                bottom: this.tBorderSoft
              })
            }, (d: SNMPDevice, i: number) => `snmp-${i}-${this.snmpKey(d)}`)
            Blank().height(12)
          }
          .width('100%')
        }
        .layoutWeight(1)
        .scrollBar(BarState.Off)
        .align(Alignment.Top)
      }
    }
    .width('100%')
    .layoutWeight(1)
  }

  @Builder
  netflowPane() {
    Column() {
      FilterChips({
        labels: this.nfDimLabels,
        selected: $netDimIdx,
        onSelect: (_i: number) => { void this.refresh(); }
      })
        .padding({ left: 12, right: 12, bottom: 4 })
      FilterChips({
        labels: this.nfRangeLabels,
        selected: $netRangeIdx,
        onSelect: (_i: number) => { void this.refresh(); }
      })
        .padding({ left: 12, right: 12, bottom: 8 })

      if (this.netflowItems.length === 0 && this.netflowFlows.length === 0) {
        EmptyHint({ text: '暂无流量数据' })
      } else {
        Scroll() {
          Column() {
            Text(`维度汇总 Top ${this.netflowItems.length}`)
              .fontSize(AppTheme.fontSmall).fontWeight(FontWeight.Bold).fontColor(this.tText)
              .padding({ left: 16, bottom: 6 }).width('100%')
            if (this.netflowItems.length === 0) {
              Text('暂无维度汇总').fontSize(AppTheme.fontCaption).fontColor(this.tMuted)
                .padding({ left: 16, bottom: 8 }).width('100%')
            }
            ForEach(this.netflowItems, (it: NetFlowSummaryItem, i: number) => {
              Column() {
                Row() {
                  Text(it.key || `条目 ${i + 1}`).fontSize(AppTheme.fontSmall).fontWeight(FontWeight.Medium)
                    .fontColor(this.tText).layoutWeight(1).maxLines(1)
                  if (this.nfDims[this.netDimIdx] === 'dst_ip' || this.nfDims[this.netDimIdx] === 'src_ip') {
                    Text('历史 ›').fontSize(AppTheme.fontTiny).fontColor(this.tAccent).margin({ right: 8 })
                  }
                  Text(FormatUtil.bytes(it.bytes ?? 0)).fontSize(AppTheme.fontCaption).fontWeight(FontWeight.Bold).fontColor(this.tAccent)
                }
                .width('100%')
                MetricBar({
                  label: '占比',
                  value: Math.min(100, Math.round((Number(it.bytes ?? 0) / this.nfMaxBytes()) * 100)),
                  color: this.tAccent,
                  compact: true
                })
                if ((it.packets ?? 0) > 0) {
                  Text(`${it.packets} 包 · ${it.flows ?? 0} flows`)
                    .fontSize(AppTheme.fontTiny).fontColor(this.tMuted).margin({ top: 4 }).width('100%')
                }
              }
              .width('100%')
              .padding(AppTheme.spaceMd)
              .margin({ left: 12, right: 12, bottom: 8 })
              .backgroundColor(this.tCard)
              .borderRadius(AppTheme.rSm)
              .onClick(() => {
                if (it.key && (this.nfDims[this.netDimIdx] === 'dst_ip' || this.nfDims[this.netDimIdx] === 'src_ip')) {
                  void this.loadIpHistory(it.key);
                }
              })
            }, (it: NetFlowSummaryItem, i: number) => `nf-${i}-${it.key ?? ''}`)

            Text(`Flow 明细（${this.netflowFlows.length}）`)
              .fontSize(AppTheme.fontSmall).fontWeight(FontWeight.Bold).fontColor(this.tText)
              .padding({ left: 16, top: 4, bottom: 6 }).width('100%')
            if (this.netflowFlows.length === 0) {
              Text('暂无 Flow 记录').fontSize(AppTheme.fontCaption).fontColor(this.tMuted)
                .padding({ left: 16, bottom: 8 }).width('100%')
            }
            ForEach(this.netflowFlows, (f: NetFlowItem, i: number) => {
              Column() {
                Row() {
                  Text(`${f.src_ip || '?'}:${f.src_port ?? 0}`)
                    .fontSize(AppTheme.fontCaption).fontColor(this.tText).layoutWeight(1).maxLines(1)
                    .textOverflow({ overflow: TextOverflow.Ellipsis })
                  Text(' → ')
                    .fontSize(AppTheme.fontCaption).fontColor(this.tMuted)
                  Text(`${f.dst_ip || '?'}:${f.dst_port ?? 0}`)
                    .fontSize(AppTheme.fontCaption).fontColor(this.tText).layoutWeight(1).maxLines(1)
                    .textOverflow({ overflow: TextOverflow.Ellipsis })
                }
                .width('100%')
                Row({ space: 10 }) {
                  Text(this.protoName(f.protocol ?? f.proto))
                    .fontSize(AppTheme.fontTiny).fontColor(this.tAccent)
                    .padding({ left: 8, right: 8, top: 2, bottom: 2 })
                    .backgroundColor(this.tAccentSoft).borderRadius(AppTheme.rChip)
                  Text(`↓ ${FormatUtil.bytes(f.bytes ?? 0)}`)
                    .fontSize(AppTheme.fontMicro).fontColor(this.tMuted)
                  Text(`✉ ${f.packets ?? 0}`)
                    .fontSize(AppTheme.fontMicro).fontColor(this.tMuted)
                  if (f.source) {
                    Text(f.source).fontSize(AppTheme.fontTiny).fontColor(this.tMuted)
                  }
                }
                .width('100%')
                .margin({ top: 4 })
                if (f.dst_enrich?.host || f.dst_enrich?.org) {
                  Text(`${f.dst_enrich?.host || ''} ${f.dst_enrich?.org || ''}`)
                    .fontSize(AppTheme.fontTiny).fontColor(this.tMuted).margin({ top: 2 })
                }
              }
              .width('100%')
              .alignItems(HorizontalAlign.Start)
              .padding(AppTheme.spaceMd)
              .margin({ left: 12, right: 12, bottom: 8 })
              .backgroundColor(this.tCard)
              .borderRadius(AppTheme.rSm)
              .onClick(() => {
                const ip = f.dst_ip || f.src_ip || '';
                if (ip) {
                  void this.loadIpHistory(ip);
                }
              })
            }, (f: NetFlowItem, i: number) => `flow-${i}-${f.src_ip ?? ''}-${f.dst_ip ?? ''}`)
            Blank().height(12)
          }
          .width('100%')
        }
        .layoutWeight(1)
        .scrollBar(BarState.Off)
        .align(Alignment.Top)
      }
    }
    .width('100%')
    .layoutWeight(1)
  }

  @Builder
  trapsPane() {
    if (this.traps.length === 0) {
      EmptyHint({ text: '暂无 Trap 事件\n本机端 :162 接收 SNMP Trap，设备侧上报后在此展示' })
    } else {
      Scroll() {
        Column() {
          ForEach(this.traps, (tr: SNMPTrap, i: number) => {
            Column({ space: 4 }) {
              Row() {
                Text(tr.source_ip || tr.device || '未知来源')
                  .fontSize(AppTheme.fontSmall).fontWeight(FontWeight.Bold).fontColor(this.tText)
                  .layoutWeight(1).maxLines(1)
                if (tr.version) {
                  Text(`v${tr.version}`).fontSize(AppTheme.fontTiny).fontColor(this.tMuted)
                }
                if (tr.severity) {
                  Text(tr.severity)
                    .fontSize(AppTheme.fontTiny)
                    .fontColor(FormatUtil.sevColor(tr.severity || ''))
                    .padding({ left: 8, right: 8, top: 2, bottom: 2 })
                    .backgroundColor(FormatUtil.sevSoft(tr.severity || ''))
                    .borderRadius(AppTheme.rChip)
                    .margin({ left: 6 })
                }
              }
              .width('100%')
              Text(tr.trap_oid || tr.oid || tr.message || 'Trap')
                .fontSize(AppTheme.fontMicro).fontColor(this.tMuted).maxLines(2).width('100%')
              if ((tr.varbinds ?? []).length > 0) {
                Text(this.trapVarbindText(tr))
                  .fontSize(AppTheme.fontTiny).fontColor(this.tMuted).maxLines(2).width('100%')
              }
              Text(this.relIso(tr.received_at) !== '—' ? this.relIso(tr.received_at) : FormatUtil.relTime(tr.time ?? 0))
                .fontSize(AppTheme.fontTiny).fontColor(this.tMuted)
            }
            .alignItems(HorizontalAlign.Start)
            .padding(AppTheme.spaceMd)
            .margin({ left: 12, right: 12, bottom: 8 })
            .backgroundColor(this.tCard)
            .borderRadius(AppTheme.rSm)
            .width('100%')
          }, (tr: SNMPTrap, i: number) => `trap-${i}-${tr.received_at ?? tr.time ?? 0}`)
          Blank().height(12)
        }
        .width('100%')
      }
      .layoutWeight(1)
      .scrollBar(BarState.Off)
      .align(Alignment.Top)
    }
  }

  @Builder
  auditPane() {
    Column() {
      SearchBar({
        text: $auditQuery,
        placeholder: '搜索 src_ip / dst_ip / method / path / status'
      })
        .padding({ left: 12, right: 12, bottom: 8 })

      if (this.audits.length === 0) {
        EmptyHint({ text: '暂无内容审计事件\n请确认本机端已开启 HTTP 内容审计采集' })
      } else if (this.filteredAudits().length === 0) {
        EmptyHint({ text: '没有匹配的审计事件' })
      } else {
        Scroll() {
          Column() {
            ForEach(this.filteredAudits(), (ev: ContentAuditEvent, i: number) => {
              Column({ space: 4 }) {
                Row() {
                  Text(`${ev.method || '—'} ${ev.status ?? ''}`)
                    .fontSize(AppTheme.fontCaption).fontWeight(FontWeight.Bold).fontColor(this.tText)
                  Blank()
                  Text(this.relIso(ev.observed_at) !== '—' ? this.relIso(ev.observed_at) : FormatUtil.relTime(ev.time ?? 0))
                    .fontSize(AppTheme.fontTiny).fontColor(this.tMuted)
                }
                .width('100%')
                Text(ev.path || '')
                  .fontSize(AppTheme.fontCaption).fontWeight(FontWeight.Medium).fontColor(this.tText).maxLines(1)
                  .textOverflow({ overflow: TextOverflow.Ellipsis }).width('100%')
                Text(`${ev.src_ip || '?'} → ${ev.dst_ip || '?'}:${ev.dst_port ?? 0}`)
                  .fontSize(AppTheme.fontMicro).fontColor(this.tMuted)
                if (ev.sensitive) {
                  Text(ev.sensitive)
                    .fontSize(AppTheme.fontTiny).fontColor(this.tDanger).maxLines(1)
                }
                if (ev.host) {
                  Text(`Host: ${ev.host}`).fontSize(AppTheme.fontTiny).fontColor(this.tMuted).maxLines(1)
                }
                if (ev.ctype || ev.resp_ctype) {
                  Text(ev.ctype || ev.resp_ctype || '')
                    .fontSize(AppTheme.fontTiny).fontColor(this.tMuted).maxLines(1)
                }
                if (ev.resp_body) {
                  Text(ev.resp_body)
                    .fontSize(AppTheme.fontTiny).fontColor(this.tMuted).maxLines(3)
                }
                if (ev.resp_truncated) {
                  Text('响应已截断').fontSize(AppTheme.fontTiny).fontColor(this.tWarning)
                }
              }
              .alignItems(HorizontalAlign.Start)
              .padding(AppTheme.spaceMd)
              .margin({ left: 12, right: 12, bottom: 8 })
              .backgroundColor(this.tCard)
              .borderRadius(AppTheme.rSm)
              .width('100%')
            }, (ev: ContentAuditEvent, i: number) => `aud-${i}-${ev.observed_at ?? ev.time ?? 0}`)
            Blank().height(12)
          }
          .width('100%')
        }
        .layoutWeight(1)
        .scrollBar(BarState.Off)
        .align(Alignment.Top)
      }
    }
    .width('100%')
    .layoutWeight(1)
  }

  @Builder
  probePane() {
    // Android ProbeContent 顺序：Overview → Tab → 类型筛选 → 新建/DNS阈值 → 列表
    Column() {
      Scroll() {
        Column({ space: 8 }) {
          FilterChips({
            labels: this.probeOverviewChipLabels(),
            selected: $probeOverviewIdx,
            onSelect: (i: number) => { this.onProbeOverviewSelect(i); }
          })
            .padding({ left: 2, right: 2 })

          if (this.nearestCertDays() >= 0) {
            Text(`HTTPS 证书最近到期 ${this.nearestCertDays()} 天`)
              .fontSize(AppTheme.fontCaption)
              .fontWeight(FontWeight.Medium)
              .fontColor(this.certColor(this.nearestCertDays()))
              .width('100%')
              .padding({ left: 2 })
          }

          FilterChips({
            labels: this.probeSegChipLabels(),
            selected: $probeSeg,
            variant: 'subtab'
          })

          if (this.probeSeg === 0 && this.probeTypes().length > 0) {
            FilterChips({
              labels: this.probeTypeLabels(),
              selected: $probeTypeIdx
            })
          }

          if (this.probeOverviewIdx !== 0) {
            Text(`快捷筛选：${this.probeOverviewLabels[this.probeOverviewIdx]} · 点击清除`)
              .fontSize(AppTheme.fontTiny)
              .fontColor(this.tAccent)
              .padding({ left: 10, right: 10, top: 5, bottom: 5 })
              .backgroundColor(this.tAccentSoft)
              .borderRadius(AppTheme.rSm)
              .onClick(() => { this.probeOverviewIdx = 0; })
          }

          if (this.probeSeg === 0) {
            Row({ space: 8 }) {
              Row({ space: 6 }) {
                Text('+').fontSize(AppTheme.fontLead).fontColor(this.tAccent)
                Text('新建拨测').fontSize(AppTheme.fontSmall).fontColor(this.tAccent)
              }
              .layoutWeight(1)
              .justifyContent(FlexAlign.Center)
              .height(40)
              .border({ width: 1, color: this.tAccent })
              .borderRadius(AppTheme.rSm)
              .onClick(() => this.openProbeCreate())

              if (this.isAdmin) {
                Text('DNS 阈值')
                  .fontSize(AppTheme.fontSmall)
                  .fontColor(this.tAccent)
                  .padding({ left: 8, right: 4 })
                  .onClick(() => { void this.openDnsThresholds(); })
              }
            }
            .width('100%')
            .padding({ top: 2, bottom: 4 })

            if (this.filteredProbes().length === 0) {
              Text(`没有${this.probeOverviewLabels[this.probeOverviewIdx] || ''}拨测`)
                .fontSize(AppTheme.fontBody).fontWeight(FontWeight.Medium).fontColor(this.tText)
                .width('100%').margin({ top: 24 })
                .textAlign(TextAlign.Center)
              Text('点击上方筛选标签可恢复全部监控对象')
                .fontSize(AppTheme.fontCaption).fontColor(this.tMuted)
                .width('100%').margin({ top: 6, bottom: 24 })
                .textAlign(TextAlign.Center)
            } else {
              ForEach(this.filteredProbes(), (p: Probe) => {
                this.probeCardItem(p)
              }, (p: Probe) => p.id)
            }
          } else {
            if (this.filteredApiSystems().length === 0) {
              Text(`没有${this.probeOverviewLabels[this.probeOverviewIdx] || ''} API`)
                .fontSize(AppTheme.fontBody).fontWeight(FontWeight.Medium).fontColor(this.tText)
                .width('100%').margin({ top: 24 })
                .textAlign(TextAlign.Center)
              Text('点击上方筛选标签可恢复全部业务系统')
                .fontSize(AppTheme.fontCaption).fontColor(this.tMuted)
                .width('100%').margin({ top: 6, bottom: 24 })
                .textAlign(TextAlign.Center)
            } else {
              ForEach(this.filteredApiSystems(), (sys: ApiMonitorSystem) => {
                this.apiSystemCardItem(sys)
              }, (sys: ApiMonitorSystem) => sys.id)
            }
          }

          Blank().height(16)
        }
        .width('100%')
        .padding({ left: 14, right: 14, top: 4 })
        .alignItems(HorizontalAlign.Start)
      }
      .layoutWeight(1)
      .scrollBar(BarState.Off)
      .align(Alignment.Top)
    }
    .width('100%')
    .layoutWeight(1)
  }

  @Builder
  probeCardItem(p: Probe) {
    Row() {
      Column()
        .width(4)
        .alignSelf(ItemAlign.Stretch)
        .backgroundColor(this.probeStateColor(p))
      Column({ space: 6 }) {
        Row({ space: 6 }) {
          StatusDot({
            online: this.probeOk(p) && p.enabled !== false && (p.checked_at ?? 0) > 0,
            dotSize: 9
          })
          Column() {
            Text(p.name || p.id)
              .fontSize(AppTheme.fontBody)
              .fontWeight(FontWeight.Bold)
              .fontColor(this.tText)
              .maxLines(1)
              .textOverflow({ overflow: TextOverflow.Ellipsis })
            if (p.builtin) {
              Text('内置自检').fontSize(AppTheme.fontTiny).fontColor(this.tMuted).margin({ top: 2 })
            }
          }
          .layoutWeight(1)
          .alignItems(HorizontalAlign.Start)
          if (p.type) {
            Text((p.type || '').toUpperCase())
              .fontSize(AppTheme.fontTiny)
              .fontColor(this.tAccent)
              .padding({ left: 8, right: 8, top: 3, bottom: 3 })
              .backgroundColor(this.tAccentSoft)
              .borderRadius(AppTheme.rChip)
          }
          Text('历史').fontSize(AppTheme.fontMicro).fontColor(this.tAccent)
            .padding({ left: 6, right: 4 })
            .onClick(() => { void this.loadProbeHistory(p); })
          Text(this.isProbeTesting(p.id) ? '探测中' : '立即测')
            .fontSize(AppTheme.fontMicro)
            .fontColor(p.enabled === false || this.isProbeTesting(p.id) ? this.tMuted : this.tAccent)
            .padding({ left: 4, right: 4 })
            .onClick(() => { void this.runProbe(p); })
          if (!p.builtin) {
            Text('编辑').fontSize(AppTheme.fontMicro).fontColor(this.tMuted)
              .padding({ left: 4, right: 4 })
              .onClick(() => this.openProbeEdit(p))
            Text('删除').fontSize(AppTheme.fontMicro).fontColor(this.tDanger)
              .padding({ left: 4, right: 2 })
              .onClick(() => {
                this.probeDeleteId = p.id;
                this.probeDeleteVisible = true;
              })
          }
        }
        .width('100%')

        Text(this.probeDisplayTarget(p))
          .fontSize(AppTheme.fontCaption)
          .fontColor(this.tMuted)
          .maxLines(1)
          .textOverflow({ overflow: TextOverflow.Ellipsis })
          .width('100%')

        Flex({ wrap: FlexWrap.Wrap }) {
          Column() {
            Text('本次延迟').fontSize(AppTheme.fontTiny).fontColor(this.tMuted)
            Text((p.checked_at ?? 0) > 0 ? `${this.probeLatency(p).toFixed(1)} ms` : '—')
              .fontSize(AppTheme.fontMicro).fontWeight(FontWeight.Bold)
              .fontColor(FormatUtil.latencyColor(this.probeLatency(p)))
          }
          .margin({ right: 16, top: 2 })
          if ((p.type || '').toLowerCase().indexOf('http') >= 0 && (p.checked_at ?? 0) > 0) {
            Column() {
              Text('状态码').fontSize(AppTheme.fontTiny).fontColor(this.tMuted)
              Text(`${(p.status_code && p.status_code > 0) ? p.status_code : 200}`)
                .fontSize(AppTheme.fontMicro).fontWeight(FontWeight.Bold)
                .fontColor(((p.status_code ?? 200) < 400) ? this.tSuccess : this.tDanger)
            }
            .margin({ right: 16, top: 2 })
          }
          if ((p.type || '').toLowerCase() === 'ping' && (p.loss_pct ?? -1) >= 0 && (p.checked_at ?? 0) > 0) {
            Column() {
              Text('丢包率').fontSize(AppTheme.fontTiny).fontColor(this.tMuted)
              Text(`${Number(p.loss_pct).toFixed(1)}%`)
                .fontSize(AppTheme.fontMicro).fontWeight(FontWeight.Bold)
                .fontColor((p.loss_pct ?? 0) === 0 ? this.tSuccess :
                  ((p.loss_pct ?? 0) < 10 ? this.tWarning : this.tDanger))
            }
            .margin({ right: 16, top: 2 })
          }
          if ((p.type || '').toLowerCase() === 'tcp' && (p.checked_at ?? 0) > 0) {
            Column() {
              Text('连接状态').fontSize(AppTheme.fontTiny).fontColor(this.tMuted)
              Text(this.probeOk(p) ? '已连接' : '未连接')
                .fontSize(AppTheme.fontMicro).fontWeight(FontWeight.Bold)
                .fontColor(this.probeOk(p) ? this.tSuccess : this.tDanger)
            }
            .margin({ right: 16, top: 2 })
          }
          if ((p.type || '').toLowerCase() === 'process' && (p.checked_at ?? 0) > 0) {
            Column() {
              Text('进程名').fontSize(AppTheme.fontTiny).fontColor(this.tMuted)
              Text(this.probeProcessName(p))
                .fontSize(AppTheme.fontMicro).fontWeight(FontWeight.Bold).fontColor(this.tAccent)
            }
            .margin({ right: 16, top: 2 })
          }
          if ((p.type || '').toLowerCase() === 'dns') {
            Column() {
              Text('记录').fontSize(AppTheme.fontTiny).fontColor(this.tMuted)
              Text((p.dns_type || 'A').toUpperCase())
                .fontSize(AppTheme.fontMicro).fontWeight(FontWeight.Bold).fontColor(this.tAccent)
            }
            .margin({ right: 16, top: 2 })
          }
          Column() {
            Text('上次探测').fontSize(AppTheme.fontTiny).fontColor(this.tMuted)
            Text(FormatUtil.relTime(p.checked_at ?? 0) || '—')
              .fontSize(AppTheme.fontMicro).fontWeight(FontWeight.Bold).fontColor(this.tText)
          }
          .margin({ right: 16, top: 2 })
          Column() {
            Text('探测结果').fontSize(AppTheme.fontTiny).fontColor(this.tMuted)
            Text(this.probeStatusText(p))
              .fontSize(AppTheme.fontMicro).fontWeight(FontWeight.Bold)
              .fontColor(this.probeStateColor(p))
          }
          .margin({ top: 2 })
        }
        .width('100%')

        if (this.isHttpsTarget(p.target)) {
          Row() {
            Text('HTTPS')
              .fontSize(AppTheme.fontTiny).fontWeight(FontWeight.Bold)
              .fontColor(this.certColor(p.cert_days))
            Blank()
            Text(this.certLabel(p.cert_days, p.checked_at))
              .fontSize(AppTheme.fontMicro).fontWeight(FontWeight.Medium)
              .fontColor(this.certColor(p.cert_days))
          }
          .width('100%')
          .padding({ left: 9, right: 9, top: 6, bottom: 6 })
          .backgroundColor(this.tElevated)
          .borderRadius(AppTheme.rXs)
        }
      }
      .layoutWeight(1)
      .padding(AppTheme.spaceMd)
      .alignItems(HorizontalAlign.Start)
    }
    .width('100%')
    .alignItems(VerticalAlign.Top)
    .margin({ bottom: 10 })
    .backgroundColor(this.tCard)
    .borderRadius(AppTheme.rMd)
    .clip(true)
  }

  @Builder
  apiSystemCardItem(sys: ApiMonitorSystem) {
    Row() {
      Column()
        .width(4)
        .alignSelf(ItemAlign.Stretch)
        .backgroundColor(sys.enabled === false ? this.tMuted :
          ((sys.endpoints ?? []).filter((e: ApiEndpoint) => (e.checked_at ?? 0) > 0 && e.ok === false).length > 0
            ? this.tDanger : this.tSuccess))
      Column({ space: 6 }) {
        Row() {
          Column() {
            Text(sys.name || sys.id)
              .fontSize(AppTheme.fontLead).fontWeight(FontWeight.Bold).fontColor(this.tText)
            Text(`${(sys.endpoints ?? []).length} 个接口 · 每 ${sys.interval_sec ?? '—'}s`)
              .fontSize(AppTheme.fontTiny).fontColor(this.tMuted).margin({ top: 2 })
          }
          .layoutWeight(1)
          .alignItems(HorizontalAlign.Start)
          Text(sys.enabled === false ? '已停用' :
            (((sys.endpoints ?? []).filter((e: ApiEndpoint) => (e.checked_at ?? 0) > 0 && e.ok === false).length > 0)
              ? `${(sys.endpoints ?? []).filter((e: ApiEndpoint) => (e.checked_at ?? 0) > 0 && e.ok === false).length} 异常`
              : '运行中'))
            .fontSize(AppTheme.fontTiny)
            .fontColor(sys.enabled === false ? this.tMuted :
              (((sys.endpoints ?? []).filter((e: ApiEndpoint) => (e.checked_at ?? 0) > 0 && e.ok === false).length > 0)
                ? this.tDanger : this.tSuccess))
            .padding({ left: 8, right: 8, top: 3, bottom: 3 })
            .backgroundColor(this.tElevated)
            .borderRadius(AppTheme.rChip)
            .margin({ right: 8 })
          Text(this.isApiTesting(sys.id) ? '探测中' : '立即测')
            .fontSize(AppTheme.fontMicro)
            .fontColor(sys.enabled === false || this.isApiTesting(sys.id) ? this.tMuted : this.tAccent)
            .onClick(() => { void this.testApi(sys); })
          Text('删除').fontSize(AppTheme.fontMicro).fontColor(this.tDanger).margin({ left: 10 })
            .onClick(() => {
              this.apiDeleteId = sys.id;
              this.apiDeleteVisible = true;
            })
        }
        .width('100%')

        if ((sys.endpoints ?? []).length === 0) {
          Text('该业务系统暂无接口').fontSize(AppTheme.fontCaption).fontColor(this.tMuted)
        } else {
          ForEach(sys.endpoints ?? [], (ep: ApiEndpoint, j: number) => {
            Column({ space: 4 }) {
              if (j > 0) {
                Divider().color(this.tBorderSoft).strokeWidth(0.5)
              }
              Row() {
                StatusDot({
                  online: ep.enabled !== false && (ep.checked_at ?? 0) > 0 && ep.ok !== false,
                  dotSize: 7
                })
                Column() {
                  Text(ep.name || ep.url || ep.id)
                    .fontSize(AppTheme.fontSmall).fontWeight(FontWeight.Bold).fontColor(this.tText)
                    .maxLines(1).textOverflow({ overflow: TextOverflow.Ellipsis })
                  Text(`${(ep.method || 'GET').toUpperCase()} ${ep.url || ''}`)
                    .fontSize(AppTheme.fontTiny).fontColor(this.tMuted).maxLines(1)
                    .textOverflow({ overflow: TextOverflow.Ellipsis })
                }
                .layoutWeight(1)
                .alignItems(HorizontalAlign.Start)
                .margin({ left: 7 })
                Text('历史').fontSize(AppTheme.fontMicro).fontColor(this.tAccent)
                  .onClick(() => { void this.loadApiHistory(ep); })
              }
              .width('100%')
              Row({ space: 6 }) {
                Column() {
                  Text((ep.checked_at ?? 0) > 0 ? `${Math.round(ep.latency_ms ?? 0)} ms` : '—')
                    .fontSize(AppTheme.fontMicro).fontWeight(FontWeight.Bold)
                    .fontColor(FormatUtil.latencyColor(ep.latency_ms ?? 0))
                  Text('本次').fontSize(AppTheme.fontTiny).fontColor(this.tMuted)
                }
                .layoutWeight(1)
                .alignItems(HorizontalAlign.Center)
                .padding({ top: 6, bottom: 6 })
                .backgroundColor(this.tElevated)
                .borderRadius(AppTheme.rSm)
                Column() {
                  Text(`${Math.round(ep.avg_ms ?? 0)} ms`)
                    .fontSize(AppTheme.fontMicro).fontWeight(FontWeight.Bold).fontColor(this.tAccent)
                  Text('平均 1h').fontSize(AppTheme.fontTiny).fontColor(this.tMuted)
                }
                .layoutWeight(1)
                .alignItems(HorizontalAlign.Center)
                .padding({ top: 6, bottom: 6 })
                .backgroundColor(this.tElevated)
                .borderRadius(AppTheme.rSm)
                Column() {
                  Text(`${Math.round(ep.p95_ms ?? 0)} ms`)
                    .fontSize(AppTheme.fontMicro).fontWeight(FontWeight.Bold).fontColor(this.tWarning)
                  Text('P95 1h').fontSize(AppTheme.fontTiny).fontColor(this.tMuted)
                }
                .layoutWeight(1)
                .alignItems(HorizontalAlign.Center)
                .padding({ top: 6, bottom: 6 })
                .backgroundColor(this.tElevated)
                .borderRadius(AppTheme.rSm)
                Column() {
                  Text(`${this.availPct(ep)}%`)
                    .fontSize(AppTheme.fontMicro).fontWeight(FontWeight.Bold)
                    .fontColor(this.availPct(ep) >= 99 ? this.tSuccess : this.tDanger)
                  Text('可用率 1h').fontSize(AppTheme.fontTiny).fontColor(this.tMuted)
                }
                .layoutWeight(1)
                .alignItems(HorizontalAlign.Center)
                .padding({ top: 6, bottom: 6 })
                .backgroundColor(this.tElevated)
                .borderRadius(AppTheme.rSm)
              }
              .width('100%')
              if (this.isHttpsTarget(ep.url)) {
                Row() {
                  Text('HTTPS').fontSize(AppTheme.fontTiny).fontWeight(FontWeight.Bold)
                    .fontColor(this.certColor(ep.cert_days))
                  Blank()
                  Text(this.certLabel(ep.cert_days, ep.checked_at))
                    .fontSize(AppTheme.fontMicro).fontColor(this.certColor(ep.cert_days))
                }
                .width('100%')
                .padding({ left: 9, right: 9, top: 6, bottom: 6 })
                .backgroundColor(this.tElevated)
                .borderRadius(AppTheme.rXs)
              }
              if (ep.message) {
                Text(ep.message)
                  .fontSize(AppTheme.fontTiny)
                  .fontColor(ep.ok === false ? this.tDanger : this.tMuted)
                  .maxLines(2)
                  .width('100%')
              }
            }
            .width('100%')
            .alignItems(HorizontalAlign.Start)
            .padding({ top: 4, bottom: 4 })
          }, (ep: ApiEndpoint, j: number) => `ep-${sys.id}-${ep.id}`)
        }
      }
      .layoutWeight(1)
      .padding(AppTheme.spaceMd)
      .alignItems(HorizontalAlign.Start)
    }
    .width('100%')
    .alignItems(VerticalAlign.Top)
    .margin({ bottom: 10 })
    .backgroundColor(this.tCard)
    .borderRadius(AppTheme.rMd)
    .clip(true)
  }
}

@Entry
@Component
struct InfraHubPage {
  @StorageLink('aiops_theme_rev') themeRev: number = 0;

  @StorageLink('aiops_bg') tBg: string = '#080C14';
  @StorageLink('aiops_card') tCard: string = '#111722';
  @StorageLink('aiops_elevated') tElevated: string = '#192130';
  @StorageLink('aiops_text') tText: string = '#E6E8F0';
  @StorageLink('aiops_muted') tMuted: string = '#9AA3B8';
  @StorageLink('aiops_accent') tAccent: string = '#4F7FFF';
  @StorageLink('aiops_accent_soft') tAccentSoft: string = '#1E3166';
  @StorageLink('aiops_accent_on') tAccentOn: string = '#D6E2FF';
  @StorageLink('aiops_success') tSuccess: string = '#00A86B';
  @StorageLink('aiops_success_soft') tSuccessSoft: string = '#0F3A2C';
  @StorageLink('aiops_warning') tWarning: string = '#F59E0B';
  @StorageLink('aiops_warning_soft') tWarningSoft: string = '#3A2A10';
  @StorageLink('aiops_danger') tDanger: string = '#EF4444';
  @StorageLink('aiops_danger_soft') tDangerSoft: string = '#3A1614';
  @StorageLink('aiops_border') tBorder: string = '#39445A';
  @StorageLink('aiops_border_soft') tBorderSoft: string = '#222C3D';
  @StorageLink('aiops_on_accent') tOnAccent: string = '#FFFFFF';
  @StorageLink('aiops_nav_bar') tNavBar: string = '#111722';
  @StorageLink('aiops_scrim') tScrim: string = '#99000000';
  @StorageLink('aiops_shadow') tShadow: string = '#40000000';

  build() {
    Column() {
      InfraHubTab()
    }
    .width('100%')
    .height('100%')
  }
}
