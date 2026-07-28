import { pasteboard } from '@kit.BasicServicesKit';
import { Api } from '../data/Api';
import { ApiClient, StreamUiThrottle } from '../data/ApiClient';
import { AppTheme } from '../common/Routes';
import { FormatUtil } from '../common/FormatUtil';
import { NavUtil } from '../common/NavUtil';
import {
  ActivityLogEntry,
  CreateIncidentRequest,
  DiagnosisChatMessage,
  DirectoryUser,
  Host,
  HTTPProxyConfig,
  Incident,
  IncidentEvent,
  InspectionReport,
  LogEntry,
  Playbook,
  PlaybookExecution,
  PortForwardCreateRequest,
  PortForwardEditRequest,
  PortForwardRule,
  RemediationRun,
  SloStatus,
  SreOverview,
  Ticket
} from '../data/models/Models';
import {
  ConfirmDialogHost,
  EmptyHint,
  ErrorBanner,
  FilterChips,
  PageHeader,
  SearchBar,
  SeverityBadge
} from '../components/CommonUI';
import {
  DetailSheet,
  OpsKpiStrip,
  ToastController,
  ToastHost
} from '../components/InteractionUI';
import { MarkdownText } from '../components/MarkdownUI';

@Component
export struct OperationsTab {
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
  onExecution: (execId: number) => void = (_execId: number) => {};

  @State tab: number = 0;
  @State govSub: number = 0;
  @State incidentFilter: number = 0; // 0 处理中 1 已解决 2 全部
  @State logLevelIdx: number = 0;
  @State logRangeIdx: number = 2; // 24h
  @State logQuery: string = '';
  /** 0=Agent 日志, 1=终端审计 — Android LogsPanel logSourceTab */
  @State logSourceTab: number = 0;
  @State terminalAuditLogs: ActivityLogEntry[] = [];
  @State terminalAuditQuery: string = '';
  @State directoryUsers: DirectoryUser[] = [];
  @State assignTicketId: number = 0;
  @State assignVisible: boolean = false;
  @State createVisible: boolean = false;
  @State createBusy: boolean = false;
  @State createTitle: string = '';
  @State createSev: number = 1; // 0 critical 1 warning 2 info
  @State playbookConfirm: boolean = false;
  @State playbookTarget: string = '';
  @State playbookName: string = '';
  @State diagVisible: boolean = false;
  @State diagTitle: string = '';
  @State diagText: string = '';
  @State diagBusy: boolean = false;
  @State diagActivity: string = '';
  @State diagError: string = '';
  @State diagIncidentId: number = 0;
  @State diagInput: string = '';
  @State diagMessages: DiagnosisChatMessage[] = [];
  @State diagIncludeTerminal: boolean = false;
  private diagThrottle: StreamUiThrottle | null = null;
  @State logDiagVisible: boolean = false;
  @State logDiagText: string = '';
  @State logDiagBusy: boolean = false;
  @State detailVisible: boolean = false;
  @State detailInc: Incident | null = null;
  @State ticketComment: string = '';
  @State toastMsg: string = '';
  @State toastError: boolean = false;
  @State syncing: boolean = false;

  @State overview: SreOverview = {};
  @State incidents: Incident[] = [];
  @State logs: LogEntry[] = [];
  @State playbooks: Playbook[] = [];
  @State executions: PlaybookExecution[] = [];
  @State forwards: PortForwardRule[] = [];
  @State proxies: HTTPProxyConfig[] = [];
  @State remediations: RemediationRun[] = [];
  @State slos: SloStatus[] = [];
  @State tickets: Ticket[] = [];
  @State error: string = '';
  @State busyKey: string = '';
  @State hostOptions: Host[] = [];
  @State showForwardForm: boolean = false;
  @State editingForwardId: string = '';
  @State fwdHostId: string = '';
  @State fwdTargetPort: string = '';
  @State fwdLocalPort: string = '';
  @State fwdProtocol: string = 'tcp';
  @State showProxyForm: boolean = false;
  @State editingProxyId: string = '';
  @State pxHostId: string = '';
  @State pxName: string = '';
  @State pxTargetPort: string = '';
  @State pxDefaultPath: string = '';
  @State deleteForwardId: string = '';
  @State deleteProxyId: string = '';
  @State showDeleteForward: boolean = false;
  @State showDeleteProxy: boolean = false;
  @State forwardBusy: boolean = false;
  private pollId: number = -1;

  private tabLabels: string[] = ['事件', '日志', '剧本', '转发', '治理'];
  private govLabels: string[] = ['审批', 'SLO', '工单'];
  private incFilterLabels: string[] = ['处理中', '已解决', '全部'];
  private logSourceLabels: string[] = ['Agent 日志', '终端审计'];
  private logLevelLabels: string[] = ['全部', 'error', 'warn', 'info'];
  private logRangeLabels: string[] = ['1小时', '6小时', '24小时'];
  private sevLabels: string[] = ['严重', '警告', '信息'];

  onInitialTab(): void {
    const t = Number(this.initialTab);
    if (t >= 0 && t <= 4) {
      this.tab = t;
    }
    void this.refresh();
  }

  aboutToAppear(): void {
    const t = Number(this.initialTab);
    if (t >= 0 && t <= 4) {
      this.tab = t;
    }
    void this.refresh();
    this.pollId = setInterval(() => {
      if (ApiClient.isSessionAlive()) {
        void this.refresh(true);
      }
    }, 15000);
  }

  aboutToDisappear(): void {
    if (this.pollId >= 0) {
      clearInterval(this.pollId);
    }
  }

  private toast(msg: string, isError: boolean = false): void {
    this.toastMsg = msg;
    this.toastError = isError;
    ToastController.delayClear((v: string) => {
      this.toastMsg = v;
    }, msg);
  }

  private openIncidents(): number {
    return this.overview.open_incidents ?? this.filteredIncidents().filter((x) => {
      const s = (x.status || '').toLowerCase();
      return s !== 'resolved' && s !== 'closed';
    }).length;
  }

  private pendingRemediations(): number {
    return this.overview.pending_remediations ?? this.overview.pending_approvals ??
      this.remediations.filter((r) => (r.status || '').toLowerCase().indexOf('pending') >= 0).length;
  }

  private openTickets(): number {
    return this.overview.open_tickets ??
      this.tickets.filter((t) => {
        const s = (t.status || '').toLowerCase();
        return s !== 'closed' && s !== 'resolved' && s !== 'done';
      }).length;
  }

  private sloBurning(): number {
    if (this.overview.slo_breaching !== undefined && this.overview.slo_breaching !== null) {
      return this.overview.slo_breaching;
    }
    if (this.overview.failing_slos !== undefined && this.overview.failing_slos !== null) {
      return this.overview.failing_slos;
    }
    return this.slos.filter((s) => {
      const st = (s.status || '').toLowerCase();
      return st.indexOf('breach') >= 0 || st.indexOf('fail') >= 0 || st.indexOf('burn') >= 0;
    }).length;
  }

  private hasAssistantReply(): boolean {
    for (let i = this.diagMessages.length - 1; i >= 0; i--) {
      const m = this.diagMessages[i];
      if ((m.role || '') === 'assistant' && (m.content || '').trim().length > 0) {
        return true;
      }
    }
    return (this.diagText || '').trim().length > 0;
  }

  private incResolved(inc: Incident): boolean {
    const s = (inc.status || '').toLowerCase();
    return s === 'resolved' || s === 'closed';
  }

  private incOpen(inc: Incident): boolean {
    const s = (inc.status || '').toLowerCase();
    return !s || s === 'open' || s === 'active' || s === 'firing';
  }

  private filteredIncidents(): Incident[] {
    if (this.incidentFilter === 2) {
      return this.incidents;
    }
    return this.incidents.filter((inc: Incident) => {
      const s = (inc.status || '').toLowerCase();
      const resolved = s === 'resolved' || s === 'closed';
      return this.incidentFilter === 1 ? resolved : !resolved;
    });
  }

  private logSinceMin(): number {
    if (this.logRangeIdx === 0) {
      return 60;
    }
    if (this.logRangeIdx === 1) {
      return 360;
    }
    return 1440;
  }

  private logLevel(): string {
    if (this.logLevelIdx <= 0) {
      return '';
    }
    return this.logLevelLabels[this.logLevelIdx];
  }

  private timelineOf(inc: Incident): IncidentEvent[] {
    const tl = inc.timeline ?? [];
    return tl.slice().sort((a: IncidentEvent, b: IncidentEvent) => (b.ts ?? 0) - (a.ts ?? 0));
  }

