import { Api } from '../data/Api';
import { ApiClient } from '../data/ApiClient';
import { AppState } from '../common/AppState';
import { FormatUtil } from '../common/FormatUtil';
import {
  Alert,
  Host,
  Incident,
  MeResponse,
  Summary,
  SreOverview,
  WeatherResponse
} from '../data/models/Models';
import {
  ConfirmDialogHost,
  ErrorBanner,
  PageHeader
} from '../components/CommonUI';
import {
  CompactHostRow,
  WeatherHero,
  dateZhToday,
  greetingNow
} from '../components/InteractionUI';
import { QuickTile, DrawIcon, pastelOf, AppSymbol } from '../components/AppIcons';
import { SessionTicker } from '../data/SessionTicker';
import { NavUtil } from '../common/NavUtil';
import { SessionStore } from '../data/SessionStore';
import { AppTheme } from '../common/Routes';


interface QuickMod {
  key: string;
  title: string;
  /** Bottom nav index: 1 monitor / 2 alerts / 3 ops / 4 ai；-1 走 route */
  bottom: number;
  /** InfraHub 子 Tab 0–3，或 Operations 子 Tab */
  sub: number;
  /** 资源区段：0 硬件 / 1 虚拟机（对齐 Android infraTab res） */
  res: number;
  route: string;
  iconKind: string;
  tint: string;
}

/** 目录与默认序对齐 Android DashboardScreen ALL_MODULES / DEFAULT_MODULE_KEYS */
const ALL_MODULES: QuickMod[] = [
  { key: 'hardware', title: '硬件监控', bottom: 1, sub: 1, res: 0, route: '', iconKind: 'hardware', tint: '#4F7FFF' },
  { key: 'hyperv', title: '虚拟机', bottom: 1, sub: 1, res: 1, route: '', iconKind: 'hyperv', tint: '#8B5CF6' },
  { key: 'dashboards', title: '仪表盘', bottom: -1, sub: 0, res: 0, route: 'pages/DashboardsPage', iconKind: 'dashboards', tint: '#06B6D4' },
  { key: 'netflow', title: '网络流量', bottom: 1, sub: 2, res: 0, route: '', iconKind: 'netflow', tint: '#00A86B' },
  { key: 'probe', title: '拨测监控', bottom: 1, sub: 3, res: 0, route: '', iconKind: 'probe', tint: '#06B6D4' },
  { key: 'hosts', title: '主机监控', bottom: 1, sub: 0, res: 0, route: '', iconKind: 'hosts', tint: '#0EA5E9' },
  { key: 'logs', title: '日志检索', bottom: 3, sub: 1, res: 0, route: '', iconKind: 'logs', tint: '#EF4444' },
  { key: 'incidents', title: 'SRE 事件', bottom: 3, sub: 0, res: 0, route: '', iconKind: 'dashboards', tint: '#8B5CF6' },
  { key: 'playbooks', title: '运维编排', bottom: 3, sub: 2, res: 0, route: '', iconKind: 'build', tint: '#06B6D4' },
  { key: 'forwards', title: '端口转发', bottom: 3, sub: 3, res: 0, route: '', iconKind: 'netflow', tint: '#4F7FFF' },
  { key: 'governance', title: '治理审计', bottom: 3, sub: 4, res: 0, route: '', iconKind: 'settings', tint: '#F59E0B' },
  { key: 'alerts', title: '告警中心', bottom: 2, sub: 0, res: 0, route: '', iconKind: 'warning', tint: '#F59E0B' },
  { key: 'ai', title: 'AI 助手', bottom: 4, sub: 0, res: 0, route: '', iconKind: 'ai', tint: '#4F7FFF' }
];

const DEFAULT_KEYS: string[] = [
  'hardware', 'hyperv', 'netflow', 'probe', 'hosts', 'logs', 'playbooks', 'ai'
];

const MAX_MODULES: number = 8;

@Component
export struct DashboardTab {
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
  @StorageLink('aiops_theme_dark') darkTheme: boolean = true;

  onOpenSettings: () => void = () => {};
  onOpenHost: (id: string) => void = (_id: string) => {};
  onOpenAlerts: (level: string) => void = (_level: string) => {};
  /** bottom / infraOrOpsSub / resourcesRes — 对齐 Android Routes.infraTab / operationsTab */
  onSwitchTab: (bottom: number, sub: number, res: number) => void = (_b: number, _s: number, _r: number) => {};

