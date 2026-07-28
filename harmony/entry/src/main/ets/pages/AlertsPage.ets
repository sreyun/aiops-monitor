import { Api } from '../data/Api';
import { ApiClient } from '../data/ApiClient';
import { FormatUtil } from '../common/FormatUtil';
import { Alert } from '../data/models/Models';
import {
  ConfirmDialogHost,
  EmptyHint,
  ErrorBanner,
  FilterChips,
  LoadingBlock,
  PageHeader
} from '../components/CommonUI';
import { AlertCardView, ToastHost } from '../components/InteractionUI';
import { SessionTicker } from '../data/SessionTicker';
import { NavUtil } from '../common/NavUtil';
import { AppTheme } from '../common/Routes';


@Component
export struct AlertsTab {
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


  @Prop @Watch('onInitialLevel') initialLevel: string = '';
  onDiagnose: (hostId: string, context?: string, display?: string) => void =
    (_id: string, _c?: string, _d?: string) => {};
  @State alerts: Alert[] = [];
  @State levelIdx: number = 0;
  @State statusIdx: number = 0;
  @State error: string = '';
  @State loading: boolean = false;
  @State syncing: boolean = false;
  @State busyKey: string = '';
  @State silenceVisible: boolean = false;
  @State silenceTarget: Alert | null = null;
  @State toastMsg: string = '';
  @State toastError: boolean = false;
  private levelKeys: string[] = ['', 'critical', 'warning'];
  private levelLabels: string[] = ['全部', '严重', '警告'];
  private statusKeys: string[] = ['', 'active', 'acknowledged', 'silenced'];
  private statusLabels: string[] = ['全部', '待处理', '已确认', '已静默'];
  private pollId: number = -1;
  private inFlight: boolean = false;
  private reqId: number = 0;
  private prevLevelIdx: number = 0;

  aboutToAppear(): void {
    this.applyInitialLevel();
    void this.refresh(false);
    this.pollId = SessionTicker.start(15000, () => {
      void this.refresh(true);
    });
  }

  aboutToDisappear(): void {
    SessionTicker.stop(this.pollId);
    this.pollId = -1;
  }

  onInitialLevel(): void {
    this.applyInitialLevel();
  }

  private applyInitialLevel(): void {
    const lv = (this.initialLevel || '').toLowerCase();
    if (!lv) {
      return;
    }
    for (let i = 0; i < this.levelKeys.length; i++) {
      if (this.levelKeys[i] === lv) {
        this.levelIdx = i;
        return;
      }
    }
  }

  private alertKey(a: Alert): string {
    return `${a.host_id}|${a.type}|${a.scope || ''}`;
  }

  private matchesStatus(a: Alert): boolean {
    const key = this.statusKeys[this.statusIdx] || '';
    if (!key) {
      return true;
    }
    const st = (a.status || '').toLowerCase();
    if (key === 'active') {
      // Android: status.isNullOrBlank()
      return !st;
    }
    return st === key;
  }

  private matchesLevel(a: Alert): boolean {
    const key = this.levelKeys[this.levelIdx] || '';
    if (!key) {
      return true;
    }
    const st = (a.status || '').toLowerCase();
    if (key === 'active') {
      return !st;
    }
    if (key === 'resolved') {
      return st === 'acknowledged' || st === 'silenced';
    }
    return (a.level || '').toLowerCase() === key;
  }

  filtered(): Alert[] {
    const out: Alert[] = [];
    for (let i = 0; i < this.alerts.length; i++) {
      const a = this.alerts[i];
      if (this.matchesLevel(a) && this.matchesStatus(a)) {
        out.push(a);
      }
    }
    return out;
  }

  private countLevel(key: string): number {
    const statusKey = this.statusKeys[this.statusIdx] || '';
    let n = 0;
    for (let i = 0; i < this.alerts.length; i++) {
      const a = this.alerts[i];
      if (!this.matchesStatusKey(a, statusKey)) {
        continue;
      }
      if (!key) {
        n++;
      } else if ((a.level || '').toLowerCase() === key) {
        n++;
      }
    }
    return n;
  }

  private matchesStatusKey(a: Alert, key: string): boolean {
    if (!key) {
      return true;
    }
    const st = (a.status || '').toLowerCase();
    if (key === 'active') {
      return !st;
    }
    return st === key;
  }