  async refresh(silent: boolean = false): Promise<void> {
    if (!silent) {
      this.error = '';
    }
    this.syncing = true;
    try {
      try {
        this.overview = await Api.sreOverview();
      } catch (_e) {
        // overview optional
      }
      if (this.tab === 0) {
        this.incidents = await Api.incidents();
      } else if (this.tab === 1) {
        if (this.logSourceTab === 1) {
          const all = await Api.activity();
          this.terminalAuditLogs = all.filter((e: ActivityLogEntry) =>
            (e.kind || '').toLowerCase() === 'terminal');
        } else {
          const res = await Api.logs(this.logQuery.trim(), this.logSinceMin(), '', this.logLevel(), 1);
          this.logs = res.items ?? [];
        }
      } else if (this.tab === 2) {
        this.playbooks = await Api.playbooks();
        this.executions = await Api.playbookExecutions();
      } else if (this.tab === 3) {
        this.forwards = await Api.portForwards();
        this.proxies = await Api.httpProxies();
      } else {
        this.remediations = await Api.remediationRuns();
        this.slos = await Api.slos();
        this.tickets = await Api.tickets();
        try {
          this.directoryUsers = await Api.directoryUsers();
        } catch (_e) {
          this.directoryUsers = [];
        }
      }
    } catch (e) {
      this.error = ApiClient.formatError(e as Object);
    } finally {
      this.syncing = false;
    }
  }

  private jumpKpi(i: number): void {
    if (i === 0) {
      this.tab = 0;
    } else {
      this.tab = 4;
      this.govSub = i === 1 ? 0 : (i === 2 ? 2 : 1);
    }
    void this.refresh();
  }

  async doCreateIncident(): Promise<void> {
    if (!this.createTitle.trim() || this.createBusy) {
      if (!this.createTitle.trim()) {
        this.toast('请填写事件标题', true);
      }
      return;
    }
    const sev = this.createSev === 0 ? 'critical' : (this.createSev === 1 ? 'warning' : 'info');
    const body: CreateIncidentRequest = {
      title: this.createTitle.trim(),
      severity: sev
    };
    this.createBusy = true;
    try {
      await Api.createIncident(body);
      this.createVisible = false;
      this.createTitle = '';
      this.toast('已创建事件');
      await this.refresh();
    } catch (e) {
      this.toast(ApiClient.formatError(e as Object), true);
    } finally {
      this.createBusy = false;
    }
  }

  async doAck(inc: Incident): Promise<void> {
    this.busyKey = `ack-${inc.id}`;
    try {
      await Api.ackIncident(inc.id);
      this.toast('已确认');
      await this.refresh();
    } catch (e) {
      this.toast(ApiClient.formatError(e as Object), true);
    } finally {
      this.busyKey = '';
    }
  }

  async doResolve(inc: Incident): Promise<void> {
    this.busyKey = `res-${inc.id}`;
    try {
      await Api.resolveIncident(inc.id);
      this.toast('已解决');
      await this.refresh();
    } catch (e) {
      this.toast(ApiClient.formatError(e as Object), true);
    } finally {
      this.busyKey = '';
    }
  }

  async doEscalate(inc: Incident): Promise<void> {
    this.busyKey = `esc-${inc.id}`;
    try {
      await Api.escalateIncident(inc.id);
      this.toast('已转工单');
      await this.refresh();
    } catch (e) {
      this.toast(ApiClient.formatError(e as Object), true);
    } finally {
      this.busyKey = '';
    }
  }

  private patchLastAssistant(content: string): void {
    const cur = this.diagMessages.slice();
    if (cur.length > 0 && (cur[cur.length - 1].role || '') === 'assistant') {
      cur[cur.length - 1] = { role: 'assistant', content: content };
    } else {
      cur.push({ role: 'assistant', content: content });
    }
    this.diagMessages = cur;
    this.diagText = content;
  }

  private stopDiag(): void {
    ApiClient.cancelActiveSse();
    if (this.diagThrottle) {
      this.diagThrottle.flush();
      this.diagThrottle = null;
    }
    this.diagBusy = false;
    this.diagActivity = '已停止';
  }

  async runDiag(inc: Incident): Promise<void> {
    this.diagVisible = true;
    this.diagIncidentId = inc.id;
    this.diagTitle = inc.title || `事件 #${inc.id}`;
    this.diagText = '';
    this.diagInput = '';
    this.diagMessages = [];
    this.diagError = '';
    this.diagBusy = true;
    this.diagActivity = '正在连接诊断服务…';
    try {
      try {
        const hist = await Api.incidentDiagnosisHistory(inc.id);
        this.diagMessages = hist.messages ?? [];
      } catch (_e) {
        // history optional
      }
      if (this.diagMessages.length === 0) {
        this.diagMessages = [
          { role: 'user', content: '请诊断该事件并给出处置建议' },
          { role: 'assistant', content: '' }
        ];
        this.diagActivity = 'AI 正在生成诊断报告…';
        const throttle = new StreamUiThrottle((t: string) => {
          this.patchLastAssistant(t);
          this.diagActivity = 'AI 正在生成诊断报告…';
        });
        this.diagThrottle = throttle;
        const text = await Api.diagnoseIncident(inc.id, (chunk: string) => {
          throttle.push(chunk);
        });
        throttle.flush();
        this.patchLastAssistant(text || this.diagText);
        this.diagActivity = '诊断完成，可继续追问';
      } else {
        this.diagActivity = '已加载历史诊断，可继续追问';
      }
    } catch (e) {
      this.diagError = ApiClient.formatError(e as Object);
      this.diagActivity = '诊断服务异常';
      this.toast(this.diagError, true);
      if (this.diagMessages.length === 0) {
        this.diagText = this.diagError;
      } else {
        this.patchLastAssistant(this.diagError);
      }
    } finally {
      this.diagThrottle = null;
      this.diagBusy = false;
    }
  }

  async sendDiagFollowUp(): Promise<void> {
    const msg = this.diagInput.trim();
    if (!msg || this.diagBusy || this.diagIncidentId <= 0) {
      return;
    }
    this.diagBusy = true;
    this.diagError = '';
    this.diagActivity = '正在生成回复…';
    const next: DiagnosisChatMessage[] = this.diagMessages.slice();
    next.push({ role: 'user', content: msg });
    next.push({ role: 'assistant', content: '' });
    this.diagMessages = next;
    this.diagInput = '';
    const throttle = new StreamUiThrottle((t: string) => {
      this.patchLastAssistant(t);
    });
    this.diagThrottle = throttle;
    try {
      const reply = await Api.incidentDiagnosisChat(
        this.diagIncidentId,
        msg,
        (chunk: string) => {
          throttle.push(chunk);
        },
        this.diagIncludeTerminal
      );
      throttle.flush();
      this.patchLastAssistant(reply || this.diagText);
      this.diagActivity = '可继续追问，回答会保留事件上下文';
    } catch (e) {
      this.diagError = ApiClient.formatError(e as Object);
      this.diagActivity = '诊断服务异常';
      this.toast(this.diagError, true);
      this.patchLastAssistant(this.diagError);
    } finally {
      this.diagThrottle = null;
      this.diagBusy = false;
    }
  }

  async sendDiagFeedback(helpful: boolean): Promise<void> {
    if (this.diagIncidentId <= 0) {
      return;
    }
    try {
      await Api.incidentDiagnosisFeedback(this.diagIncidentId, helpful, '');
      this.toast(helpful ? '已标记有帮助' : '已记录反馈');
    } catch (e) {
      this.toast(ApiClient.formatError(e as Object), true);
    }
  }

  async diagnoseLogRow(row: LogEntry): Promise<void> {
    this.logDiagVisible = true;
    this.logDiagText = '';
    this.logDiagBusy = true;
    try {
      const r: InspectionReport = await Api.diagnoseLog(row.host || '', row.message || this.logQuery);
      const findings = (r.findings ?? []).join('\n· ');
      this.logDiagText = `${r.summary || '诊断完成'}${findings ? '\n· ' + findings : ''}`;
    } catch (e) {
      this.logDiagText = ApiClient.formatError(e as Object);
    } finally {
      this.logDiagBusy = false;
    }
  }

  async doExecutePlaybook(): Promise<void> {
    this.playbookConfirm = false;
    if (!this.playbookTarget) {
      return;
    }
    try {
      await Api.executePlaybook(this.playbookTarget);
      this.toast(`已触发：${this.playbookName || this.playbookTarget}`);
      await this.refresh();
    } catch (e) {
      this.toast(ApiClient.formatError(e as Object), true);
    }
  }

  async toggleForward(f: PortForwardRule): Promise<void> {
    try {
      await Api.togglePortForward(f.id, !(f.enabled ?? false));
      await this.refresh();
    } catch (e) {
      this.toast(ApiClient.formatError(e as Object), true);
    }
  }

  async toggleProxy(p: HTTPProxyConfig): Promise<void> {
    try {
      await Api.toggleHTTPProxy(p.id, !(p.enabled ?? false));
      await this.refresh();
    } catch (e) {
      this.toast(ApiClient.formatError(e as Object), true);
    }
  }

  private forwardTargetPort(f: PortForwardRule): number {
    return f.target_port ?? f.remote_port ?? 0;
  }

  private proxyTargetPort(p: HTTPProxyConfig): number {
    return p.target_port ?? p.port ?? 0;
  }

  private async ensureHosts(): Promise<void> {
    if (this.hostOptions.length > 0) {
      return;
    }
    try {
      this.hostOptions = await Api.hosts();
    } catch (e) {
      this.toast(ApiClient.formatError(e as Object), true);
    }
  }