  @State summary: Summary = {};
  @State hosts: Host[] = [];
  @State weather: WeatherResponse = {};
  @State sre: SreOverview = {};
  @State incidents: Incident[] = [];
  @State alerts: Alert[] = [];
  @State me: MeResponse = {};
  @State error: string = '';
  @State loading: boolean = false;
  @State showMenu: boolean = false;
  @State showLogoutConfirm: boolean = false;
  @State showSwitchConfirm: boolean = false;
  @State showEditModules: boolean = false;
  @State moduleKeys: string[] = DEFAULT_KEYS.slice();
  @State editSelected: string[] = [];
  @State accountBusy: boolean = false;
  private pollId: number = -1;
  private inFlight: boolean = false;
  private reqId: number = 0;

  aboutToAppear(): void {
    void this.loadModules();
    void this.refresh();
    this.pollId = SessionTicker.start(10000, () => {
      void this.refresh();
    });
  }

  aboutToDisappear(): void {
    SessionTicker.stop(this.pollId);
    this.pollId = -1;
  }

  private migrateModuleKey(k: string): string {
    if (k === 'forward') {
      return 'forwards';
    }
    if (k === 'gov') {
      return 'governance';
    }
    return k;
  }

  private async loadModules(): Promise<void> {
    const saved = await SessionStore.getQuickModules();
    if (saved.length > 0) {
      const migrated: string[] = [];
      for (let i = 0; i < saved.length; i++) {
        const k = this.migrateModuleKey(saved[i]);
        if (migrated.indexOf(k) < 0) {
          migrated.push(k);
        }
      }
      this.moduleKeys = migrated;
      if (migrated.join(',') !== saved.join(',')) {
        await SessionStore.saveQuickModules(migrated);
      }
    }
  }

  async refresh(): Promise<void> {
    if (this.inFlight || !ApiClient.isSessionAlive()) {
      return;
    }
    this.inFlight = true;
    const myId = ++this.reqId;
    this.loading = true;
    this.error = '';
    try {
      const summary = await Api.summary();
      const hosts = await Api.hosts();
      let weather: WeatherResponse = {};
      let sre: SreOverview = {};
      let incidents: Incident[] = [];
      let alerts: Alert[] = [];
      let me: MeResponse = AppState.me ?? {};
      try {
        weather = await Api.weather();
      } catch (e) {
        weather = {};
      }
      try {
        sre = await Api.sreOverview();
      } catch (e) {
        sre = {};
      }
      try {
        incidents = await Api.incidents();
      } catch (e) {
        incidents = [];
      }
      try {
        alerts = await Api.alerts();
      } catch (e) {
        alerts = [];
      }
      try {
        me = await Api.me();
        AppState.me = me;
      } catch (e) {
        // best-effort
      }
      if (myId !== this.reqId) {
        return;
      }
      this.summary = summary;
      this.hosts = hosts;
      this.weather = weather;
      this.sre = sre;
      this.incidents = incidents
        .filter((x: Incident) => x.status !== 'resolved' && x.status !== 'closed')
        .slice()
        .sort((a: Incident, b: Incident) => (b.created_at ?? 0) - (a.created_at ?? 0));
      this.alerts = alerts
        .filter((a: Alert) =>
          a.status !== 'acknowledged' && a.status !== 'silenced' && a.status !== 'resolved')
        .slice()
        .sort((a: Alert, b: Alert) => {
          const ac = (a.level || '').toLowerCase() === 'critical' ? 0 : 1;
          const bc = (b.level || '').toLowerCase() === 'critical' ? 0 : 1;
          return ac - bc;
        });
      this.me = me;
    } catch (e) {
      if (myId === this.reqId) {
        this.error = ApiClient.formatError(e as Object);
      }
    } finally {
      if (myId === this.reqId) {
        this.loading = false;
        this.inFlight = false;
      }
    }
  }

  private openModule(m: QuickMod): void {
    if (m.route) {
      NavUtil.push(m.route);
      return;
    }
    if (m.bottom >= 0) {
      this.onSwitchTab(m.bottom, m.sub, m.res);
    }
  }