  private countStatus(key: string): number {
    const levelKey = this.levelKeys[this.levelIdx] || '';
    let n = 0;
    for (let i = 0; i < this.alerts.length; i++) {
      const a = this.alerts[i];
      if (levelKey === 'active') {
        if ((a.status || '').toLowerCase()) {
          continue;
        }
      } else if (levelKey === 'resolved') {
        const st = (a.status || '').toLowerCase();
        if (st !== 'acknowledged' && st !== 'silenced') {
          continue;
        }
      } else if (levelKey && (a.level || '').toLowerCase() !== levelKey) {
        continue;
      }
      if (!this.matchesStatusKey(a, key)) {
        continue;
      }
      n++;
    }
    return n;
  }

  private levelCounts(): number[] {
    return [
      this.countLevel(''),
      this.countLevel('critical'),
      this.countLevel('warning')
    ];
  }

  private statusCounts(): number[] {
    return [
      this.countStatus(''),
      this.countStatus('active'),
      this.countStatus('acknowledged'),
      this.countStatus('silenced')
    ];
  }

  private subtitle(): string {
    if (this.loading && this.alerts.length === 0) {
      return '正在同步告警数据…';
    }
    if (this.alerts.length === 0) {
      return '所有系统运行正常，无活动告警';
    }
    return `共 ${this.alerts.length} 条告警 · 15 秒自动刷新`;
  }

  private toast(msg: string, err: boolean = false): void {
    this.toastMsg = msg;
    this.toastError = err;
    setTimeout(() => {
      if (this.toastMsg === msg) {
        this.toastMsg = '';
      }
    }, 2200);
  }

  async refresh(silent: boolean = false): Promise<void> {
    if (this.inFlight || !ApiClient.isSessionAlive()) {
      return;
    }
    this.inFlight = true;
    const myId = ++this.reqId;
    if (!silent || this.alerts.length === 0) {
      this.loading = true;
    } else {
      this.syncing = true;
    }
    if (!silent) {
      this.error = '';
    }
    try {
      const list = await Api.alerts();
      if (myId !== this.reqId) {
        return;
      }
      this.alerts = list ?? [];
      this.error = '';
    } catch (e) {
      if (myId !== this.reqId) {
        return;
      }
      this.error = ApiClient.formatError(e as Object);
    } finally {
      if (myId === this.reqId) {
        this.loading = false;
        this.syncing = false;
        this.inFlight = false;
      }
    }
  }

  private async doAck(a: Alert): Promise<void> {
    const key = this.alertKey(a);
    this.busyKey = key;
    try {
      await Api.ackAlert({ host_id: a.host_id, type: a.type, scope: a.scope });
      this.toast(`告警已确认：${a.hostname || a.host_id} / ${(a.type || '').toUpperCase()}`);
      await this.refresh();
    } catch (e) {
      this.toast(ApiClient.formatError(e as Object), true);
    } finally {
      this.busyKey = '';
    }
  }

  private silenceMessage(): string {
    const a = this.silenceTarget;
    if (!a) {
      return '静默后同类告警将暂时不再通知。';
    }
    return `主机 ${a.hostname || a.host_id}\n级别 ${FormatUtil.levelLabel(a.level)} · 类型 ${this.localizeType(a.type)}\n\n静默后同类告警将暂时不再通知。`;
  }

  private localizeType(t: string): string {
    const x = (t || '').toLowerCase();
    if (x === 'offline') {
      return '失联';
    }
    if (x === 'cpu') {
      return 'CPU';
    }
    if (x === 'mem' || x === 'memory') {
      return '内存';
    }
    if (x === 'disk') {
      return '磁盘';
    }
    if (x === 'diskio' || x === 'disk_io') {
      return '磁盘IO';
    }
    if (x === 'gpu') {
      return 'GPU';
    }
    if (x === 'load') {
      return '负载';
    }
    if (x === 'process') {
      return '进程';
    }
    if (x === 'api') {
      return 'API';
    }
    if (x === 'net' || x === 'network') {
      return '网络';
    }
    if (x === 'probe' || x === 'check') {
      return '拨测';
    }
    return (t || '—').toUpperCase();
  }

  private async doSilence(): Promise<void> {
    const a = this.silenceTarget;
    if (!a || this.busyKey) {
      return;
    }
    const key = this.alertKey(a);
    this.busyKey = key;
    try {
      await Api.silenceAlert({ host_id: a.host_id, type: a.type, scope: a.scope });
      this.silenceVisible = false;
      this.toast(`告警已静默：${a.hostname || a.host_id} / ${this.localizeType(a.type)}`);
      await this.refresh();
    } catch (e) {
      this.toast(ApiClient.formatError(e as Object), true);
    } finally {
      this.busyKey = '';
      this.silenceTarget = null;
    }
  }