  private async openCreateForward(): Promise<void> {
    await this.ensureHosts();
    this.editingForwardId = '';
    this.fwdHostId = this.hostOptions.length > 0 ? this.hostOptions[0].id : '';
    this.fwdTargetPort = '';
    this.fwdLocalPort = '';
    this.fwdProtocol = 'tcp';
    this.showForwardForm = true;
  }

  private async openEditForward(f: PortForwardRule): Promise<void> {
    await this.ensureHosts();
    this.editingForwardId = f.id;
    this.fwdHostId = f.host_id || '';
    this.fwdTargetPort = `${this.forwardTargetPort(f) || ''}`;
    this.fwdLocalPort = f.local_port && f.local_port > 0 ? `${f.local_port}` : '';
    this.fwdProtocol = f.protocol || 'tcp';
    this.showForwardForm = true;
  }

  private async saveForward(): Promise<void> {
    const hostId = this.fwdHostId.trim();
    const target = Number(this.fwdTargetPort) || 0;
    const local = Number(this.fwdLocalPort) || 0;
    if (!hostId || target < 1 || target > 65535) {
      this.toast('请填写目标主机与合法目标端口', true);
      return;
    }
    this.forwardBusy = true;
    try {
      if (this.editingForwardId) {
        const body: PortForwardEditRequest = {
          host_id: hostId,
          target_port: target,
          local_port: local
        };
        await Api.updatePortForward(this.editingForwardId, body);
        this.toast('转发规则已更新');
      } else {
        const body: PortForwardCreateRequest = {
          host_id: hostId,
          target_port: target,
          local_port: local,
          protocol: this.fwdProtocol || 'tcp'
        };
        await Api.createPortForward(body);
        this.toast('转发规则已创建');
      }
      this.showForwardForm = false;
      await this.refresh();
    } catch (e) {
      this.toast(ApiClient.formatError(e as Object), true);
    } finally {
      this.forwardBusy = false;
    }
  }

  private async openCreateProxy(): Promise<void> {
    await this.ensureHosts();
    this.editingProxyId = '';
    this.pxHostId = this.hostOptions.length > 0 ? this.hostOptions[0].id : '';
    this.pxName = '';
    this.pxTargetPort = '';
    this.pxDefaultPath = '';
    this.showProxyForm = true;
  }

  private async openEditProxy(p: HTTPProxyConfig): Promise<void> {
    await this.ensureHosts();
    this.editingProxyId = p.id;
    this.pxHostId = p.host_id || '';
    this.pxName = p.name || '';
    this.pxTargetPort = `${this.proxyTargetPort(p) || ''}`;
    this.pxDefaultPath = p.default_path || '';
    this.showProxyForm = true;
  }

  private async saveProxy(): Promise<void> {
    const hostId = this.pxHostId.trim();
    const target = Number(this.pxTargetPort) || 0;
    if (!hostId || target < 1 || target > 65535) {
      this.toast('请填写目标主机与合法端口', true);
      return;
    }
    this.forwardBusy = true;
    try {
      const body: HTTPProxyConfig = {
        id: this.editingProxyId || '',
        name: this.pxName.trim(),
        host_id: hostId,
        target_port: target,
        default_path: this.pxDefaultPath.trim(),
        enabled: true
      };
      if (this.editingProxyId) {
        await Api.updateHTTPProxy(this.editingProxyId, body);
        this.toast('HTTP 代理已更新');
      } else {
        await Api.createHTTPProxy(body);
        this.toast('HTTP 代理已创建');
      }
      this.showProxyForm = false;
      await this.refresh();
    } catch (e) {
      this.toast(ApiClient.formatError(e as Object), true);
    } finally {
      this.forwardBusy = false;
    }
  }

  private async doDeleteForward(): Promise<void> {
    try {
      await Api.deletePortForward(this.deleteForwardId);
      this.showDeleteForward = false;
      this.deleteForwardId = '';
      this.toast('已删除转发规则');
      await this.refresh();
    } catch (e) {
      this.toast(ApiClient.formatError(e as Object), true);
    }
  }

  private async doDeleteProxy(): Promise<void> {
    try {
      await Api.deleteHTTPProxy(this.deleteProxyId);
      this.showDeleteProxy = false;
      this.deleteProxyId = '';
      this.toast('已删除 HTTP 代理');
      await this.refresh();
    } catch (e) {
      this.toast(ApiClient.formatError(e as Object), true);
    }
  }

  private async copyProxyUrl(p: HTTPProxyConfig): Promise<void> {
    try {
      const tokenResp = await Api.proxyToken();
      const base = ApiClient.buildProxyUrl(p.host_id || '', this.proxyTargetPort(p), p.default_path || '');
      const url = tokenResp.token ? `${base}${base.indexOf('?') >= 0 ? '&' : '?'}pt=${tokenResp.token}` : base;
      const data = pasteboard.createData(pasteboard.MIMETYPE_TEXT_PLAIN, url);
      pasteboard.getSystemPasteboard().setData(data);
      this.toast('已复制代理地址');
    } catch (e) {
      this.toast(ApiClient.formatError(e as Object), true);
    }
  }

  async approveRem(r: RemediationRun): Promise<void> {
    try {
      await Api.approveRemediation(r.id);
      this.toast('已批准');
      await this.refresh();
    } catch (e) {
      this.toast(ApiClient.formatError(e as Object), true);
    }
  }

  async rejectRem(r: RemediationRun): Promise<void> {
    try {
      await Api.rejectRemediation(r.id);
      this.toast('已拒绝');
      await this.refresh();
    } catch (e) {
      this.toast(ApiClient.formatError(e as Object), true);
    }
  }

  async commentOnTicket(t: Ticket): Promise<void> {
    if (!this.ticketComment.trim()) {
      return;
    }
    try {
      await Api.commentTicket(t.id, this.ticketComment.trim());
      this.ticketComment = '';
      this.toast('已添加备注');
      await this.refresh();
    } catch (e) {
      this.toast(ApiClient.formatError(e as Object), true);
    }
  }

  async setTicketStatus(t: Ticket, status: string): Promise<void> {
    try {
      await Api.updateTicket(t.id, status);
      this.toast(`工单已标记为 ${status}`);
      await this.refresh();
    } catch (e) {
      this.toast(ApiClient.formatError(e as Object), true);
    }
  }

  async assignTicketTo(t: Ticket, username: string): Promise<void> {
    try {
      await Api.assignTicket(t.id, username);
      this.assignVisible = false;
      this.assignTicketId = 0;
      this.toast(username ? `已指派给 ${username}` : '已取消指派');
      await this.refresh();
    } catch (e) {
      this.toast(ApiClient.formatError(e as Object), true);
    }
  }

  private filteredTerminalAudit(): ActivityLogEntry[] {
    const q = this.terminalAuditQuery.trim().toLowerCase();
    if (!q) {
      return this.terminalAuditLogs;
    }
    return this.terminalAuditLogs.filter((e: ActivityLogEntry) => {
      const hay = `${e.actor ?? ''} ${e.host ?? ''} ${e.message ?? ''} ${e.ip ?? ''}`.toLowerCase();
      return hay.indexOf(q) >= 0;
    });
  }

  private levelColor(level: string): string {
    const l = (level || '').toLowerCase();
    if (l.indexOf('err') >= 0 || l === 'critical' || l === 'fatal') {
      return this.tDanger;
    }
    if (l.indexOf('warn') >= 0) {
      return this.tWarning;
    }
    return this.tMuted;
  }

