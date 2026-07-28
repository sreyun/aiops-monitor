import { Api } from '../data/Api';
import { ApiClient } from '../data/ApiClient';
import { FormatUtil } from '../common/FormatUtil';
import {
  HostExecutionResult,
  PlaybookExecution,
  PlaybookStepResult
} from '../data/models/Models';
import {
  EmptyHint,
  ErrorBanner,
  KeyValueRow,
  PageHeader,
  SectionHeader,
  SeverityBadge
} from '../components/CommonUI';
import { NavUtil } from '../common/NavUtil';
import { SessionTicker } from '../data/SessionTicker';
import { AppTheme } from '../common/Routes';


interface HostResultView {
  hostId: string;
  hostname: string;
  status: string;
  output: string;
  successSteps: number;
  failedSteps: number;
  stepCount: number;
  steps: PlaybookStepResult[];
  expanded: boolean;
}

@Entry
@Component
struct ExecutionDetailPage {
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

  @State executionId: number = 0;
  @State ex: PlaybookExecution | null = null;
  @State hosts: HostResultView[] = [];
  @State error: string = '';
  @State loading: boolean = false;
  @State lastUpdated: number = 0;
  private pollId: number = -1;
  private inFlight: boolean = false;
  private expandMap: Map<string, boolean> = new Map();

  aboutToAppear(): void {
    const raw =
      NavUtil.paramString('executionId', '') ||
      NavUtil.paramString('id', '');
    this.executionId = Number(raw) || 0;
    void this.load(true);
    // Android：running 时每 2s 自动刷新
    this.pollId = SessionTicker.start(2000, () => {
      const st = (this.ex?.status || '').toLowerCase();
      if (st === 'running' || st === 'pending' || !this.ex) {
        void this.load(false);
      }
    });
  }

  aboutToDisappear(): void {
    SessionTicker.stop(this.pollId);
    this.pollId = -1;
  }

  private hostStatusRank(status: string): number {
    const s = (status || '').toLowerCase();
    if (s === 'failed' || s === 'timeout') {
      return 0;
    }
    if (s === 'running' || s === 'pending') {
      return 1;
    }
    return 2;
  }

  private localizeExecStatus(st: string): string {
    const s = (st || '').toLowerCase();
    if (s === 'running') {
      return '执行中';
    }
    if (s === 'completed' || s === 'success') {
      return '已完成';
    }
    if (s === 'failed') {
      return '执行失败';
    }
    if (s === 'cancelled') {
      return '已取消';
    }
    return st || '—';
  }

  private localizeHostStatus(st: string): string {
    const s = (st || '').toLowerCase();
    if (s === 'pending') {
      return '等待中';
    }
    if (s === 'running') {
      return '执行中';
    }
    if (s === 'success') {
      return '成功';
    }
    if (s === 'failed') {
      return '失败';
    }
    if (s === 'timeout') {
      return '超时';
    }
    return st || '—';
  }

  private statusLevel(st: string): string {
    const s = (st || '').toLowerCase();
    if (s.indexOf('fail') >= 0 || s.indexOf('error') >= 0 || s === 'timeout') {
      return 'critical';
    }
    if (s.indexOf('run') >= 0 || s.indexOf('pend') >= 0) {
      return 'warning';
    }
    if (s.indexOf('ok') >= 0 || s.indexOf('success') >= 0 || s.indexOf('done') >= 0 ||
      s === 'completed') {
      return 'info';
    }
    return 'warning';
  }

  private startSec(ex: PlaybookExecution): number {
    return ex.start_time || ex.started_at || 0;
  }

  private endSec(ex: PlaybookExecution): number {
    return ex.end_time || ex.finished_at || 0;
  }

  private durationLabel(ex: PlaybookExecution): string {
    const start = this.startSec(ex);
    if (!start) {
      return '—';
    }
    const end = this.endSec(ex) || Math.floor(Date.now() / 1000);
    let sec = end - start;
    if (sec < 0) {
      sec = 0;
    }
    if (sec >= 3600) {
      const h = Math.floor(sec / 3600);
      const m = Math.floor((sec % 3600) / 60);
      return `${h}h${m < 10 ? '0' : ''}${m}m`;
    }
    if (sec >= 60) {
      const m = Math.floor(sec / 60);
      const s = sec % 60;
      return `${m}m${s < 10 ? '0' : ''}${s}s`;
    }
    return `${sec}s`;
  }