  private visibleModules(): QuickMod[] {
    const out: QuickMod[] = [];
    for (let i = 0; i < this.moduleKeys.length && out.length < MAX_MODULES; i++) {
      const k = this.moduleKeys[i];
      for (let j = 0; j < ALL_MODULES.length; j++) {
        if (ALL_MODULES[j].key === k) {
          out.push(ALL_MODULES[j]);
          break;
        }
      }
    }
    return out;
  }

  private worstLoad(h: Host): number {
    return Math.max(
      h.latest?.cpu_percent ?? 0,
      h.latest?.mem_percent ?? 0,
      h.latest?.disk_percent ?? 0,
      h.latest?.gpu_percent ?? 0
    );
  }

  private worstMetricLabel(h: Host): string {
    const cpu = h.latest?.cpu_percent ?? 0;
    const mem = h.latest?.mem_percent ?? 0;
    const disk = h.latest?.disk_percent ?? 0;
    const gpu = h.latest?.gpu_percent ?? 0;
    const maxv = Math.max(cpu, mem, disk, gpu);
    if (maxv === mem) {
      return '内存';
    }
    if (maxv === disk) {
      return '磁盘';
    }
    if (maxv === gpu && gpu > 0) {
      return 'GPU';
    }
    return 'CPU';
  }

  private offlineHosts(): Host[] {
    return this.hosts.filter((h: Host) => !h.online);
  }

  private hotHosts(): Host[] {
    return this.hosts
      .filter((h: Host) => !!h.online && !!h.latest)
      .filter((h: Host) => this.worstLoad(h) >= 1)
      .slice()
      .sort((a: Host, b: Host) => this.worstLoad(b) - this.worstLoad(a))
      .slice(0, 4);
  }

  private hotspotTrailing(h: Host): string {
    if (!h.online) {
      return '离线';
    }
    const v = this.worstLoad(h);
    return `${this.worstMetricLabel(h)} ${Math.round(v)}%`;
  }

  private weatherLocLine(): string {
    const w = this.weather;
    if (w.ok === false) {
      return '';
    }
    const loc = w.location || w.city || '';
    const text = w.text || '';
    if (!loc && !text) {
      return '';
    }
    return `${loc}${loc && text ? ' · ' : ''}${text}`;
  }

  private weatherTemp(): string {
    const w = this.weather;
    if (w.ok === false) {
      return '';
    }
    const temp = w.temp_c ?? w.temp;
    return temp !== undefined && temp !== null ? `${temp}°` : '';
  }

  private openIncidentCount(): number {
    return this.sre.open_incidents ?? this.incidents.length;
  }

  private alertCountLabel(): string {
    let crit = 0;
    let warn = 0;
    for (let i = 0; i < this.alerts.length; i++) {
      const lv = (this.alerts[i].level || '').toLowerCase();
      if (lv === 'critical' || lv === '严重') {
        crit++;
      } else {
        warn++;
      }
    }
    return `严重 ${crit} · 警告 ${warn}`;
  }

  private alertCountColor(): string {
    for (let i = 0; i < this.alerts.length; i++) {
      const lv = (this.alerts[i].level || '').toLowerCase();
      if (lv === 'critical' || lv === '严重') {
        return this.tDanger;
      }
    }
    return this.tWarning;
  }

  private softPillBg(hex: string): string {
    const h = (hex || '').replace('#', '');
    if (h.length === 6) {
      return `#24${h}`;
    }
    return this.tElevated;
  }

  private mfaLineColor(): string {
    return this.me.mfa_enabled ? this.tSuccess : this.tWarning;
  }

  private async saveModules(): Promise<void> {
    this.moduleKeys = this.editSelected.slice(0, MAX_MODULES);
    await SessionStore.saveQuickModules(this.moduleKeys);
    this.showEditModules = false;
  }

  private async doLogout(): Promise<void> {
    this.showLogoutConfirm = false;
    this.showSwitchConfirm = false;
    this.accountBusy = true;
    try {
      await AppState.logout();
    } finally {
      this.accountBusy = false;
    }
  }