  build() {
    Stack() {
      Column() {
        PageHeader({
          title: '运维中心',
          subtitle: '事件闭环 · 日志检索 · 自动化剧本 · 15 秒刷新',
          showLoading: this.syncing,
          trailing: '刷新',
          onTrailing: () => {
            void this.refresh(false);
          }
        })
        ErrorBanner({
          message: this.error,
          showRetry: this.error.length > 0,
          onRetry: () => {
            void this.refresh(false);
          }
        })

        OpsKpiStrip({
          items: [
            { label: '开放事件', value: `${this.openIncidents()}`, color: this.tDanger },
            { label: '待审批', value: `${this.pendingRemediations()}`, color: this.tWarning },
            { label: '开放工单', value: `${this.openTickets()}`, color: this.tAccent },
            { label: 'SLO 燃烧', value: `${this.sloBurning()}`, color: '#8B5CF6' }
          ],
          onSelect: (i: number) => this.jumpKpi(i)
        })

        FilterChips({
          labels: this.tabLabels,
          selected: $tab,
          onSelect: (_i: number) => {
            void this.refresh();
          }
        })
          .padding({ left: 12, right: 12, bottom: 8 })

        if (this.tab === 0) {
          this.incidentsPane()
        } else if (this.tab === 1) {
          this.logsPane()
        } else if (this.tab === 2) {
          this.playbooksPane()
        } else if (this.tab === 3) {
          this.forwardsPane()
        } else {
          this.govPane()
        }
      }
      .width('100%')
      .height('100%')
      .backgroundColor(this.tBg)

      ConfirmDialogHost({
        visible: this.playbookConfirm,
        title: '执行剧本？',
        message: `确认执行「${this.playbookName || this.playbookTarget}」？请确认 Web 端已审核目标与命令，执行后不可撤销。`,
        confirmText: '执行',
        onConfirm: () => {
          void this.doExecutePlaybook();
        },
        onCancel: () => {
          this.playbookConfirm = false;
        }
      })

      if (this.createVisible) {
        Column() {
          Blank()
            .layoutWeight(1)
            .width('100%')
            .onClick(() => {
              if (!this.createBusy) {
                this.createVisible = false;
              }
            })
          Column() {
            Text('新建事件')
              .fontSize(AppTheme.fontLead)
              .fontWeight(FontWeight.Bold)
              .fontColor(this.tText)
              .margin({ bottom: 12 })
            Text('事件标题').fontSize(AppTheme.fontCaption).fontColor(this.tMuted)
            TextInput({ text: this.createTitle, placeholder: '简述故障现象' })
              .height(40)
              .margin({ top: 6, bottom: 10 })
              .backgroundColor(this.tElevated)
              .fontColor(this.tText)
              .onChange((v: string) => {
                this.createTitle = v;
              })
            Row({ space: 8 }) {
              ForEach(this.sevLabels, (lab: string, i: number) => {
                Text(lab)
                  .fontSize(AppTheme.fontCaption)
                  .fontColor(this.createSev === i ? this.tOnAccent : this.tMuted)
                  .padding({ left: 10, right: 10, top: 6, bottom: 6 })
                  .backgroundColor(this.createSev === i ? this.tAccent : this.tElevated)
                  .borderRadius(AppTheme.rSm)
                  .onClick(() => {
                    this.createSev = i;
                  })
              }, (lab: string, i: number) => `sev-${i}`)
            }
            .margin({ bottom: 16 })
            Row({ space: 12 }) {
              Button('取消')
                .layoutWeight(1)
                .height(40)
                .backgroundColor(this.tElevated)
                .fontColor(this.tText)
                .onClick(() => {
                  this.createVisible = false;
                })
              Button(this.createBusy ? '创建中…' : '创建')
                .layoutWeight(1)
                .height(40)
                .enabled(!this.createBusy)
                .backgroundColor(this.tAccent)
                .fontColor(this.tOnAccent)
                .onClick(() => {
                  void this.doCreateIncident();
                })
            }
            .width('100%')
          }
          .width('84%')
          .padding(AppTheme.pagePadH)
          .backgroundColor(this.tCard)
          .borderRadius(AppTheme.rSm)
          .border({ width: 1, color: this.tBorderSoft })
          Blank().layoutWeight(1)
        }
        .width('100%')
        .height('100%')
        .justifyContent(FlexAlign.Center)
        .backgroundColor(this.tScrim)
        .zIndex(95)
      }

      DetailSheet({
        visible: this.diagVisible,
        title: `事件 AI 诊断`,
        onClose: () => {
          this.stopDiag();
          this.diagVisible = false;
        }
      }) {
        Column({ space: 8 }) {
          Text(`#${this.diagIncidentId} ${this.diagTitle}`)
            .fontSize(AppTheme.fontMicro)
            .fontColor(this.tMuted)
            .maxLines(1)
            .textOverflow({ overflow: TextOverflow.Ellipsis })
            .width('100%')

          Row({ space: 9 }) {
            if (this.diagBusy) {
              LoadingProgress().width(18).height(18).color(this.tAccent)
            } else {
              Circle({ width: 8, height: 8 })
                .fill(this.diagError ? this.tDanger : this.tSuccess)
            }
            Column() {
              Text(this.diagActivity || (this.diagBusy ? '诊断中…' : '就绪'))
                .fontSize(AppTheme.fontCaption)
                .fontWeight(FontWeight.Medium)
                .fontColor(this.tText)
              Text(this.diagBusy ? '窗口保持打开，诊断结果会实时显示在下方' : '可继续追问，回答会保留事件上下文')
                .fontSize(AppTheme.fontTiny)
                .fontColor(this.tMuted)
                .margin({ top: 2 })
            }
            .alignItems(HorizontalAlign.Start)
            .layoutWeight(1)
            if (this.diagBusy) {
              Text('停止')
                .fontSize(AppTheme.fontCaption)
                .fontColor(this.tWarning)
                .onClick(() => this.stopDiag())
            }
          }
          .width('100%')
          .padding(10)
          .backgroundColor(this.diagBusy ? this.tAccentSoft : this.tSuccessSoft)
          .borderRadius(AppTheme.rSm)

          if (this.diagError) {
            Column({ space: 6 }) {
              Text(this.diagError)
                .fontSize(AppTheme.fontCaption)
                .fontColor(this.tDanger)
                .width('100%')
              Text('事件详情仍可正常处理。请检查服务端 AI 配置后重试；Provider / 告警通道在网页端设置。')
                .fontSize(AppTheme.fontMicro)
                .fontColor(this.tMuted)
                .width('100%')
              Text('打开设置说明')
                .fontSize(AppTheme.fontCaption)
                .fontColor(this.tAccent)
                .onClick(() => {
                  this.diagVisible = false;
                  NavUtil.push('pages/SettingsPage');
                })
            }
            .width('100%')
            .padding(10)
            .backgroundColor(this.tDangerSoft)
            .borderRadius(AppTheme.rSm)
            .alignItems(HorizontalAlign.Start)
          }

          Scroll() {
            Column({ space: 8 }) {
              if (this.diagMessages.length === 0 && this.diagBusy) {
                Text('正在建立诊断上下文，请稍候…')
                  .fontSize(AppTheme.fontMicro)
                  .fontColor(this.tMuted)
                  .width('100%')
              }
              ForEach(this.diagMessages, (m: DiagnosisChatMessage, mi: number) => {
                Row() {
                  if ((m.role || '') === 'user') {
                    Blank().layoutWeight(1)
                    Column() {
                      Text('我')
                        .fontSize(AppTheme.fontTiny)
                        .fontWeight(FontWeight.Bold)
                        .fontColor(this.tAccent)
                      Text(m.content || '')
                        .fontSize(AppTheme.fontSmall)
                        .fontColor(this.tOnAccent)
                        .margin({ top: 4 })
                    }
                    .alignItems(HorizontalAlign.Start)
                    .padding(10)
                    .backgroundColor(this.tAccent)
                    .borderRadius(AppTheme.rSm)
                    .constraintSize({ maxWidth: '84%' })
                  } else {
                    Column() {
                      Text('AIOps 诊断引擎')
                        .fontSize(AppTheme.fontTiny)
                        .fontWeight(FontWeight.Bold)
                        .fontColor(this.tAccent)
                      if (!(m.content || '').trim() && this.diagBusy && mi === this.diagMessages.length - 1) {
                        Row({ space: 7 }) {
                          LoadingProgress().width(14).height(14).color(this.tAccent)
                          Text('正在生成回复…')
                            .fontSize(AppTheme.fontMicro)
                            .fontColor(this.tMuted)
                        }
                        .margin({ top: 6 })
                      } else {
                        MarkdownText({
                          content: m.content || this.diagText || '',
                          color: this.tText,
                          fontSize: AppTheme.fontSmall
                        })
                          .margin({ top: 4 })
                          .width('100%')
                      }
                    }
                    .alignItems(HorizontalAlign.Start)
                    .padding(10)
                    .backgroundColor(this.tElevated)
                    .borderRadius(AppTheme.rSm)
                    .constraintSize({ maxWidth: '94%' })
                    Blank().layoutWeight(1)
                  }
                }
                .width('100%')
              }, (m: DiagnosisChatMessage, mi: number) => `dm-${mi}-${(m.content || '').length}`)
            }
            .width('100%')
            .alignItems(HorizontalAlign.Start)
          }
          .constraintSize({ maxHeight: 420 })
          .scrollBar(BarState.Off)
          .width('100%')
          .layoutWeight(1)

          Row() {
            Column() {
              Text('结合终端操作记录')
                .fontSize(AppTheme.fontMicro)
                .fontColor(this.tText)
              Text('仅发送该事件关联主机的终端摘要')
                .fontSize(AppTheme.fontTiny)
                .fontColor(this.tMuted)
                .margin({ top: 2 })
            }
            .alignItems(HorizontalAlign.Start)
            .layoutWeight(1)
            Toggle({ type: ToggleType.Switch, isOn: this.diagIncludeTerminal })
              .selectedColor(this.tAccent)
              .enabled(!this.diagBusy)
              .onChange((v: boolean) => {
                this.diagIncludeTerminal = v;
              })
          }
          .width('100%')
          .padding({ top: 4, bottom: 4 })

          if (this.hasAssistantReply() && !this.diagBusy) {
            Row({ space: 12 }) {
              Text('有帮助').fontSize(AppTheme.fontCaption).fontColor(this.tSuccess)
                .onClick(() => { void this.sendDiagFeedback(true); })
              Text('无帮助').fontSize(AppTheme.fontCaption).fontColor(this.tMuted)
                .onClick(() => { void this.sendDiagFeedback(false); })
            }
            .width('100%')
          }

          Row({ space: 8 }) {
            TextInput({ text: this.diagInput, placeholder: '追问根因、验证方法或回滚步骤…' })
              .layoutWeight(1)
              .height(40)
              .enabled(!this.diagBusy)
              .backgroundColor(this.tElevated)
              .fontColor(this.tText)
              .onChange((v: string) => { this.diagInput = v; })
            Button(this.diagBusy ? '…' : '发送')
              .height(40)
              .enabled(!this.diagBusy)
              .backgroundColor(this.tAccent)
              .fontColor(this.tOnAccent)
              .onClick(() => { void this.sendDiagFollowUp(); })
          }
          .width('100%')
        }
        .width('100%')
        .alignItems(HorizontalAlign.Start)
        .padding({ bottom: 12 })
      }

      DetailSheet({
        visible: this.logDiagVisible,
        title: '日志诊断',
        onClose: () => { this.logDiagVisible = false; }
      }) {
        Text(this.logDiagBusy ? '诊断中…' : (this.logDiagText || '暂无结果'))
          .fontSize(AppTheme.fontSmall)
          .fontColor(this.tText)
          .width('100%')
          .padding({ bottom: 12 })
      }

      DetailSheet({
        visible: this.detailVisible,
        title: this.detailInc?.title || '事件详情',
        onClose: () => {
          this.detailVisible = false;
          this.detailInc = null;
        }
      }) {
        Column() {
          if (this.detailInc) {
            Text(`${FormatUtil.levelLabel(this.detailInc.severity || '')} · ${FormatUtil.incidentStatusZh(this.detailInc.status || '')} · ${this.detailInc.hostname ?? ''}`)
              .fontSize(AppTheme.fontCaption)
              .fontColor(this.tMuted)
              .margin({ bottom: 8 })
            if (this.detailInc.summary) {
              Text(this.detailInc.summary)
                .fontSize(AppTheme.fontSmall)
                .fontColor(this.tText)
                .margin({ bottom: 10 })
            }
            Text('时间线').fontSize(AppTheme.fontBody).fontWeight(FontWeight.Medium).fontColor(this.tText).margin({ bottom: 6 })
            if (this.timelineOf(this.detailInc).length === 0) {
              Text('暂无时间线').fontSize(AppTheme.fontCaption).fontColor(this.tMuted)
            } else {
              ForEach(this.timelineOf(this.detailInc), (ev: IncidentEvent, i: number) => {
                Column() {
                  Text(`${ev.kind ?? '事件'} · ${FormatUtil.relTime(ev.ts ?? 0)}`)
                    .fontSize(AppTheme.fontMicro)
                    .fontColor(this.tAccent)
                  Text(ev.text || '')
                    .fontSize(AppTheme.fontCaption)
                    .fontColor(this.tText)
                    .margin({ top: 2 })
                  if (ev.actor) {
                    Text(ev.actor).fontSize(AppTheme.fontTiny).fontColor(this.tMuted).margin({ top: 2 })
                  }
                }
                .width('100%')
                .alignItems(HorizontalAlign.Start)
                .padding(10)
                .margin({ bottom: 6 })
                .backgroundColor(this.tElevated)
                .borderRadius(AppTheme.rXs)
              }, (ev: IncidentEvent, i: number) => `tl-${i}-${ev.ts ?? 0}`)
            }
            if (!this.incResolved(this.detailInc)) {
              Button('转工单')
                .width('100%')
                .height(40)
                .margin({ top: 12 })
                .backgroundColor(this.tWarningSoft)
                .fontColor(this.tWarning)
                .enabled(this.busyKey.length === 0)
                .onClick(() => {
                  const inc = this.detailInc;
                  if (inc) {
                    void this.doEscalate(inc);
                  }
                })
            }
          }
        }
        .width('100%')
        .alignItems(HorizontalAlign.Start)
      }

      ConfirmDialogHost({
        visible: this.showDeleteForward,
        title: '删除转发规则',
        message: '确认删除该端口转发规则？此操作不可撤销。',
        confirmText: '删除',
        danger: true,
        onConfirm: () => {
          void this.doDeleteForward();
        },
        onCancel: () => {
          this.showDeleteForward = false;
        }
      })

      ConfirmDialogHost({
        visible: this.showDeleteProxy,
        title: '删除 HTTP 代理',
        message: '确认删除该 HTTP 代理？此操作不可撤销。',
        confirmText: '删除',
        danger: true,
        onConfirm: () => {
          void this.doDeleteProxy();
        },
        onCancel: () => {
          this.showDeleteProxy = false;
        }
      })

      if (this.showForwardForm) {
        Column() {
          Blank().layoutWeight(1).onClick(() => {
            if (!this.forwardBusy) {
              this.showForwardForm = false;
            }
          })
          Column({ space: 8 }) {
            Text(this.editingForwardId ? '编辑端口转发' : '新增端口转发')
              .fontSize(AppTheme.fontLead)
              .fontWeight(FontWeight.Medium)
              .fontColor(this.tText)
            TextInput({ text: this.fwdHostId, placeholder: '目标主机 ID' })
              .height(40)
              .backgroundColor(this.tElevated)
              .fontColor(this.tText)
              .onChange((v: string) => {
                this.fwdHostId = v;
              })
            if (this.hostOptions.length > 0) {
              Scroll() {
                Row({ space: 8 }) {
                  ForEach(this.hostOptions.slice(0, 12), (h: Host) => {
                    Text(h.hostname || h.id)
                      .fontSize(AppTheme.fontMicro)
                      .fontColor(this.fwdHostId === h.id ? this.tOnAccent : this.tText)
                      .padding({ left: 8, right: 8, top: 4, bottom: 4 })
                      .backgroundColor(this.fwdHostId === h.id ? this.tAccent : this.tElevated)
                      .borderRadius(AppTheme.rChip)
                      .onClick(() => {
                        this.fwdHostId = h.id;
                      })
                  }, (h: Host) => `fwd-host-${h.id}`)
                }
              }
              .scrollable(ScrollDirection.Horizontal)
              .scrollBar(BarState.Off)
              .height(36)
            }
            TextInput({ text: this.fwdTargetPort, placeholder: '目标端口' })
              .height(40)
              .backgroundColor(this.tElevated)
              .fontColor(this.tText)
              .type(InputType.Number)
              .onChange((v: string) => {
                this.fwdTargetPort = v;
              })
            TextInput({ text: this.fwdLocalPort, placeholder: '本地端口 (0=自动)' })
              .height(40)
              .backgroundColor(this.tElevated)
              .fontColor(this.tText)
              .type(InputType.Number)
              .onChange((v: string) => {
                this.fwdLocalPort = v;
              })
            Row({ space: 8 }) {
              Text('TCP')
                .fontSize(AppTheme.fontCaption)
                .fontColor(this.fwdProtocol === 'tcp' ? this.tOnAccent : this.tText)
                .padding({ left: 12, right: 12, top: 6, bottom: 6 })
                .backgroundColor(this.fwdProtocol === 'tcp' ? this.tAccent : this.tElevated)
                .borderRadius(AppTheme.rXs)
                .onClick(() => {
                  this.fwdProtocol = 'tcp';
                })
              Text('UDP')
                .fontSize(AppTheme.fontCaption)
                .fontColor(this.fwdProtocol === 'udp' ? this.tOnAccent : this.tText)
                .padding({ left: 12, right: 12, top: 6, bottom: 6 })
                .backgroundColor(this.fwdProtocol === 'udp' ? this.tAccent : this.tElevated)
                .borderRadius(AppTheme.rXs)
                .onClick(() => {
                  this.fwdProtocol = 'udp';
                })
            }
            Row({ space: 12 }) {
              Button('取消')
                .layoutWeight(1)
                .backgroundColor(this.tElevated)
                .enabled(!this.forwardBusy)
                .onClick(() => {
                  this.showForwardForm = false;
                })
              Button('保存')
                .layoutWeight(1)
                .backgroundColor(this.tAccent)
                .enabled(!this.forwardBusy)
                .onClick(() => {
                  void this.saveForward();
                })
            }
          }
          .width('100%')
          .padding(AppTheme.pagePadH)
          .backgroundColor(this.tCard)
          .borderRadius({ topLeft: AppTheme.rMd, topRight: AppTheme.rMd })
        }
        .width('100%')
        .height('100%')
        .backgroundColor(this.tScrim)
        .position({ x: 0, y: 0 })
        .zIndex(50)
      }

      if (this.showProxyForm) {
        Column() {
          Blank().layoutWeight(1).onClick(() => {
            if (!this.forwardBusy) {
              this.showProxyForm = false;
            }
          })
          Column({ space: 8 }) {
            Text(this.editingProxyId ? '编辑 HTTP 代理' : '新增 HTTP 代理')
              .fontSize(AppTheme.fontLead)
              .fontWeight(FontWeight.Medium)
              .fontColor(this.tText)
            TextInput({ text: this.pxName, placeholder: '名称（可选）' })
              .height(40)
              .backgroundColor(this.tElevated)
              .fontColor(this.tText)
              .onChange((v: string) => {
                this.pxName = v;
              })
            TextInput({ text: this.pxHostId, placeholder: '目标主机 ID' })
              .height(40)
              .backgroundColor(this.tElevated)
              .fontColor(this.tText)
              .onChange((v: string) => {
                this.pxHostId = v;
              })
            if (this.hostOptions.length > 0) {
              Scroll() {
                Row({ space: 8 }) {
                  ForEach(this.hostOptions.slice(0, 12), (h: Host) => {
                    Text(h.hostname || h.id)
                      .fontSize(AppTheme.fontMicro)
                      .fontColor(this.pxHostId === h.id ? this.tOnAccent : this.tText)
                      .padding({ left: 8, right: 8, top: 4, bottom: 4 })
                      .backgroundColor(this.pxHostId === h.id ? this.tAccent : this.tElevated)
                      .borderRadius(AppTheme.rChip)
                      .onClick(() => {
                        this.pxHostId = h.id;
                      })
                  }, (h: Host) => `px-host-${h.id}`)
                }
              }
              .scrollable(ScrollDirection.Horizontal)
              .scrollBar(BarState.Off)
              .height(36)
            }
            TextInput({ text: this.pxTargetPort, placeholder: '目标端口' })
              .height(40)
              .backgroundColor(this.tElevated)
              .fontColor(this.tText)
              .type(InputType.Number)
              .onChange((v: string) => {
                this.pxTargetPort = v;
              })
            TextInput({ text: this.pxDefaultPath, placeholder: '默认路径（可选）' })
              .height(40)
              .backgroundColor(this.tElevated)
              .fontColor(this.tText)
              .onChange((v: string) => {
                this.pxDefaultPath = v;
              })
            Row({ space: 12 }) {
              Button('取消')
                .layoutWeight(1)
                .backgroundColor(this.tElevated)
                .enabled(!this.forwardBusy)
                .onClick(() => {
                  this.showProxyForm = false;
                })
              Button('保存')
                .layoutWeight(1)
                .backgroundColor(this.tAccent)
                .enabled(!this.forwardBusy)
                .onClick(() => {
                  void this.saveProxy();
                })
            }
          }
          .width('100%')
          .padding(AppTheme.pagePadH)
          .backgroundColor(this.tCard)
          .borderRadius({ topLeft: AppTheme.rMd, topRight: AppTheme.rMd })
        }
        .width('100%')
        .height('100%')
        .backgroundColor(this.tScrim)
        .position({ x: 0, y: 0 })
        .zIndex(50)
      }

      ToastHost({ message: this.toastMsg, isError: this.toastError })

      DetailSheet({
        visible: this.assignVisible,
        title: '指派工单',
        onClose: () => {
          this.assignVisible = false;
          this.assignTicketId = 0;
        }
      }) {
        Column({ space: 8 }) {
          Text('选择处理人（点选即指派）')
            .fontSize(AppTheme.fontCaption)
            .fontColor(this.tMuted)
            .width('100%')
          Text('取消指派')
            .fontSize(AppTheme.fontBody)
            .fontColor(this.tWarning)
            .width('100%')
            .padding({ top: 10, bottom: 10 })
            .onClick(() => {
              const t = this.tickets.find((x: Ticket) => x.id === this.assignTicketId);
              if (t) {
                void this.assignTicketTo(t, '');
              }
            })
          if (this.directoryUsers.length === 0) {
            Text('暂无可用用户目录，可稍后在网页端配置或刷新重试')
              .fontSize(AppTheme.fontCaption)
              .fontColor(this.tMuted)
              .width('100%')
          } else {
            ForEach(this.directoryUsers, (u: DirectoryUser) => {
              Text(u.label || u.display_name || u.username)
                .fontSize(AppTheme.fontBody)
                .fontColor(this.tText)
                .width('100%')
                .padding({ top: 10, bottom: 10 })
                .onClick(() => {
                  const t = this.tickets.find((x: Ticket) => x.id === this.assignTicketId);
                  if (t) {
                    void this.assignTicketTo(t, u.username);
                  }
                })
            }, (u: DirectoryUser) => `du-${u.username}`)
          }
        }
        .width('100%')
        .alignItems(HorizontalAlign.Start)
      }
    }
    .width('100%')
    .height('100%')
  }