  private parseHosts(ex: PlaybookExecution): HostResultView[] {
    const out: HostResultView[] = [];
    const raw = ex.host_results;
    if (!raw) {
      return out;
    }
    const keys: string[] = Object.keys(raw as Object);
    for (let i = 0; i < keys.length; i++) {
      const hostId = keys[i];
      const r: HostExecutionResult = (raw as Record<string, HostExecutionResult>)[hostId];
      if (!r) {
        continue;
      }
      const steps: PlaybookStepResult[] = r.steps ?? [];
      let successSteps = 0;
      let failedSteps = 0;
      for (let j = 0; j < steps.length; j++) {
        const st = (steps[j].status || '').toLowerCase();
        if (st === 'success') {
          successSteps++;
        } else if (st === 'failed') {
          failedSteps++;
        }
      }
      const status = r.status || 'pending';
      let expanded = this.expandMap.has(hostId) ?
        (this.expandMap.get(hostId) === true) :
        (status === 'failed' || status === 'timeout');
      if (status === 'failed' || status === 'timeout') {
        expanded = true;
        this.expandMap.set(hostId, true);
      }
      out.push({
        hostId: hostId,
        hostname: r.hostname || hostId,
        status: status,
        output: r.output || '',
        successSteps: successSteps,
        failedSteps: failedSteps,
        stepCount: steps.length,
        steps: steps,
        expanded: expanded
      });
    }
    out.sort((a: HostResultView, b: HostResultView) => {
      const ra = this.hostStatusRank(a.status);
      const rb = this.hostStatusRank(b.status);
      if (ra !== rb) {
        return ra - rb;
      }
      return a.hostname.localeCompare(b.hostname);
    });
    return out;
  }

  private countByStatus(key: string): number {
    let n = 0;
    for (let i = 0; i < this.hosts.length; i++) {
      const st = (this.hosts[i].status || '').toLowerCase();
      if (key === 'success' && st === 'success') {
        n++;
      } else if (key === 'failed' && (st === 'failed' || st === 'timeout')) {
        n++;
      } else if (key === 'pending' && st !== 'success' && st !== 'failed' && st !== 'timeout') {
        n++;
      }
    }
    return n;
  }

  private toggleHost(hostId: string): void {
    const next = !(this.expandMap.get(hostId) === true);
    this.expandMap.set(hostId, next);
    const copy: HostResultView[] = [];
    for (let i = 0; i < this.hosts.length; i++) {
      const h = this.hosts[i];
      if (h.hostId === hostId) {
        copy.push({
          hostId: h.hostId,
          hostname: h.hostname,
          status: h.status,
          output: h.output,
          successSteps: h.successSteps,
          failedSteps: h.failedSteps,
          stepCount: h.stepCount,
          steps: h.steps,
          expanded: next
        });
      } else {
        copy.push(h);
      }
    }
    this.hosts = copy;
  }

  private async load(showLoading: boolean): Promise<void> {
    if (!this.executionId || this.inFlight) {
      if (!this.executionId) {
        this.error = '缺少执行 ID';
      }
      return;
    }
    this.inFlight = true;
    if (showLoading) {
      this.loading = true;
    }
    try {
      const ex = await Api.playbookExecution(this.executionId);
      this.ex = ex;
      this.hosts = this.parseHosts(ex);
      this.lastUpdated = Date.now();
      this.error = '';
    } catch (e) {
      this.error = ApiClient.formatError(e as Object);
    } finally {
      this.inFlight = false;
      this.loading = false;
    }
  }

  private syncHint(): string {
    const st = (this.ex?.status || '').toLowerCase();
    if (st === 'running' || st === 'pending') {
      return '每 2 秒自动刷新，完成后会展示每台主机的步骤输出';
    }
    if (this.lastUpdated > 0) {
      return `最后同步 ${FormatUtil.relTime(Math.floor(this.lastUpdated / 1000))}`;
    }
    return '执行结果已落库';
  }