  build() {
    Stack() {
      Column() {
        PageHeader({
          title: 'SRE 驾驶舱',
          showLoading: this.loading,
          trailing2Icon: 'refresh',
          onTrailing2: () => {
            void this.refresh();
          },
          trailingIcon: 'settings',
          onTrailing: () => {
            this.showMenu = true;
          }
        })

        if (this.error && this.hosts.length === 0 && !(this.summary.total_hosts !== undefined)) {
          ErrorBanner({
            message: this.error || '同步错误',
            showRetry: true,
            onRetry: () => {
              void this.refresh();
            }
          })
        } else if (this.loading && this.hosts.length === 0 && !(this.summary.total_hosts !== undefined)) {
          Column() {
            LoadingProgress().width(36).height(36).color(this.tAccent)
            Text('加载中…').fontSize(AppTheme.fontSmall).fontColor(this.tMuted).margin({ top: 10 })
          }
          .width('100%')
          .layoutWeight(1)
          .justifyContent(FlexAlign.Center)
          .alignItems(HorizontalAlign.Center)
        } else {
          Scroll() {
            Column({ space: 8 }) {
              WeatherHero({
                greeting: greetingNow(),
                dateText: dateZhToday(),
                weatherLine: this.weatherLocLine(),
                tempText: this.weatherTemp() || '—',
                onlineText: `${this.summary.online_hosts ?? 0}/${this.summary.total_hosts ?? 0}`,
                criticalText: `${this.summary.critical_alerts ?? 0}`,
                warningText: `${this.summary.warning_alerts ?? 0}`,
                onOnline: () => this.onSwitchTab(1, 0, 0),
                onCritical: () => this.onOpenAlerts('critical'),
                onWarning: () => this.onOpenAlerts('warning')
              })

              SectionQuick({
                modules: this.visibleModules(),
                onOpen: (m: QuickMod) => this.openModule(m),
                onEdit: () => {
                  this.editSelected = this.moduleKeys.slice();
                  this.showEditModules = true;
                }
              })

              // 待处理事件
              Column({ space: 6 }) {
                Row() {
                  Text('待处理事件')
                    .fontSize(AppTheme.fontSmall)
                    .fontWeight(FontWeight.Bold)
                    .fontColor(this.tText)
                  if (this.openIncidentCount() > 0) {
                    Text(`${this.openIncidentCount()}`)
                      .fontSize(AppTheme.fontTiny)
                      .fontWeight(FontWeight.Bold)
                      .fontColor(this.tDanger)
                      .padding({ left: 6, right: 6, top: 1, bottom: 1 })
                      .backgroundColor(this.softPillBg(this.tDanger))
                      .borderRadius(AppTheme.rChip)
                      .margin({ left: 6 })
                  }
                  Blank()
                  if (this.incidents.length > 0) {
                    Text('全部 ›')
                      .fontSize(AppTheme.fontMicro)
                      .fontColor(this.tAccent)
                      .onClick(() => this.onSwitchTab(3, 0, 0))
                  }
                }
                .width('100%')

                if (this.incidents.length === 0) {
                  Row({ space: 6 }) {
                    AppSymbol({ kind: 'spark', tint: this.tSuccess, iconSize: 14 })
                    Text('暂无待处理事件').fontSize(AppTheme.fontMicro).fontColor(this.tMuted)
                  }
                  .width('100%')
                  .padding({ top: 2, bottom: 2 })
                } else {
                  ForEach(this.incidents.slice(0, 3), (inc: Incident) => {
                    Row() {
                      Circle({ width: 7, height: 7 })
                        .fill(FormatUtil.sevColor(inc.severity || 'info'))
                        .margin({ right: 8 })
                      Column() {
                        Text(inc.title || `事件 #${inc.id}`)
                          .fontSize(AppTheme.fontCaption).fontWeight(FontWeight.Medium).fontColor(this.tText).maxLines(1)
                          .textOverflow({ overflow: TextOverflow.Ellipsis })
                        Text(`${FormatUtil.levelLabel(inc.severity || '')} · ${FormatUtil.incidentStatusZh(inc.status || '')}`)
                          .fontSize(AppTheme.fontTiny).fontColor(this.tMuted).margin({ top: 2 })
                      }
                      .alignItems(HorizontalAlign.Start)
                      .layoutWeight(1)
                      Text(FormatUtil.relTime(inc.created_at ?? 0))
                        .fontSize(AppTheme.fontTiny).fontColor(this.tMuted)
                    }
                    .width('100%')
                    .padding({ top: 4, bottom: 4 })
                    .onClick(() => this.onSwitchTab(3, 0, 0))
                  }, (inc: Incident) => `${inc.id}`)
                }
              }
              .width('100%')
              .padding(AppTheme.spaceMd)
              .backgroundColor(this.tCard)
              .borderRadius(AppTheme.rMd)
              .border({ width: AppTheme.hairline, color: this.tBorderSoft })
              .shadow({ radius: 4, color: this.tShadow, offsetY: 1 })

              // 活跃告警
              Column({ space: 6 }) {
                Row() {
                  Text('活跃告警')
                    .fontSize(AppTheme.fontSmall)
                    .fontWeight(FontWeight.Bold)
                    .fontColor(this.tText)
                  if (this.alerts.length > 0) {
                    Text(this.alertCountLabel())
                      .fontSize(AppTheme.fontTiny)
                      .fontWeight(FontWeight.Bold)
                      .fontColor(this.alertCountColor())
                      .padding({ left: 6, right: 6, top: 1, bottom: 1 })
                      .backgroundColor(this.softPillBg(this.alertCountColor()))
                      .borderRadius(AppTheme.rChip)
                      .margin({ left: 6 })
                  }
                  Blank()
                  if (this.alerts.length > 0) {
                    Text('全部 ›')
                      .fontSize(AppTheme.fontMicro)
                      .fontColor(this.tAccent)
                      .onClick(() => this.onOpenAlerts('all'))
                  }
                }
                .width('100%')

                if (this.alerts.length === 0) {
                  Row({ space: 6 }) {
                    AppSymbol({ kind: 'spark', tint: this.tSuccess, iconSize: 14 })
                    Text('无活跃告警').fontSize(AppTheme.fontMicro).fontColor(this.tMuted)
                  }
                  .width('100%')
                  .padding({ top: 2, bottom: 2 })
                } else {
                  ForEach(this.alerts.slice(0, 3), (a: Alert) => {
                    Row() {
                      Text(FormatUtil.levelLabel(a.level))
                        .fontSize(AppTheme.fontTiny)
                        .fontWeight(FontWeight.Bold)
                        .fontColor(FormatUtil.sevColor(a.level))
                        .padding({ left: 8, right: 8, top: 3, bottom: 3 })
                        .backgroundColor(FormatUtil.sevSoft(a.level))
                        .borderRadius(AppTheme.rChip)
                        .margin({ right: 8 })
                      Column() {
                        Text(`${a.hostname || a.host_id} · ${(a.type || '').toUpperCase()}`)
                          .fontSize(AppTheme.fontCaption).fontWeight(FontWeight.Medium).fontColor(this.tText).maxLines(1)
                          .textOverflow({ overflow: TextOverflow.Ellipsis })
                        Text(a.message || a.type)
                          .fontSize(AppTheme.fontTiny).fontColor(this.tMuted).margin({ top: 2 }).maxLines(1)
                          .textOverflow({ overflow: TextOverflow.Ellipsis })
                      }
                      .alignItems(HorizontalAlign.Start)
                      .layoutWeight(1)
                    }
                    .width('100%')
                    .padding({ top: 4, bottom: 4 })
                    .onClick(() => this.onOpenAlerts(a.level))
                  }, (a: Alert, i: number) => `${a.host_id}-${a.type}-${i}`)
                }
              }
              .width('100%')
              .padding(AppTheme.spaceMd)
              .backgroundColor(this.tCard)
              .borderRadius(AppTheme.rMd)
              .border({ width: AppTheme.hairline, color: this.tBorderSoft })
              .shadow({ radius: 4, color: this.tShadow, offsetY: 1 })

              // 容量热点
              Column({ space: 6 }) {
                Row() {
                  Text('容量热点')
                    .fontSize(AppTheme.fontSmall)
                    .fontWeight(FontWeight.Bold)
                    .fontColor(this.tText)
                  if (this.offlineHosts().length > 0) {
                    Text(`离线 ${this.offlineHosts().length}`)
                      .fontSize(AppTheme.fontTiny)
                      .fontWeight(FontWeight.Bold)
                      .fontColor(this.tDanger)
                      .padding({ left: 6, right: 6, top: 1, bottom: 1 })
                      .backgroundColor(this.softPillBg(this.tDanger))
                      .borderRadius(AppTheme.rChip)
                      .margin({ left: 6 })
                  }
                  Blank()
                  Text('主机 ›')
                    .fontSize(AppTheme.fontMicro)
                    .fontColor(this.tAccent)
                    .onClick(() => this.onSwitchTab(1, 0, 0))
                }
                .width('100%')

                if (this.offlineHosts().length === 0 && this.hotHosts().length === 0) {
                  Row({ space: 6 }) {
                    AppSymbol({ kind: 'spark', tint: this.tSuccess, iconSize: 14 })
                    Text('全部主机负载正常').fontSize(AppTheme.fontMicro).fontColor(this.tMuted)
                  }
                  .width('100%')
                  .padding({ top: 2, bottom: 2 })
                } else {
                  ForEach(this.offlineHosts().slice(0, 2), (h: Host) => {
                    CompactHostRow({
                      title: h.hostname || h.id,
                      subtitle: h.ip || h.category || '',
                      trailing: '离线',
                      danger: true,
                      onTap: () => this.onOpenHost(h.id)
                    })
                  }, (h: Host) => `off-${h.id}`)
                  ForEach(this.hotHosts(), (h: Host) => {
                    CompactHostRow({
                      title: h.hostname || h.id,
                      subtitle: h.ip || h.category || '',
                      trailing: this.hotspotTrailing(h),
                      danger: this.worstLoad(h) >= 85,
                      onTap: () => this.onOpenHost(h.id)
                    })
                  }, (h: Host) => `hot-${h.id}`)
                }
              }
              .width('100%')
              .padding(AppTheme.spaceMd)
              .backgroundColor(this.tCard)
              .borderRadius(AppTheme.rMd)
              .border({ width: AppTheme.hairline, color: this.tBorderSoft })
              .shadow({ radius: 4, color: this.tShadow, offsetY: 1 })

              if (this.error) {
                Text(this.error || '部分数据刷新失败')
                  .fontSize(AppTheme.fontCaption)
                  .fontColor(this.tWarning)
                  .width('100%')
                  .padding(AppTheme.spaceMd)
                  .backgroundColor(this.tWarningSoft)
                  .borderRadius(AppTheme.rSm)
              }

              Blank().height(12)
            }
            .width('100%')
            .padding({ left: AppTheme.cardPad, right: AppTheme.cardPad, top: AppTheme.spaceSm, bottom: 20 })
          }
          .layoutWeight(1)
          .scrollBar(BarState.Off)
        }
      }
      .width('100%')
      .height('100%')
      .backgroundColor(this.tBg)

      // 账号菜单（对齐 Android DropdownMenu）
      if (this.showMenu) {
        Column() {
          Blank()
            .layoutWeight(1)
            .onClick(() => {
              this.showMenu = false;
            })
          Column() {
            Column() {
              Text(this.me.username || '当前账号')
                .fontSize(AppTheme.fontSmall)
                .fontWeight(FontWeight.Bold)
                .fontColor(this.tText)
                .width('100%')
              Text(`${this.me.role || '用户'} · ${this.me.mfa_enabled ? 'MFA 已启用' : 'MFA 未启用'}`)
                .fontSize(AppTheme.fontMicro)
                .fontColor(this.mfaLineColor())
                .margin({ top: 4 })
                .width('100%')
            }
            .width('100%')
            .padding({ left: 14, right: 14, top: 10, bottom: 10 })

            Divider().color(this.tBorderSoft).strokeWidth(0.5)

            this.menuRow('settings', '配置中心', this.tText, () => {
              this.showMenu = false;
              this.onOpenSettings();
            })
            this.menuRow('spark', '消息中心', this.tText, () => {
              this.showMenu = false;
              NavUtil.push('pages/MessagesPage');
            })
            this.menuRow('hosts', '重复主机清理', this.tText, () => {
              this.showMenu = false;
              NavUtil.push('pages/DuplicatesPage');
            })
            this.menuRow('build', '终端会话回放', this.tText, () => {
              this.showMenu = false;
              NavUtil.push('pages/TerminalSessionsPage');
            })

            Row() {
              AppSymbol({
                kind: this.darkTheme ? 'settings' : 'spark',
                tint: this.tMuted,
                iconSize: 18
              })
              Column() {
                Text(this.darkTheme ? '深色模式' : '浅色模式')
                  .fontSize(AppTheme.fontSmall)
                  .fontColor(this.tText)
                Text('点击切换应用外观')
                  .fontSize(AppTheme.fontMicro)
                  .fontColor(this.tMuted)
                  .margin({ top: 2 })
              }
              .alignItems(HorizontalAlign.Start)
              .layoutWeight(1)
              .margin({ left: 12 })
              Toggle({ type: ToggleType.Switch, isOn: !this.darkTheme })
                .selectedColor(this.tAccent)
                .enabled(false)
            }
            .width('100%')
            .padding({ left: 14, right: 14, top: 12, bottom: 12 })
            .onClick(() => {
              void AppState.toggleTheme();
            })

            this.menuRow('person', '切换账号', this.tText, () => {
              this.showMenu = false;
              this.showSwitchConfirm = true;
            })
            this.menuRow('lock', '退出登录', this.tDanger, () => {
              this.showMenu = false;
              this.showLogoutConfirm = true;
            })
          }
          .width('100%')
          .backgroundColor(this.tCard)
          .borderRadius({ topLeft: AppTheme.rMd, topRight: AppTheme.rMd })
          .padding({ bottom: 20 })
        }
        .width('100%')
        .height('100%')
        .backgroundColor(this.tScrim)
        .zIndex(99)
      }

      ConfirmDialogHost({
        visible: this.showLogoutConfirm,
        title: '退出登录？',
        message: '将同时注销服务器会话并清除本机登录凭据。下次登录仍会执行账号密码与 MFA 验证。',
        confirmText: '退出',
        danger: true,
        onConfirm: () => {
          void this.doLogout();
        },
        onCancel: () => {
          this.showLogoutConfirm = false;
        }
      })

      ConfirmDialogHost({
        visible: this.showSwitchConfirm,
        title: '切换账号？',
        message: '将注销当前会话并返回登录页。新账号必须重新验证密码；已启用 MFA 的账号还需输入 6 位动态口令。',
        confirmText: '切换',
        danger: false,
        onConfirm: () => {
          void this.doLogout();
        },
        onCancel: () => {
          this.showSwitchConfirm = false;
        }
      })

      if (this.showEditModules) {
        Column() {
          Blank().layoutWeight(1).onClick(() => {
            this.showEditModules = false;
          })
          Column({ space: 4 }) {
            Row() {
              Text('自定义快捷入口')
                .fontSize(AppTheme.fontLead)
                .fontWeight(FontWeight.Bold)
                .fontColor(this.tText)
                .layoutWeight(1)
              Text(`${this.editSelected.length}/${MAX_MODULES}`)
                .fontSize(AppTheme.fontCaption)
                .fontColor(this.tMuted)
            }
            .width('100%')
            .margin({ bottom: 4 })

            Text(`勾选要在总览显示的模块，最多 ${MAX_MODULES} 个`)
              .fontSize(AppTheme.fontMicro)
              .fontColor(this.tMuted)
              .width('100%')
              .margin({ bottom: 8 })

            Scroll() {
              Column() {
                ForEach(ALL_MODULES, (m: QuickMod) => {
                  Row({ space: 10 }) {
                    Column() {
                      DrawIcon({ kind: m.iconKind, tint: m.tint, iconSize: 16 })
                    }
                    .width(32)
                    .height(32)
                    .justifyContent(FlexAlign.Center)
                    .borderRadius(AppTheme.rXs)
                    .backgroundColor(pastelOf(m.tint))
                    .opacity(this.editSelected.indexOf(m.key) < 0 && this.editSelected.length >= MAX_MODULES ? 0.4 : 1)
                    Text(m.title)
                      .fontSize(AppTheme.fontSmall)
                      .fontColor(this.editSelected.indexOf(m.key) < 0 && this.editSelected.length >= MAX_MODULES
                        ? this.tMuted : this.tText)
                      .layoutWeight(1)
                    Text(this.editSelected.indexOf(m.key) >= 0 ? '✓' : '')
                      .fontSize(AppTheme.fontLead)
                      .fontWeight(FontWeight.Bold)
                      .fontColor(this.tAccent)
                      .width(24)
                      .textAlign(TextAlign.Center)
                  }
                  .width('100%')
                  .padding({ top: 6, bottom: 6, left: 6, right: 6 })
                  .onClick(() => {
                    const idx = this.editSelected.indexOf(m.key);
                    if (idx >= 0) {
                      this.editSelected = this.editSelected.filter((k: string) => k !== m.key);
                    } else if (this.editSelected.length < MAX_MODULES) {
                      this.editSelected = this.editSelected.concat([m.key]);
                    }
                  })
                }, (m: QuickMod) => m.key)
              }
              .width('100%')
            }
            .constraintSize({ maxHeight: '52%' })
            .scrollBar(BarState.Off)
            .width('100%')

            Button(`保存 (${this.editSelected.length})`)
              .width('100%')
              .height(44)
              .margin({ top: 8 })
              .backgroundColor(this.tAccent)
              .fontColor(this.tOnAccent)
              .borderRadius(AppTheme.rSm)
              .enabled(this.editSelected.length > 0)
              .onClick(() => {
                void this.saveModules();
              })
          }
          .width('100%')
          .padding({ left: 16, right: 16, top: 16, bottom: 24 })
          .backgroundColor(this.tCard)
          .borderRadius({ topLeft: AppTheme.rMd, topRight: AppTheme.rMd })
        }
        .width('100%')
        .height('100%')
        .backgroundColor(this.tScrim)
        .zIndex(98)
      }
    }
    .width('100%')
    .height('100%')
  }