  @Builder
  incidentsPane() {
    // FAB 只挂在本页局部 Stack，alignContent 锚定右下；不占用全屏叠层
    Stack({ alignContent: Alignment.BottomEnd }) {
      Column() {
        FilterChips({
          labels: this.incFilterLabels,
          selected: $incidentFilter
        })
          .padding({ left: 12, right: 12, bottom: 8 })

        if (this.filteredIncidents().length === 0) {
          EmptyHint({ text: '暂无事件', fill: true })
        } else {
          List() {
            ForEach(this.filteredIncidents(), (inc: Incident) => {
              ListItem() {
                Column() {
                  Row() {
                    Text(inc.title || `#${inc.id}`)
                      .fontSize(AppTheme.fontBody)
                      .fontWeight(FontWeight.Medium)
                      .fontColor(this.tText)
                      .layoutWeight(1)
                      .maxLines(2)
                      .textOverflow({ overflow: TextOverflow.Ellipsis })
                    SeverityBadge({ level: inc.severity || 'info' })
                  }
                  .width('100%')
                  Text(`${FormatUtil.incidentStatusZh(inc.status || '')} · ${inc.hostname || inc.host_id || '—'} · ${FormatUtil.relTime(inc.created_at ?? 0)}`)
                    .fontSize(AppTheme.fontMicro)
                    .fontColor(this.tMuted)
                    .margin({ top: 4 })
                  Row({ space: 12 }) {
                    Text('详情').fontSize(AppTheme.fontCaption).fontColor(this.tMuted)
                      .onClick(() => {
                        this.detailInc = inc;
                        this.detailVisible = true;
                      })
                    if (this.incOpen(inc) && !this.incResolved(inc)) {
                      Text('确认')
                        .fontSize(AppTheme.fontCaption)
                        .fontColor(this.busyKey === `ack-${inc.id}` ? this.tMuted : this.tAccent)
                        .opacity(this.busyKey.indexOf(`${inc.id}`) >= 0 ? 0.5 : 1)
                        .enabled(this.busyKey.length === 0)
                        .onClick(() => {
                          void this.doAck(inc);
                        })
                    }
                    if (!this.incResolved(inc)) {
                      Text('解决')
                        .fontSize(AppTheme.fontCaption)
                        .fontColor(this.tSuccess)
                        .opacity(this.busyKey.indexOf(`${inc.id}`) >= 0 ? 0.5 : 1)
                        .enabled(this.busyKey.length === 0)
                        .onClick(() => {
                          void this.doResolve(inc);
                        })
                      Text('AI 诊断').fontSize(AppTheme.fontCaption).fontColor('#8B5CF6')
                        .onClick(() => {
                          void this.runDiag(inc);
                        })
                    } else {
                      Text('AI 诊断').fontSize(AppTheme.fontCaption).fontColor('#8B5CF6')
                        .onClick(() => {
                          void this.runDiag(inc);
                        })
                    }
                  }
                  .margin({ top: 8 })
                }
                .width('100%')
                .alignItems(HorizontalAlign.Start)
                .padding(AppTheme.spaceMd)
                .backgroundColor(this.tCard)
                .borderRadius(AppTheme.rSm)
                .borderWidth({ left: 4, right: 1, top: 1, bottom: 1 })
                .borderColor({
                  left: FormatUtil.sevColor(inc.severity || 'info'),
                  right: this.tBorderSoft,
                  top: this.tBorderSoft,
                  bottom: this.tBorderSoft
                })
              }
              .width('100%')
              .padding({ left: 12, right: 12, bottom: 8 })
            }, (inc: Incident) => `${inc.id}`)

            // 末尾留白，避免最后一张卡被右下角「+」挡住（不产生独立灰/白条容器）
            ListItem() {
              Column()
                .width('100%')
                .height(72)
            }
          }
          .layoutWeight(1)
          .width('100%')
          .scrollBar(BarState.Off)
          .edgeEffect(EdgeEffect.Spring)
        }
      }
      .width('100%')
      .height('100%')
      .align(Alignment.TopStart)

      // 仅 48×48 圆形加号，无全宽父级
      Text('+')
        .width(AppTheme.fabSize)
        .height(AppTheme.fabSize)
        .fontSize(28)
        .fontWeight(FontWeight.Medium)
        .fontColor(this.tOnAccent)
        .textAlign(TextAlign.Center)
        .backgroundColor(this.tAccent)
        .borderRadius(AppTheme.fabSize / 2)
        .shadow({ radius: 8, color: this.tShadow, offsetY: 3 })
        .margin({ right: 16, bottom: 16 })
        .onClick(() => {
          this.createVisible = true;
        })
    }
    .width('100%')
    .layoutWeight(1)
  }