  private sortedFiltered(): Alert[] {
    const list = this.filtered().slice();
    list.sort((a: Alert, b: Alert) => {
      const as = (a.status || '').toLowerCase();
      const bs = (b.status || '').toLowerCase();
      const aRank = !as ? 0 : (as === 'acknowledged' ? 1 : (as === 'silenced' ? 2 : 3));
      const bRank = !bs ? 0 : (bs === 'acknowledged' ? 1 : (bs === 'silenced' ? 2 : 3));
      if (aRank !== bRank) {
        return aRank - bRank;
      }
      const al = (a.level || '').toLowerCase();
      const bl = (b.level || '').toLowerCase();
      const aLv = al === 'critical' ? 0 : (al === 'warning' ? 1 : 2);
      const bLv = bl === 'critical' ? 0 : (bl === 'warning' ? 1 : 2);
      if (aLv !== bLv) {
        return aLv - bLv;
      }
      return (b.timestamp ?? b.since ?? 0) - (a.timestamp ?? a.since ?? 0);
    });
    return list;
  }

  private clearFilters(): void {
    this.levelIdx = 0;
    this.statusIdx = 0;
  }

  private onLevelSelect(i: number): void {
    // FilterChips 已先写入 selected=i；对齐 Android 再点同一级别回到「全部」
    if (this.prevLevelIdx === i && i !== 0) {
      this.levelIdx = 0;
      this.prevLevelIdx = 0;
    } else {
      this.levelIdx = i;
      this.prevLevelIdx = i;
    }
    this.statusIdx = 0;
  }

  private onStatusSelect(i: number): void {
    this.statusIdx = i;
    const lv = this.levelKeys[this.levelIdx] || '';
    if (lv === 'active' || lv === 'resolved') {
      this.levelIdx = 0;
    }
  }

  private localizeLevel(level: string): string {
    const lv = (level || '').toLowerCase();
    if (lv === 'critical') {
      return '严重';
    }
    if (lv === 'warning') {
      return '警告';
    }
    return level || '';
  }

  /** Android AiCopilotViewModel.diagnoseAlert — full request context */
  private diagnoseCtx(a: Alert): string {
    const levelLabel = this.localizeLevel(a.level || '');
    let durationText = '未知';
    if ((a.since ?? 0) > 0) {
      const seconds = Math.floor(Date.now() / 1000) - (a.since ?? 0);
      if (seconds >= 86400) {
        durationText = `${Math.floor(seconds / 86400)}天${Math.floor((seconds % 86400) / 3600)}小时`;
      } else if (seconds >= 3600) {
        durationText = `${Math.floor(seconds / 3600)}小时${Math.floor((seconds % 3600) / 60)}分钟`;
      } else if (seconds >= 60) {
        durationText = `${Math.floor(seconds / 60)}分钟`;
      } else {
        durationText = `${seconds}秒`;
      }
    }
    const lines: string[] = [
      '【告警上下文】',
      `- 告警级别：${levelLabel}`,
      `- 告警类型：${a.type}`,
      `- 主机名：${a.hostname || a.host_id}（host_id=${a.host_id}）`
    ];
    if (a.ip) {
      lines.push(`- 主机 IP：${a.ip}`);
    }
    lines.push(`- 告警消息：${a.message || ''}`);
    if (a.scope) {
      lines.push(`- 告警范围：${a.scope}`);
    }
    lines.push(`- 告警值：${a.value ?? ''}`);
    lines.push(`- 持续时间：${durationText}`);
    lines.push('');
    lines.push('请基于以上告警上下文进行深度分析：');
    lines.push('1. 查询该主机当前所有性能指标（CPU/内存/磁盘/负载/网络/IO）');
    lines.push('2. 检索该主机最近 30 分钟的 error/warn 级别日志');
    lines.push('3. 查看该主机当前所有活跃告警');
    lines.push('4. 在历史案例库中搜索相似故障的处理经验');
    lines.push('5. 综合分析可能的根因、影响范围和风险等级');
    lines.push('6. 给出按优先级排序的处置建议');
    return lines.join('\n');
  }

  private diagnoseDisplay(a: Alert): string {
    const levelLabel = this.localizeLevel(a.level || '');
    return `AI 诊断告警：${a.hostname || a.host_id} 的 ${levelLabel} ${(a.type || '').toUpperCase()}`;
  }