  build() {
    Column() {
      PageHeader({
        title: '剧本执行详情',
        showBack: true,
        onBack: () => NavUtil.back(),
        trailing: this.loading ? '…' : '刷新',
        onTrailing: () => {
          void this.load(true);
        }
      })
      ErrorBanner({ message: this.error })

      if (!this.ex && this.loading) {
        EmptyHint({ text: '加载中…' })
      } else if (!this.ex) {
        EmptyHint({ text: this.error ? '加载失败' : '执行实例不存在' })
      } else {
        Scroll() {
          Column() {
            Column() {
              Row() {
                Text(this.ex.playbook_name || this.ex.playbook_id || `执行 #${this.ex.id}`)
                  .fontSize(AppTheme.fontTitle)
                  .fontWeight(FontWeight.Bold)
                  .fontColor(this.tText)
                  .layoutWeight(1)
                SeverityBadge({ level: this.statusLevel(this.ex.status || '') })
              }
              .width('100%')

              Text(`执行人 ${this.ex.operator || 'system'} · ${FormatUtil.relTime(this.startSec(this.ex))}`)
                .fontSize(AppTheme.fontCaption)
                .fontColor(this.tMuted)
                .margin({ top: 6 })

              Text(this.localizeExecStatus(this.ex.status || ''))
                .fontSize(AppTheme.fontCaption)
                .fontColor(this.tAccent)
                .margin({ top: 4 })

              Row() {
                this.metricCell('成功', `${this.countByStatus('success')}`, this.tSuccess)
                this.metricCell('失败', `${this.countByStatus('failed')}`,
                  this.countByStatus('failed') > 0 ? this.tDanger : this.tMuted)
                this.metricCell('处理中', `${this.countByStatus('pending')}`,
                  this.countByStatus('pending') > 0 ? this.tAccent : this.tMuted)
                this.metricCell('耗时', this.durationLabel(this.ex), this.tText)
              }
              .width('100%')
              .justifyContent(FlexAlign.SpaceBetween)
              .margin({ top: 12 })

              Text(this.syncHint())
                .fontSize(AppTheme.fontMicro)
                .fontColor(this.tMuted)
                .margin({ top: 10 })
            }
            .width('100%')
            .padding(AppTheme.cardPad)
            .margin({ left: 14, right: 14, top: 8, bottom: 10 })
            .backgroundColor(this.tCard)
            .borderRadius(AppTheme.rMd)

            if (this.ex.error) {
              Text(this.ex.error)
                .fontSize(AppTheme.fontSmall)
                .fontColor(this.tDanger)
                .padding({ left: 16, right: 16, bottom: 8 })
            }

            KeyValueRow({ label: '状态', value: this.localizeExecStatus(this.ex.status || '') })
              .padding({ left: 16, right: 16 })
            KeyValueRow({ label: '开始', value: FormatUtil.relTime(this.startSec(this.ex)) })
              .padding({ left: 16, right: 16 })
            KeyValueRow({
              label: '结束',
              value: this.endSec(this.ex) ? FormatUtil.relTime(this.endSec(this.ex)) : '进行中'
            })
              .padding({ left: 16, right: 16 })

            Row() {
              SectionHeader({ title: '主机执行结果' })
              Blank().layoutWeight(1)
              Text(`${this.hosts.length} 台`)
                .fontSize(AppTheme.fontCaption)
                .fontColor(this.tMuted)
                .padding({ right: 16 })
            }
            .width('100%')
            .alignItems(VerticalAlign.Center)

            if (this.hosts.length === 0) {
              Column() {
                Text('等待目标主机接收任务')
                  .fontSize(AppTheme.fontBody)
                  .fontWeight(FontWeight.Medium)
                  .fontColor(this.tText)
                Text('Agent 接收任务后，这里会出现主机和步骤结果')
                  .fontSize(AppTheme.fontCaption)
                  .fontColor(this.tMuted)
                  .margin({ top: 6 })
              }
              .width('100%')
              .padding(26)
              .margin({ left: 14, right: 14 })
              .backgroundColor(this.tCard)
              .borderRadius(AppTheme.rSm)
              .alignItems(HorizontalAlign.Center)
            }

            ForEach(this.hosts, (h: HostResultView) => {
              Column() {
                Row() {
                  Column() {
                    Text(h.hostname)
                      .fontSize(AppTheme.fontBody)
                      .fontWeight(FontWeight.Bold)
                      .fontColor(this.tText)
                      .maxLines(1)
                    Text(`${h.hostId} · ${h.stepCount} 步 · 成功 ${h.successSteps}` +
                      (h.failedSteps > 0 ? ` · 失败 ${h.failedSteps}` : ''))
                      .fontSize(AppTheme.fontMicro)
                      .fontColor(this.tMuted)
                      .margin({ top: 2 })
                      .maxLines(1)
                  }
                  .alignItems(HorizontalAlign.Start)
                  .layoutWeight(1)
                  Text(this.localizeHostStatus(h.status))
                    .fontSize(AppTheme.fontMicro)
                    .fontColor(this.tAccent)
                    .padding({ left: 8, right: 8, top: 2, bottom: 2 })
                    .backgroundColor(this.tElevated)
                    .borderRadius(AppTheme.rChip)
                  Text(h.expanded ? '收起' : '展开')
                    .fontSize(AppTheme.fontMicro)
                    .fontColor(this.tMuted)
                    .margin({ left: 8 })
                }
                .width('100%')

                if (h.expanded) {
                  if (h.steps.length === 0) {
                    Text(h.output || '（无标准输出）')
                      .fontSize(AppTheme.fontMicro)
                      .fontFamily('monospace')
                      .fontColor(this.tMuted)
                      .width('100%')
                      .padding(9)
                      .margin({ top: 8 })
                      .backgroundColor('#6A000000')
                      .borderRadius(AppTheme.rXs)
                      .maxLines(20)
                  } else {
                    ForEach(h.steps, (step: PlaybookStepResult, idx: number) => {
                      Column() {
                        Row() {
                          Text(`${idx + 1}`)
                            .fontSize(AppTheme.fontCaption)
                            .fontFamily('monospace')
                            .fontColor(this.tAccent)
                          Text(step.name || '未命名步骤')
                            .fontSize(AppTheme.fontSmall)
                            .fontColor(this.tText)
                            .layoutWeight(1)
                            .margin({ left: 8 })
                          Text(this.localizeHostStatus(step.status || ''))
                            .fontSize(AppTheme.fontMicro)
                            .fontColor(this.tMuted)
                        }
                        .width('100%')
                        if (step.output) {
                          Text(step.output.length > 2000 ?
                            step.output.substring(step.output.length - 2000) : step.output)
                            .fontSize(AppTheme.fontMicro)
                            .fontFamily('monospace')
                            .fontColor(this.tMuted)
                            .width('100%')
                            .padding(AppTheme.spaceSm)
                            .margin({ top: 6 })
                            .backgroundColor('#6A000000')
                            .borderRadius(AppTheme.rChip)
                            .maxLines(16)
                        }
                      }
                      .width('100%')
                      .padding(10)
                      .margin({ top: 8 })
                      .backgroundColor(this.tBg)
                      .borderRadius(AppTheme.rXs)
                    }, (step: PlaybookStepResult, idx: number) =>
                      `${h.hostId}-step-${idx}-${step.name ?? ''}`)
                  }
                }
              }
              .width('100%')
              .padding(AppTheme.spaceMd)
              .margin({ left: 14, right: 14, bottom: 10 })
              .backgroundColor(this.tCard)
              .borderRadius(AppTheme.rSm)
              .onClick(() => this.toggleHost(h.hostId))
            }, (h: HostResultView) => h.hostId)

            Blank().height(28)
          }
        }
        .layoutWeight(1)
        .scrollBar(BarState.Off)
      }
    }
    .width('100%')
    .height('100%')
    .backgroundColor(this.tBg)
  }

  @Builder
  metricCell(label: string, value: string, color: string) {
    Column() {
      Text(value)
        .fontSize(AppTheme.fontSubtitle)
        .fontWeight(FontWeight.Bold)
        .fontColor(color)
      Text(label)
        .fontSize(AppTheme.fontMicro)
        .fontColor(this.tMuted)
        .margin({ top: 2 })
    }
    .alignItems(HorizontalAlign.Center)
  }
}