  @Builder
  logsPane() {
    Column() {
      FilterChips({
        labels: this.logSourceLabels,
        selected: $logSourceTab,
        variant: 'subtab',
        onSelect: (_i: number) => {
          void this.refresh();
        }
      })
        .padding({ left: 12, right: 12, bottom: 8 })

      if (this.logSourceTab === 0) {
        SearchBar({
          text: $logQuery,
          placeholder: '搜索 Agent 日志…',
          onSubmit: () => {
            void this.refresh();
          }
        })
          .padding({ left: 12, right: 12, bottom: 6 })
        FilterChips({
          labels: this.logLevelLabels,
          selected: $logLevelIdx,
          onSelect: (_i: number) => {
            void this.refresh();
          }
        })
          .padding({ left: 12, right: 12, bottom: 6 })
        FilterChips({
          labels: this.logRangeLabels,
          selected: $logRangeIdx,
          onSelect: (_i: number) => {
            void this.refresh();
          }
        })
          .padding({ left: 12, right: 12, bottom: 8 })

        if (this.logs.length === 0) {
          EmptyHint({ text: '暂无 Agent 日志', fill: true })
        } else {
          List() {
            ForEach(this.logs, (row: LogEntry, i: number) => {
              ListItem() {
                Column() {
                  Row() {
                    Text((row.level || 'info').toUpperCase())
                      .fontSize(AppTheme.fontTiny)
                      .fontWeight(FontWeight.Bold)
                      .fontColor(this.levelColor(row.level || ''))
                      .padding({ left: 6, right: 6, top: 2, bottom: 2 })
                      .backgroundColor(this.tElevated)
                      .borderRadius(AppTheme.rChip)
                    Text(row.host || '—')
                      .fontSize(AppTheme.fontMicro)
                      .fontColor(this.tMuted)
                      .margin({ left: 8 })
                      .layoutWeight(1)
                    Text(FormatUtil.relTime(row.timestamp ?? 0))
                      .fontSize(AppTheme.fontTiny)
                      .fontColor(this.tMuted)
                  }
                  .width('100%')
                  Text(row.message || '')
                    .fontSize(AppTheme.fontCaption)
                    .fontColor(this.tText)
                    .margin({ top: 4 })
                    .maxLines(4)
                    .textOverflow({ overflow: TextOverflow.Ellipsis })
                  Text('AI 诊断')
                    .fontSize(AppTheme.fontCaption)
                    .fontColor(this.tAccent)
                    .margin({ top: 8 })
                    .onClick(() => { void this.diagnoseLogRow(row); })
                }
                .width('100%')
                .alignItems(HorizontalAlign.Start)
                .padding(AppTheme.spaceMd)
                .backgroundColor(this.tCard)
                .borderRadius(AppTheme.rSm)
              }
              .padding({ left: 12, right: 12, bottom: 8 })
            }, (row: LogEntry, i: number) => `log-${i}-${row.timestamp ?? 0}`)
          }
          .layoutWeight(1)
          .scrollBar(BarState.Off)
        }
      } else {
        SearchBar({
          text: $terminalAuditQuery,
          placeholder: '搜索终端审计（用户/主机/命令）…',
          onSubmit: () => {
            // local filter via @Watch not needed — filteredTerminalAudit reads state
          }
        })
          .padding({ left: 12, right: 12, bottom: 8 })
        Text(`终端审计记录：${this.filteredTerminalAudit().length}/${this.terminalAuditLogs.length} 条`)
          .fontSize(AppTheme.fontMicro)
          .fontColor(this.tMuted)
          .width('100%')
          .padding({ left: 16, right: 16, bottom: 6 })

        if (this.filteredTerminalAudit().length === 0) {
          EmptyHint({ text: '暂无终端审计记录', fill: true })
        } else {
          List() {
            ForEach(this.filteredTerminalAudit(), (row: ActivityLogEntry, i: number) => {
              ListItem() {
                Column() {
                  Row() {
                    Text((row.actor || '—'))
                      .fontSize(AppTheme.fontCaption)
                      .fontWeight(FontWeight.Medium)
                      .fontColor(this.tText)
                      .layoutWeight(1)
                    Text(FormatUtil.relTime(row.timestamp ?? 0))
                      .fontSize(AppTheme.fontTiny)
                      .fontColor(this.tMuted)
                  }
                  .width('100%')
                  Text(`${row.host || '—'} · ${row.ip || ''}`)
                    .fontSize(AppTheme.fontMicro)
                    .fontColor(this.tMuted)
                    .margin({ top: 2 })
                  Text(row.message || '')
                    .fontSize(AppTheme.fontCaption)
                    .fontColor(this.tText)
                    .margin({ top: 6 })
                    .maxLines(6)
                    .textOverflow({ overflow: TextOverflow.Ellipsis })
                }
                .width('100%')
                .alignItems(HorizontalAlign.Start)
                .padding(AppTheme.spaceMd)
                .backgroundColor(this.tCard)
                .borderRadius(AppTheme.rSm)
                .border({ width: AppTheme.hairline, color: this.tBorderSoft })
              }
              .padding({ left: 12, right: 12, bottom: 8 })
            }, (row: ActivityLogEntry, i: number) => `ta-${i}-${row.timestamp ?? 0}-${row.message ?? ''}`)
          }
          .layoutWeight(1)
          .scrollBar(BarState.Off)
        }
      }
    }
    .width('100%')
    .layoutWeight(1)
  }