  build() {
    Stack() {
      Column() {
        PageHeader({
          title: '告警中心',
          subtitle: this.subtitle(),
          showLoading: this.syncing,
          trailing2: '刷新',
          onTrailing2: () => {
            void this.refresh(false);
          },
          trailing: '治理',
          onTrailing: () => {
            NavUtil.push('pages/AlertExtrasPage');
          }
        })
        ErrorBanner({
          message: this.error,
          showRetry: this.error.length > 0,
          onRetry: () => {
            void this.refresh(false);
          }
        })

        if (this.alerts.length > 0) {
          Text('告警级别')
            .fontSize(AppTheme.fontMicro)
            .fontWeight(FontWeight.Medium)
            .fontColor(this.tMuted)
            .width('100%')
            .padding({ left: AppTheme.pagePadH, right: AppTheme.pagePadH, bottom: 6 })
          FilterChips({
            labels: this.levelLabels,
            counts: this.levelCounts(),
            selected: $levelIdx,
            onSelect: (i: number) => {
              this.onLevelSelect(i);
            }
          })
            .padding({ left: AppTheme.pagePadH, right: AppTheme.pagePadH, bottom: 8 })

          Text('处理状态')
            .fontSize(AppTheme.fontMicro)
            .fontWeight(FontWeight.Medium)
            .fontColor(this.tMuted)
            .width('100%')
            .padding({ left: AppTheme.pagePadH, right: AppTheme.pagePadH, bottom: 6 })
          FilterChips({
            labels: this.statusLabels,
            counts: this.statusCounts(),
            selected: $statusIdx,
            onSelect: (i: number) => {
              this.onStatusSelect(i);
            }
          })
            .padding({ left: AppTheme.pagePadH, right: AppTheme.pagePadH, bottom: 8 })

          Row() {
            Text('告警列表')
              .fontSize(AppTheme.fontLead)
              .fontWeight(FontWeight.Bold)
              .fontColor(this.tText)
            Blank()
            Text(`显示 ${this.filtered().length} / ${this.alerts.length}`)
              .fontSize(AppTheme.fontMicro)
              .fontColor(this.tMuted)
          }
          .width('100%')
          .padding({ left: AppTheme.pagePadH, right: AppTheme.pagePadH, bottom: 8 })
        }

        if (this.alerts.length === 0 && this.loading) {
          LoadingBlock({ text: '加载告警…', fill: true })
        } else if (this.alerts.length === 0 && !this.loading) {
          EmptyHint({ text: '所有系统运行正常 · 暂无活动告警', fill: true })
        } else if (this.filtered().length === 0) {
          Column({ space: 16 }) {
            EmptyHint({ text: '当前筛选条件下没有告警' })
            Button('清除筛选')
              .height(44)
              .fontSize(AppTheme.fontBody)
              .fontWeight(FontWeight.Medium)
              .backgroundColor(this.tAccentSoft)
              .fontColor(this.tAccent)
              .borderRadius(AppTheme.rSm)
              .padding({ left: 24, right: 24 })
              .onClick(() => this.clearFilters())
          }
          .layoutWeight(1)
          .width('100%')
          .justifyContent(FlexAlign.Center)
        } else {
          List() {
            ForEach(this.sortedFiltered(), (a: Alert, idx: number) => {
              ListItem() {
                AlertCardView({
                  alert: a,
                  busy: this.busyKey === this.alertKey(a),
                  onAck: () => {
                    void this.doAck(a);
                  },
                  onSilence: () => {
                    this.silenceTarget = a;
                    this.silenceVisible = true;
                  },
                  onDiagnose: () => {
                    this.onDiagnose(a.host_id, this.diagnoseCtx(a), this.diagnoseDisplay(a));
                  }
                })
              }
              .width('100%')
              .padding({ left: AppTheme.pagePadH, right: AppTheme.pagePadH, bottom: 10 })
            }, (a: Alert, idx: number) => `${this.alertKey(a)}-${idx}`)
          }
          .layoutWeight(1)
          .width('100%')
          .scrollBar(BarState.Off)
        }
      }
      .width('100%')
      .height('100%')
      .backgroundColor(this.tBg)

      ConfirmDialogHost({
        visible: this.silenceVisible,
        title: '静默告警？',
        message: this.silenceMessage(),
        confirmText: this.busyKey ? '处理中…' : '静默',
        danger: false,
        onConfirm: () => {
          void this.doSilence();
        },
        onCancel: () => {
          if (!this.busyKey) {
            this.silenceVisible = false;
            this.silenceTarget = null;
          }
        }
      })

      ToastHost({ message: this.toastMsg, isError: this.toastError })
    }
    .width('100%')
    .height('100%')
  }
}

@Entry
@Component
struct AlertsPage {
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
      AlertsTab()
    }
    .width('100%')
    .height('100%')
  }
}