  @Builder
  menuRow(icon: string, label: string, color: string, onTap: () => void) {
    Row() {
      AppSymbol({ kind: icon, tint: color === this.tDanger ? this.tDanger : this.tMuted, iconSize: 18 })
      Text(label)
        .fontSize(AppTheme.fontSmall)
        .fontColor(color)
        .margin({ left: 12 })
        .layoutWeight(1)
    }
    .width('100%')
    .padding({ left: 14, right: 14, top: 12, bottom: 12 })
    .onClick(() => onTap())
  }
}

@Component
struct SectionQuick {
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

  modules: QuickMod[] = [];
  onOpen: (m: QuickMod) => void = (_m: QuickMod) => {};
  onEdit: () => void = () => {};

  build() {
    Column() {
      Row() {
        Text('快捷入口')
          .fontSize(AppTheme.fontCaption)
          .fontWeight(FontWeight.Bold)
          .fontColor(this.tAccent)
          .letterSpacing(0.8)
          .layoutWeight(1)
        Column() {
          DrawIcon({ kind: 'add', tint: this.tAccent, iconSize: 16 })
        }
        .width(24)
        .height(24)
        .justifyContent(FlexAlign.Center)
        .onClick(() => this.onEdit())
      }
      .width('100%')
      .padding({ left: 6, right: 6, bottom: 4 })

      Flex({ wrap: FlexWrap.Wrap }) {
        ForEach(this.modules, (m: QuickMod) => {
          Column() {
            QuickTile({
              title: m.title,
              iconKind: m.iconKind,
              tint: m.tint,
              onTap: () => this.onOpen(m)
            })
          }
          .width('25%')
        }, (m: QuickMod) => m.key)
      }
    }
    .width('100%')
    .padding({ top: 10, bottom: 10, left: 6, right: 6 })
    .backgroundColor(this.tCard)
    .borderRadius(AppTheme.rMd)
    .shadow({ radius: 8, color: '#14000000', offsetY: 2 })
  }
}

@Entry
@Component
struct DashboardPage {
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
      DashboardTab({
        onOpenSettings: () => NavUtil.push('pages/SettingsPage'),
        onOpenHost: (id: string) => NavUtil.push('pages/HostDetailPage', { hostId: id }),
        onOpenAlerts: (_level: string) => NavUtil.push('pages/AlertsPage')
      })
    }
    .width('100%')
    .height('100%')
    .backgroundColor(this.tBg)
  }
}