  @Builder
  playbooksPane() {
    Column() {
      if (this.playbooks.length === 0 && this.executions.length === 0) {
        EmptyHint({ text: '暂无剧本' })
      } else {
        List() {
          ForEach(this.playbooks, (pb: Playbook) => {
            ListItem() {
              Row() {
                Column() {
                  Text(pb.name || pb.id).fontSize(AppTheme.fontBody).fontColor(this.tText).fontWeight(FontWeight.Medium)
                  Text(pb.description || '').fontSize(AppTheme.fontCaption).fontColor(this.tMuted).margin({ top: 4 }).maxLines(2)
                }
                .alignItems(HorizontalAlign.Start)
                .layoutWeight(1)
                Text('执行')
                  .fontSize(AppTheme.fontCaption)
                  .fontColor(this.tOnAccent)
                  .padding({ left: 12, right: 12, top: 6, bottom: 6 })
                  .backgroundColor(this.tAccent)
                  .borderRadius(AppTheme.rSm)
                  .onClick(() => {
                    this.playbookTarget = pb.id;
                    this.playbookName = pb.name || pb.id;
                    this.playbookConfirm = true;
                  })
              }
              .width('100%')
              .padding(AppTheme.spaceMd)
              .backgroundColor(this.tCard)
              .borderRadius(AppTheme.rSm)
            }
            .padding({ left: 12, right: 12, bottom: 8 })
          }, (pb: Playbook) => pb.id)

          ListItem() {
            Text('最近执行').fontSize(AppTheme.fontSmall).fontColor(this.tMuted).padding({ left: 4, top: 8, bottom: 4 })
          }
          ForEach(this.executions.slice(0, 20), (ex: PlaybookExecution) => {
            ListItem() {
              Text(`#${ex.id} ${ex.playbook_name ?? ''} · ${ex.status ?? ''} · ${FormatUtil.relTime(ex.start_time ?? ex.started_at ?? 0)}`)
                .fontSize(AppTheme.fontCaption)
                .fontColor(this.tText)
                .padding(AppTheme.spaceMd)
                .width('100%')
                .backgroundColor(this.tElevated)
                .borderRadius(AppTheme.rXs)
                .onClick(() => this.onExecution(ex.id))
            }
            .padding({ left: 12, right: 12, bottom: 6 })
          }, (ex: PlaybookExecution) => `ex-${ex.id}`)
        }
        .layoutWeight(1)
        .scrollBar(BarState.Off)
      }
    }
    .width('100%')
    .layoutWeight(1)
  }

  @Builder
  forwardsPane() {
    Column() {
      Row({ space: 12 }) {
        Text('+ 端口转发')
          .fontSize(AppTheme.fontCaption)
          .fontColor(this.tAccent)
          .padding({ left: 10, right: 10, top: 6, bottom: 6 })
          .backgroundColor(this.tAccentSoft)
          .borderRadius(AppTheme.rXs)
          .onClick(() => {
            void this.openCreateForward();
          })
        Text('+ HTTP 代理')
          .fontSize(AppTheme.fontCaption)
          .fontColor(this.tAccent)
          .padding({ left: 10, right: 10, top: 6, bottom: 6 })
          .backgroundColor(this.tAccentSoft)
          .borderRadius(AppTheme.rXs)
          .onClick(() => {
            void this.openCreateProxy();
          })
      }
      .width('100%')
      .padding({ left: 12, right: 12, bottom: 8 })

      if (this.forwards.length === 0 && this.proxies.length === 0) {
        EmptyHint({ text: '暂无转发规则，点击上方新建' })
      } else {
        List() {
          ForEach(this.forwards, (f: PortForwardRule) => {
            ListItem() {
              Column() {
                Row() {
                  Column() {
                    Text(f.hostname || f.name || f.host_id || f.id)
                      .fontSize(AppTheme.fontBody)
                      .fontColor(this.tText)
                    Text(`${f.host_id ?? ''}  ${f.local_port ?? 0}->${this.forwardTargetPort(f)} · ${f.protocol || 'tcp'}`)
                      .fontSize(AppTheme.fontMicro)
                      .fontColor(this.tMuted)
                      .margin({ top: 4 })
                    if ((f.sessions ?? 0) > 0) {
                      Text(`会话 ${f.sessions}`)
                        .fontSize(AppTheme.fontMicro)
                        .fontColor(this.tSuccess)
                        .margin({ top: 2 })
                    }
                  }
                  .alignItems(HorizontalAlign.Start)
                  .layoutWeight(1)
                  Text(f.enabled ? '已启用' : '已停用')
                    .fontSize(AppTheme.fontMicro)
                    .fontColor(f.enabled ? this.tSuccess : this.tMuted)
                }
                .width('100%')
                Row({ space: 14 }) {
                  Text(f.enabled ? '停用' : '启用')
                    .fontSize(AppTheme.fontCaption)
                    .fontColor(f.enabled ? this.tWarning : this.tSuccess)
                    .onClick(() => {
                      void this.toggleForward(f);
                    })
                  Text('编辑')
                    .fontSize(AppTheme.fontCaption)
                    .fontColor(this.tAccent)
                    .onClick(() => {
                      void this.openEditForward(f);
                    })
                  Text('删除')
                    .fontSize(AppTheme.fontCaption)
                    .fontColor(this.tDanger)
                    .onClick(() => {
                      this.deleteForwardId = f.id;
                      this.showDeleteForward = true;
                    })
                }
                .margin({ top: 8 })
              }
              .width('100%')
              .padding(AppTheme.spaceMd)
              .backgroundColor(this.tCard)
              .borderRadius(AppTheme.rSm)
            }
            .padding({ left: 12, right: 12, bottom: 8 })
          }, (f: PortForwardRule) => f.id)

          ForEach(this.proxies, (p: HTTPProxyConfig) => {
            ListItem() {
              Column() {
                Row() {
                  Column() {
                    Text(`HTTP 代理 · ${p.name || p.hostname || p.id}`)
                      .fontSize(AppTheme.fontBody)
                      .fontColor(this.tText)
                    Text(`:${this.proxyTargetPort(p)} · ${p.host_id ?? ''}`)
                      .fontSize(AppTheme.fontMicro)
                      .fontColor(this.tMuted)
                      .margin({ top: 4 })
                    if (p.default_path) {
                      Text(`路径 ${p.default_path}`)
                        .fontSize(AppTheme.fontMicro)
                        .fontColor(this.tMuted)
                        .margin({ top: 2 })
                    }
                  }
                  .alignItems(HorizontalAlign.Start)
                  .layoutWeight(1)
                  Text(p.enabled ? '已启用' : '已停用')
                    .fontSize(AppTheme.fontMicro)
                    .fontColor(p.enabled ? this.tSuccess : this.tMuted)
                }
                .width('100%')
                Row({ space: 14 }) {
                  Text(p.enabled ? '停用' : '启用')
                    .fontSize(AppTheme.fontCaption)
                    .fontColor(p.enabled ? this.tWarning : this.tSuccess)
                    .onClick(() => {
                      void this.toggleProxy(p);
                    })
                  Text('编辑')
                    .fontSize(AppTheme.fontCaption)
                    .fontColor(this.tAccent)
                    .onClick(() => {
                      void this.openEditProxy(p);
                    })
                  Text('地址')
                    .fontSize(AppTheme.fontCaption)
                    .fontColor(this.tAccent)
                    .onClick(() => {
                      void this.copyProxyUrl(p);
                    })
                  Text('删除')
                    .fontSize(AppTheme.fontCaption)
                    .fontColor(this.tDanger)
                    .onClick(() => {
                      this.deleteProxyId = p.id;
                      this.showDeleteProxy = true;
                    })
                }
                .margin({ top: 8 })
              }
              .width('100%')
              .padding(AppTheme.spaceMd)
              .backgroundColor(this.tCard)
              .borderRadius(AppTheme.rSm)
            }
            .padding({ left: 12, right: 12, bottom: 8 })
          }, (p: HTTPProxyConfig) => p.id)
        }
        .layoutWeight(1)
        .scrollBar(BarState.Off)
      }
    }
    .width('100%')
    .layoutWeight(1)
  }

  @Builder
  govPane() {
    Column() {
      FilterChips({
        labels: this.govLabels,
        selected: $govSub,
        onSelect: (_i: number) => {
          void this.refresh();
        }
      })
        .padding({ left: 12, right: 12, bottom: 8 })

      if (this.govSub === 0) {
        if (this.remediations.length === 0) {
          EmptyHint({ text: '暂无待审批项' })
        } else {
          List() {
            ForEach(this.remediations, (r: RemediationRun) => {
              ListItem() {
                Column() {
                  Text(r.title || `修复 #${r.id}`).fontSize(AppTheme.fontBody).fontColor(this.tText)
                  Text(`${r.status ?? ''} · ${r.playbook_id ?? ''} · ${FormatUtil.relTime(r.created_at ?? 0)}`)
                    .fontSize(AppTheme.fontMicro).fontColor(this.tMuted).margin({ top: 4 })
                  Row({ space: 16 }) {
                    Text('批准').fontSize(AppTheme.fontCaption).fontColor(this.tSuccess)
                      .onClick(() => {
                        void this.approveRem(r);
                      })
                    Text('拒绝').fontSize(AppTheme.fontCaption).fontColor(this.tDanger)
                      .onClick(() => {
                        void this.rejectRem(r);
                      })
                  }
                  .margin({ top: 8 })
                }
                .width('100%')
                .alignItems(HorizontalAlign.Start)
                .padding(AppTheme.spaceMd)
                .backgroundColor(this.tCard)
                .borderRadius(AppTheme.rSm)
              }
              .padding({ left: 12, right: 12, bottom: 8 })
            }, (r: RemediationRun) => `rem-${r.id}`)
          }
          .layoutWeight(1)
        }
      } else if (this.govSub === 1) {
        if (this.slos.length === 0) {
          EmptyHint({ text: '暂无 SLO 配置' })
        } else {
          List() {
            ForEach(this.slos, (s: SloStatus, i: number) => {
              ListItem() {
                Column() {
                  Row() {
                    Text(s.name || s.id || `SLO ${i + 1}`).fontSize(AppTheme.fontBody).fontColor(this.tText).layoutWeight(1)
                    Text(s.status || '—')
                      .fontSize(AppTheme.fontMicro)
                      .fontColor(this.tWarning)
                      .padding({ left: 8, right: 8, top: 2, bottom: 2 })
                      .backgroundColor(this.tElevated)
                      .borderRadius(AppTheme.rChip)
                  }
                  .width('100%')
                  Text(`SLI ${(s.sli ?? 0).toFixed(2)} · 目标 ${(s.target ?? 0).toFixed(2)}`)
                    .fontSize(AppTheme.fontCaption).fontColor(this.tMuted).margin({ top: 6 })
                }
                .width('100%')
                .alignItems(HorizontalAlign.Start)
                .padding(AppTheme.spaceMd)
                .backgroundColor(this.tCard)
                .borderRadius(AppTheme.rSm)
              }
              .padding({ left: 12, right: 12, bottom: 8 })
            }, (s: SloStatus, i: number) => `slo-${s.id ?? i}`)
          }
          .layoutWeight(1)
        }
      } else {
        Column() {
          TextInput({ text: this.ticketComment, placeholder: '工单备注（选中工单后提交）' })
            .height(40)
            .margin({ left: 12, right: 12, bottom: 8 })
            .backgroundColor(this.tElevated)
            .fontColor(this.tText)
            .onChange((v: string) => {
              this.ticketComment = v;
            })
          if (this.tickets.length === 0) {
            EmptyHint({ text: '暂无开放工单' })
          } else {
            List() {
              ForEach(this.tickets, (t: Ticket) => {
                ListItem() {
                  Column() {
                    Text(`#${t.id} ${t.title ?? ''}`).fontSize(AppTheme.fontBody).fontColor(this.tText)
                    Text(`${FormatUtil.ticketStatusZh(t.status || '')} · ${t.priority ?? ''} · ${t.assignee || '未指派'}`)
                      .fontSize(AppTheme.fontMicro).fontColor(this.tMuted).margin({ top: 4 })
                    Row({ space: 12 }) {
                      Text(t.assignee ? `指派: ${t.assignee}` : '指派')
                        .fontSize(AppTheme.fontCaption)
                        .fontColor(this.tAccent)
                        .onClick(() => {
                          this.assignTicketId = t.id;
                          this.assignVisible = true;
                        })
                      Text('备注').fontSize(AppTheme.fontCaption).fontColor(this.tAccent)
                        .onClick(() => {
                          void this.commentOnTicket(t);
                        })
                      Text('解决').fontSize(AppTheme.fontCaption).fontColor(this.tSuccess)
                        .onClick(() => {
                          void this.setTicketStatus(t, 'resolved');
                        })
                      Text('关闭').fontSize(AppTheme.fontCaption).fontColor(this.tMuted)
                        .onClick(() => {
                          void this.setTicketStatus(t, 'closed');
                        })
                    }
                    .margin({ top: 8 })
                  }
                  .width('100%')
                  .alignItems(HorizontalAlign.Start)
                  .padding(AppTheme.spaceMd)
                  .backgroundColor(this.tCard)
                  .borderRadius(AppTheme.rSm)
                }
                .padding({ left: 12, right: 12, bottom: 8 })
              }, (t: Ticket) => `tk-${t.id}`)
            }
            .layoutWeight(1)
          }
        }
        .width('100%')
        .layoutWeight(1)
      }
    }
    .width('100%')
    .layoutWeight(1)
  }
}

@Entry
@Component
struct OperationsPage {
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
      OperationsTab()
    }
    .width('100%')
    .height('100%')
  }
}
