import router from '@ohos.router';
import { Api } from '../data/Api';
import { ApiClient } from '../data/ApiClient';
import { FormatUtil } from '../common/FormatUtil';
import { DiskInfo, Host, Sample, GpuInfo, ProcessInfo } from '../data/models/Models';
import {
  ErrorBanner,
  KeyValueRow,
  MetricBar,
  PageHeader,
  PrimaryButton,
  StatusDot
} from '../components/CommonUI';
import {
  ChartPoint,
  HistoryChart,
  TimeRangeChips,
  rangeToSeconds,
  samplesToChartPoints
} from '../components/HistoryChart';
import { AiAssistSheet, ToastController, ToastHost } from '../components/InteractionUI';
import { NavUtil } from '../common/NavUtil';
import { SessionTicker } from '../data/SessionTicker';
import { AppTheme } from '../common/Routes';


/**
 * Android HostDetailScreen 1:1
 * metrics = GET hosts/{id}/metrics（最新 Sample）
 * history = GET hosts/{id}/history
 */
@Entry
@Component
struct HostDetailPage {
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

  @State hostId: string = '';
  @State host: Host | null = null;
  @State metrics: Sample | null = null;
  @State rangeKey: string = '1h';
  @State cpuPts: ChartPoint[] = [];
  @State memPts: ChartPoint[] = [];
  @State diskPts: ChartPoint[] = [];
  @State gpuUtilPts: ChartPoint[] = [];
  @State gpuMemPts: ChartPoint[] = [];
  @State history: Sample[] = [];
  @State categoryEdit: string = '';
  @State showCategory: boolean = false;
  @State softError: string = '';
  @State error: string = '';
  @State loading: boolean = false;
  @State toastMsg: string = '';
  @State toastError: boolean = false;
  @State assistVisible: boolean = false;
  @State expandMetric: string = ''; // cpu|mem|disk|gpu|gpumem
  private pollId: number = -1;

  private m(): Sample | null {
    return this.metrics || this.host?.latest || null;
  }

  private gpus(): GpuInfo[] {
    return this.m()?.gpus ?? [];
  }

  private disks(): DiskInfo[] {
    return this.m()?.disks ?? [];
  }

  private procs(): ProcessInfo[] {
    const list = this.m()?.processes ?? [];
    return list.slice().sort((a: ProcessInfo, b: ProcessInfo) => (b.cpu ?? 0) - (a.cpu ?? 0)).slice(0, 10);
  }

  private rangeLabel(): string {
    const map: Record<string, string> = {
      '1h': '1小时',
      '3h': '3小时',
      '6h': '6小时',
      '12h': '12小时',
      '1d': '1天',
      '3d': '3天',
      '7d': '7天',
      '14d': '14天'
    };
    return map[this.rangeKey] || '1小时';
  }

  private toast(msg: string, isError: boolean = false): void {
    this.toastMsg = msg;
    this.toastError = isError;
    ToastController.delayClear((v: string) => {
      this.toastMsg = v;
    }, msg);
  }

  aboutToAppear(): void {
    const params = router.getParams() as Record<string, Object>;
    const raw = params ? params['hostId'] : undefined;
    this.hostId = raw !== undefined && raw !== null ? String(raw) : '';
    void this.load();
    this.pollId = SessionTicker.start(10000, () => {
      void this.load();
    });
  }

  aboutToDisappear(): void {
    SessionTicker.stop(this.pollId);
    this.pollId = -1;
  }

  private pickLatestMetrics(list: Sample[]): Sample | null {
    if (!list || list.length === 0) {
      return null;
    }
    let best = list[0];
    let bestTs = Number(best.timestamp ?? 0);
    for (let i = 1; i < list.length; i++) {
      const ts = Number(list[i].timestamp ?? 0);
      if (ts >= bestTs) {
        best = list[i];
        bestTs = ts;
      }
    }
    return best;
  }

  private async load(): Promise<void> {
    if (!this.hostId) {
      return;
    }
    this.loading = true;
    this.error = '';
    this.softError = '';
    try {
      const hosts = await Api.hosts();
      this.host = hosts.find((h: Host) => h.id === this.hostId) ?? null;
      if (!this.host) {
        this.error = '未找到主机';
        return;
      }
      this.categoryEdit = this.host.category ?? '';

      try {
        const metricList = await Api.hostMetrics(this.hostId);
        this.metrics = this.pickLatestMetrics(metricList) || this.host.latest || null;
      } catch (e) {
        this.metrics = this.host.latest || null;
        this.softError = `实时数据刷新失败：${ApiClient.formatError(e as Object)}`;
      }

      const to = Math.floor(Date.now() / 1000);
      const from = to - rangeToSeconds(this.rangeKey);
      let hist: Sample[] = [];
      try {
        hist = await Api.hostHistory(this.hostId, from, to);
      } catch (_e) {
        hist = [];
      }
      if (hist.length < 2) {
        try {
          const fallback = await Api.hostMetrics(this.hostId);
          hist = fallback;
        } catch (_e2) {
          // keep empty
        }
      }
      this.history = hist;
      this.cpuPts = samplesToChartPoints(hist, 'cpu_percent');
      this.memPts = samplesToChartPoints(hist, 'mem_percent');
      this.diskPts = samplesToChartPoints(hist, 'disk_percent');
      this.gpuUtilPts = samplesToChartPoints(hist, 'gpu_util');
      this.gpuMemPts = samplesToChartPoints(hist, 'gpu_mem');
    } catch (e) {
      this.error = ApiClient.formatError(e as Object);
    } finally {
      this.loading = false;
    }
  }

  private async reloadHistoryOnly(): Promise<void> {
    if (!this.hostId) {
      return;
    }
    try {
      const to = Math.floor(Date.now() / 1000);
      const from = to - rangeToSeconds(this.rangeKey);
      let hist = await Api.hostHistory(this.hostId, from, to);
      if (hist.length < 2) {
        hist = await Api.hostMetrics(this.hostId);
      }
      this.history = hist;
      this.cpuPts = samplesToChartPoints(hist, 'cpu_percent');
      this.memPts = samplesToChartPoints(hist, 'mem_percent');
      this.diskPts = samplesToChartPoints(hist, 'disk_percent');
      this.gpuUtilPts = samplesToChartPoints(hist, 'gpu_util');
      this.gpuMemPts = samplesToChartPoints(hist, 'gpu_mem');
    } catch (e) {
      this.softError = `实时数据刷新失败：${ApiClient.formatError(e as Object)}`;
    }
  }

  private async saveCategory(): Promise<void> {
    try {
      await Api.setHostCategory(this.hostId, this.categoryEdit.trim());
      this.showCategory = false;
      await this.load();
      this.toast('分组已保存');
    } catch (e) {
      this.toast(ApiClient.formatError(e as Object), true);
    }
  }

  private peak(pts: ChartPoint[]): number {
    let maxv = 0;
    for (let i = 0; i < pts.length; i++) {
      if (pts[i].v > maxv) {
        maxv = pts[i].v;
      }
    }
    return maxv;
  }

  private last(pts: ChartPoint[]): number {
    if (pts.length === 0) {
      return 0;
    }
    return pts[pts.length - 1].v;
  }

  private aiContext(): string {
    const h = this.host;
    const m = this.m();
    const lines: string[] = [];
    lines.push(`主机：${h?.hostname || ''}（id=${this.hostId} IP=${h?.ip || ''}）`);
    lines.push(`在线：${h?.online ?? false} 分组：${h?.category || '-'}`);
    if (m) {
      lines.push(
        `CPU ${Number(m.cpu_percent ?? 0).toFixed(1)}%  内存 ${Number(m.mem_percent ?? 0).toFixed(1)}%  磁盘 ${Number(m.disk_percent ?? 0).toFixed(1)}%`
      );
      lines.push(
        `负载 load1=${Number(m.load1 ?? 0).toFixed(2)} load5=${Number(m.load5 ?? 0).toFixed(2)} load15=${Number(m.load15 ?? 0).toFixed(2)}`
      );
    }
    if (this.cpuPts.length > 0 || this.memPts.length > 0) {
      lines.push(
        `近期趋势样本 ${Math.max(this.cpuPts.length, this.memPts.length)}：CPU 峰值 ${this.peak(this.cpuPts).toFixed(1)}% 内存峰值 ${this.peak(this.memPts).toFixed(1)}%`
      );
    }
    return lines.join('\n');
  }

  private expandTitle(): string {
    if (this.expandMetric === 'cpu') {
      return 'CPU 占用率';
    }
    if (this.expandMetric === 'mem') {
      return '内存使用率';
    }
    if (this.expandMetric === 'disk') {
      return '磁盘使用率';
    }
    if (this.expandMetric === 'gpu') {
      return 'GPU 计算利用率';
    }
    if (this.expandMetric === 'gpumem') {
      return 'GPU 显存利用率';
    }
    return '';
  }

  private expandPts(): ChartPoint[] {
    if (this.expandMetric === 'cpu') {
      return this.cpuPts;
    }
    if (this.expandMetric === 'mem') {
      return this.memPts;
    }
    if (this.expandMetric === 'disk') {
      return this.diskPts;
    }
    if (this.expandMetric === 'gpu') {
      return this.gpuUtilPts;
    }
    if (this.expandMetric === 'gpumem') {
      return this.gpuMemPts;
    }
    return [];
  }

  private expandColor(): string {
    if (this.expandMetric === 'cpu') {
      return '#4F7FFF';
    }
    if (this.expandMetric === 'mem') {
      return '#00A86B';
    }
    if (this.expandMetric === 'disk') {
      return '#F59E0B';
    }
    if (this.expandMetric === 'gpu') {
      return '#8B5CF6';
    }
    return '#06B6D4';
  }

  build() {
    Stack() {
      Column() {
        PageHeader({
          title: this.host?.hostname || '主机详情',
          subtitle: this.host ? (this.host.ip || '未知 IP') : '',
          showBack: true,
          showLoading: this.loading,
          trailing3Icon: 'spark',
          onTrailing3: () => {
            if (this.host) {
              this.assistVisible = true;
            }
          },
          trailing2Icon: 'settings',
          onTrailing2: () => {
            if (this.host) {
              this.categoryEdit = this.host.category || '';
              this.showCategory = true;
            }
          },
          trailingIcon: 'refresh',
          onTrailing: () => {
            void this.load();
          }
        })

        if (this.error && !this.host) {
          ErrorBanner({
            message: this.error || '同步错误',
            showRetry: true,
            onRetry: () => {
              void this.load();
            }
          })
        } else if (this.loading && !this.host) {
          Column() {
            LoadingProgress().width(36).height(36).color(this.tAccent)
          }
          .width('100%')
          .layoutWeight(1)
          .justifyContent(FlexAlign.Center)
        } else {
          Scroll() {
            Column({ space: 12 }) {
              if (this.softError) {
                Text(this.softError)
                  .fontSize(AppTheme.fontCaption)
                  .fontColor(this.tWarning)
                  .width('100%')
                  .padding(10)
                  .backgroundColor(this.tWarningSoft)
                  .borderRadius(AppTheme.rXs)
              }

              // 状态行
              Row({ space: 8 }) {
                StatusDot({ online: !!this.host?.online })
                Text(this.host?.online ? '在线' : '离线')
                  .fontSize(AppTheme.fontCaption)
                  .fontWeight(FontWeight.Medium)
                  .fontColor(this.host?.online ? this.tSuccess : this.tDanger)
                  .padding({ left: 8, right: 8, top: 3, bottom: 3 })
                  .backgroundColor(this.host?.online ? this.tSuccessSoft : this.tDangerSoft)
                  .borderRadius(AppTheme.rChip)
                if (this.host?.category) {
                  Text((this.host.category || '').toUpperCase())
                    .fontSize(AppTheme.fontTiny)
                    .fontWeight(FontWeight.Bold)
                    .fontColor(this.tAccent)
                    .padding({ left: 8, right: 8, top: 3, bottom: 3 })
                    .backgroundColor(this.tAccentSoft)
                    .borderRadius(AppTheme.rChip)
                }
                Blank()
                Text(`运行时间: ${FormatUtil.uptimeZh(this.m()?.uptime)}`)
                  .fontSize(AppTheme.fontMicro)
                  .fontColor(this.tMuted)
              }
              .width('100%')

              // 性能趋势 — Android：有 history 才显示；三列 TrendBox；时间范围仅在全屏图
              if (this.history.length > 0) {
                Column({ space: 10 }) {
                  Text(`${this.rangeLabel()}性能趋势 (点击查看详情)`)
                    .fontSize(AppTheme.fontSmall)
                    .fontWeight(FontWeight.Bold)
                    .fontColor(this.tText)
                    .width('100%')

                  Row({ space: 12 }) {
                    this.trendBox('CPU', this.cpuPts, '#4F7FFF', 'cpu')
                    this.trendBox('内存', this.memPts, '#00A86B', 'mem')
                    this.trendBox('磁盘', this.diskPts, '#F59E0B', 'disk')
                  }
                  .width('100%')
                  .height(140)

                  if (this.gpuTrendCount() >= 2) {
                    Row({ space: 12 }) {
                      this.trendBox('GPU 峰值', this.gpuUtilPts, '#8B5CF6', 'gpu')
                      this.trendBox('GPU 显存', this.gpuMemPts, '#06B6D4', 'gpumem')
                    }
                    .width('100%')
                    .height(140)
                  }
                }
                .width('100%')
                .padding(AppTheme.spaceMd)
                .backgroundColor(this.tCard)
                .borderRadius(AppTheme.rSm)
                .border({ width: 1, color: this.tBorderSoft })
                .alignItems(HorizontalAlign.Start)
              }

              // 系统资源
              Column({ space: 8 }) {
                Text('系统资源'.toUpperCase())
                  .fontSize(AppTheme.fontCaption)
                  .fontWeight(FontWeight.Bold)
                  .fontColor(this.tAccent)
                  .letterSpacing(0.8)
                  .width('100%')

                if (!this.m()) {
                  Text('暂未收到实时指标数据')
                    .fontSize(AppTheme.fontCaption)
                    .fontColor(this.tMuted)
                } else {
                  MetricBar({
                    label: 'CPU 负载',
                    value: this.m()?.cpu_percent ?? 0,
                    detailText: this.cpuDetail()
                  })
                  if ((this.m()?.cpu_iowait ?? 0) > 0.1) {
                    KeyValueRow({
                      label: 'CPU I/O 等待',
                      value: `${Number(this.m()?.cpu_iowait ?? 0).toFixed(1)}%`
                    })
                  }
                  MetricBar({
                    label: '内存使用',
                    value: this.m()?.mem_percent ?? 0,
                    detailText: this.memDetail()
                  })
                  if ((this.m()?.swap_percent ?? 0) > 0.1) {
                    MetricBar({
                      label: 'Swap 使用',
                      value: this.m()?.swap_percent ?? 0,
                      detailText: this.swapDetail()
                    })
                  }
                  MetricBar({
                    label: '磁盘使用',
                    value: this.m()?.disk_percent ?? 0,
                    detailText: this.diskDetail()
                  })

                  Divider().color(this.tBorderSoft).strokeWidth(0.5).margin({ top: 4, bottom: 4 })

                  KeyValueRow({
                    label: '系统平均负载',
                    value: `${Number(this.m()?.load1 ?? 0).toFixed(2)} / ${Number(this.m()?.load5 ?? 0).toFixed(2)} / ${Number(this.m()?.load15 ?? 0).toFixed(2)}`
                  })
                  KeyValueRow({ label: '进程总数', value: `${this.m()?.proc_count ?? 0}` })
                  KeyValueRow({ label: '网络发送', value: FormatUtil.bytesRate(this.m()?.net_sent_rate) })
                  KeyValueRow({ label: '网络接收', value: FormatUtil.bytesRate(this.m()?.net_recv_rate) })
                  KeyValueRow({ label: '磁盘读取', value: FormatUtil.bytesRate(this.m()?.disk_read_rate) })
                  KeyValueRow({ label: '磁盘写入', value: FormatUtil.bytesRate(this.m()?.disk_write_rate) })
                  KeyValueRow({
                    label: '磁盘 I/O 利用率',
                    value: `${Number(this.m()?.disk_io_util_percent ?? 0).toFixed(1)}%`
                  })
                }
              }
              .width('100%')
              .padding(AppTheme.spaceMd)
              .backgroundColor(this.tCard)
              .borderRadius(AppTheme.rSm)
              .border({ width: 1, color: this.tBorderSoft })
              .alignItems(HorizontalAlign.Start)

              // GPU
              if (this.gpus().length > 0) {
                Column({ space: 10 }) {
                  Text(`GPU 加速卡 · ${this.gpus().length} 块`.toUpperCase())
                    .fontSize(AppTheme.fontCaption)
                    .fontWeight(FontWeight.Bold)
                    .fontColor(this.tAccent)
                    .letterSpacing(0.8)
                    .width('100%')
                  ForEach(this.gpus(), (g: GpuInfo, gi: number) => {
                    Column({ space: 6 }) {
                      Row() {
                        Column() {
                          Text(g.name || `GPU ${gi}`)
                            .fontSize(AppTheme.fontSmall)
                            .fontWeight(FontWeight.Bold)
                            .fontColor(this.tText)
                            .maxLines(1)
                            .textOverflow({ overflow: TextOverflow.Ellipsis })
                          Text(`GPU ${gi + 1}`)
                            .fontSize(AppTheme.fontTiny)
                            .fontColor(this.tMuted)
                            .margin({ top: 2 })
                        }
                        .layoutWeight(1)
                        .alignItems(HorizontalAlign.Start)
                        if ((g.temp ?? 0) > 0) {
                          Text(`${Math.round(g.temp ?? 0)}°C`)
                            .fontSize(AppTheme.fontMicro)
                            .fontWeight(FontWeight.Bold)
                            .fontColor(FormatUtil.thresholdColor(g.temp ?? 0))
                            .padding({ left: 8, right: 8, top: 2, bottom: 2 })
                            .backgroundColor(this.tElevated)
                            .borderRadius(AppTheme.rChip)
                        }
                      }
                      .width('100%')
                      MetricBar({ label: '计算利用率', value: g.util_percent ?? 0, color: '#8B5CF6' })
                      MetricBar({ label: '显存利用率', value: g.mem_percent ?? 0, color: '#06B6D4' })
                      if ((g.mem_total ?? 0) > 0) {
                        KeyValueRow({
                          label: '显存用量',
                          value: `${FormatUtil.bytes(g.mem_used)} / ${FormatUtil.bytes(g.mem_total)}`
                        })
                        if ((g.mem_free ?? 0) > 0) {
                          KeyValueRow({
                            label: '显存可用',
                            value: FormatUtil.bytes(g.mem_free)
                          })
                        }
                      }
                      KeyValueRow({
                        label: '核心温度',
                        value: (g.temp ?? 0) > 0 ? `${Number(g.temp).toFixed(1)}°C` : '暂未采集'
                      })
                    }
                    .width('100%')
                    .alignItems(HorizontalAlign.Start)
                  }, (g: GpuInfo, gi: number) => `gpu-${gi}-${g.name ?? ''}`)
                }
                .width('100%')
                .padding(AppTheme.spaceMd)
                .backgroundColor(this.tCard)
                .borderRadius(AppTheme.rSm)
                .border({ width: 1, color: this.tBorderSoft })
              }

              // TOP 进程
              if (this.procs().length > 0) {
                Column({ space: 6 }) {
                  Text('TOP 进程'.toUpperCase())
                    .fontSize(AppTheme.fontCaption)
                    .fontWeight(FontWeight.Bold)
                    .fontColor(this.tAccent)
                    .letterSpacing(0.8)
                    .width('100%')
                  Row() {
                    Text('进程名').fontSize(AppTheme.fontMicro).fontColor(this.tMuted).layoutWeight(1)
                    Text('CPU').fontSize(AppTheme.fontMicro).fontColor(this.tMuted).width(52).textAlign(TextAlign.End)
                    Text('内存').fontSize(AppTheme.fontMicro).fontColor(this.tMuted).width(52).textAlign(TextAlign.End)
                  }
                  .width('100%')
                  ForEach(this.procs(), (pr: ProcessInfo, pi: number) => {
                    Row() {
                      Text(pr.name || `pid ${pr.pid ?? ''}`)
                        .fontSize(AppTheme.fontCaption)
                        .fontColor(this.tText)
                        .layoutWeight(1)
                        .maxLines(1)
                        .textOverflow({ overflow: TextOverflow.Ellipsis })
                      Text(`${Number(pr.cpu ?? 0).toFixed(1)}%`)
                        .fontSize(AppTheme.fontCaption)
                        .fontColor('#4F7FFF')
                        .width(52)
                        .textAlign(TextAlign.End)
                      Text(`${Number(pr.mem ?? 0).toFixed(1)}%`)
                        .fontSize(AppTheme.fontCaption)
                        .fontColor('#00A86B')
                        .width(52)
                        .textAlign(TextAlign.End)
                    }
                    .width('100%')
                    .padding({ top: 4, bottom: 4 })
                  }, (pr: ProcessInfo, pi: number) => `p-${pi}-${pr.pid ?? 0}-${pr.name ?? ''}`)
                }
                .width('100%')
                .padding(AppTheme.spaceMd)
                .backgroundColor(this.tCard)
                .borderRadius(AppTheme.rSm)
                .border({ width: 1, color: this.tBorderSoft })
              }

              // 硬件与环境
              Column({ space: 4 }) {
                Text('硬件与环境'.toUpperCase())
                  .fontSize(AppTheme.fontCaption)
                  .fontWeight(FontWeight.Bold)
                  .fontColor(this.tAccent)
                  .letterSpacing(0.8)
                  .width('100%')
                  .margin({ bottom: 4 })
                KeyValueRow({
                  label: '操作系统 / 平台',
                  value: `${this.host?.os || '—'} (${this.host?.platform || '—'})`
                })
                KeyValueRow({ label: '架构', value: this.host?.arch || '—' })
                KeyValueRow({ label: '内核版本', value: this.host?.kernel || '—' })
                KeyValueRow({
                  label: '初次发现',
                  value: FormatUtil.firstSeenZh(this.host?.first_seen)
                })
              }
              .width('100%')
              .padding(AppTheme.spaceMd)
              .backgroundColor(this.tCard)
              .borderRadius(AppTheme.rSm)
              .border({ width: 1, color: this.tBorderSoft })
              .alignItems(HorizontalAlign.Start)

              // 磁盘分区
              if (this.disks().length > 0) {
                Column({ space: 8 }) {
                  Text('磁盘分区'.toUpperCase())
                    .fontSize(AppTheme.fontCaption)
                    .fontWeight(FontWeight.Bold)
                    .fontColor(this.tAccent)
                    .letterSpacing(0.8)
                    .width('100%')
                  ForEach(this.disks(), (d: DiskInfo, di: number) => {
                    Column({ space: 4 }) {
                      MetricBar({
                        label: d.path || `盘 ${di + 1}`,
                        value: d.percent ?? 0
                      })
                      Text(`已用: ${FormatUtil.bytes(d.used)} · 总量: ${FormatUtil.bytes(d.total)}`)
                        .fontSize(AppTheme.fontMicro)
                        .fontColor(this.tMuted)
                        .width('100%')
                      if ((d.inodes_percent ?? 0) > 0) {
                        Text(`Inodes 使用率: ${Number(d.inodes_percent).toFixed(1)}%`)
                          .fontSize(AppTheme.fontMicro)
                          .fontColor(this.tMuted)
                          .width('100%')
                      }
                    }
                    .width('100%')
                    .alignItems(HorizontalAlign.Start)
                  }, (d: DiskInfo, di: number) => `d-${di}-${d.path ?? ''}`)
                }
                .width('100%')
                .padding(AppTheme.spaceMd)
                .backgroundColor(this.tCard)
                .borderRadius(AppTheme.rSm)
                .border({ width: 1, color: this.tBorderSoft })
              }

              PrimaryButton({
                label: this.host?.online ? '打开远程终端' : '主机离线，终端不可用',
                isEnabled: !!this.host?.online,
                onTap: () => {
                  if (this.host?.online) {
                    NavUtil.push('pages/TerminalPage', { hostId: this.hostId });
                  }
                }
              })

              Blank().height(20)
            }
            .width('100%')
            .padding(AppTheme.pagePadH)
          }
          .layoutWeight(1)
          .scrollBar(BarState.Off)
          .align(Alignment.Top)
        }
      }
      .width('100%')
      .height('100%')
      .backgroundColor(this.tBg)

      // 全屏图表
      if (this.expandMetric) {
        Column() {
          Row() {
            Text(this.expandTitle())
              .fontSize(AppTheme.fontLead)
              .fontWeight(FontWeight.Bold)
              .fontColor(this.tText)
              .layoutWeight(1)
            Text('关闭')
              .fontSize(AppTheme.fontBody)
              .fontColor(this.tAccent)
              .onClick(() => {
                this.expandMetric = '';
              })
          }
          .width('100%')
          .padding(AppTheme.pagePadH)

          Text('原生时间序列 · 可精确查看任意采样点')
            .fontSize(AppTheme.fontCaption)
            .fontColor(this.tMuted)
            .padding({ left: 16, right: 16, bottom: 8 })

          TimeRangeChips({
            rangeKey: $rangeKey,
            onChange: (_k: string) => {
              void this.reloadHistoryOnly();
            }
          })
            .padding({ left: 16, right: 16, bottom: 8 })

          HistoryChart({
            title: this.expandTitle(),
            unit: '%',
            color: this.expandColor(),
            points: this.expandPts(),
            heightVp: 220,
            interactive: true
          })
            .padding(AppTheme.pagePadH)
            .layoutWeight(1)

          Text(`${this.expandPts().length} 个采样点`)
            .fontSize(AppTheme.fontMicro)
            .fontColor(this.tMuted)
            .padding(AppTheme.pagePadH)
        }
        .width('100%')
        .height('100%')
        .backgroundColor(this.tBg)
        .zIndex(80)
      }

      // 编辑分组
      if (this.showCategory) {
        Column() {
          Column({ space: 12 }) {
            Text('编辑分组')
              .fontSize(AppTheme.fontLead)
              .fontWeight(FontWeight.Bold)
              .fontColor(this.tText)
            Text('给主机设置分组标签（如 公司专用/家庭专用），留空则清除分组。')
              .fontSize(AppTheme.fontSmall)
              .fontColor(this.tMuted)
              .width('100%')
            TextInput({ text: this.categoryEdit, placeholder: '分组' })
              .onChange((v: string) => {
                this.categoryEdit = v;
              })
              .height(44)
              .backgroundColor(this.tBg)
              .fontColor(this.tText)
              .borderRadius(AppTheme.rSm)
              .border({ width: 1, color: this.tBorderSoft })
            Row({ space: 10 }) {
              Button('取消')
                .layoutWeight(1)
                .height(40)
                .backgroundColor(this.tElevated)
                .fontColor(this.tText)
                .onClick(() => {
                  this.showCategory = false;
                })
              Button('保存')
                .layoutWeight(1)
                .height(40)
                .backgroundColor(this.tAccent)
                .fontColor(this.tOnAccent)
                .onClick(() => {
                  void this.saveCategory();
                })
            }
          }
          .width('86%')
          .padding(18)
          .backgroundColor(this.tCard)
          .borderRadius(AppTheme.rMd)
        }
        .width('100%')
        .height('100%')
        .justifyContent(FlexAlign.Center)
        .backgroundColor(this.tScrim)
        .zIndex(90)
      }

      AiAssistSheet({
        visible: this.assistVisible,
        title: `AI 主机分析 · ${this.host?.hostname || this.hostId}`,
        task: 'chart_analysis',
        context: this.aiContext(),
        onClose: () => {
          this.assistVisible = false;
        }
      })

      ToastHost({ message: this.toastMsg, isError: this.toastError })
    }
    .width('100%')
    .height('100%')
  }

  private cpuDetail(): string {
    const m = this.m();
    if (!m) {
      return '';
    }
    const cores = Number(m.cpu_cores ?? 0);
    if (cores > 0) {
      const usedCores = (Number(m.cpu_percent ?? 0) / 100) * cores;
      return `${usedCores.toFixed(1)} / ${cores} 核`;
    }
    if ((m.load1 ?? 0) > 0) {
      return `负载 ${Number(m.load1).toFixed(2)}`;
    }
    return '';
  }

  private memDetail(): string {
    const m = this.m();
    if (!m || !(m.mem_total && m.mem_total > 0)) {
      return '';
    }
    const used = (m.mem_used && m.mem_used > 0)
      ? m.mem_used
      : Math.round((Number(m.mem_percent ?? 0) / 100) * m.mem_total);
    return `${FormatUtil.bytes(used)} / ${FormatUtil.bytes(m.mem_total)}`;
  }

  private swapDetail(): string {
    const m = this.m();
    if (!m || !(m.swap_total && m.swap_total > 0)) {
      return '';
    }
    const used = (m.swap_used && m.swap_used > 0)
      ? m.swap_used
      : Math.round((Number(m.swap_percent ?? 0) / 100) * m.swap_total);
    return `${FormatUtil.bytes(used)} / ${FormatUtil.bytes(m.swap_total)}`;
  }

  private diskDetail(): string {
    const m = this.m();
    if (!m || !(m.disk_total && m.disk_total > 0)) {
      return '';
    }
    const used = (m.disk_used && m.disk_used > 0)
      ? m.disk_used
      : Math.round((Number(m.disk_percent ?? 0) / 100) * m.disk_total);
    return `${FormatUtil.bytes(used)} / ${FormatUtil.bytes(m.disk_total)}`;
  }

  /** Android：仅统计带 GPU 的历史采样数 */
  private gpuTrendCount(): number {
    let n = 0;
    for (let i = 0; i < this.history.length; i++) {
      if ((this.history[i].gpus ?? []).length > 0) {
        n++;
      }
    }
    return n;
  }

  @Builder
  trendBox(label: string, pts: ChartPoint[], color: string, key: string) {
    Column() {
      Row() {
        Text(label)
          .fontSize(AppTheme.fontTiny)
          .fontColor(this.tMuted)
        Blank()
        if (pts.length > 0) {
          Text(`${this.last(pts).toFixed(1)}%`)
            .fontSize(AppTheme.fontTiny)
            .fontWeight(FontWeight.Bold)
            .fontColor(color)
            .margin({ right: 4 })
        }
        Text('⤢')
          .fontSize(AppTheme.fontTiny)
          .fontColor(this.tMuted)
          .opacity(0.5)
      }
      .width('100%')
      .margin({ bottom: 4 })

      HistoryChart({
        title: '',
        unit: '%',
        color: color,
        points: pts,
        heightVp: 100,
        interactive: false
      })
        .layoutWeight(1)
        .width('100%')
    }
    .layoutWeight(1)
    .padding(4)
    .alignItems(HorizontalAlign.Start)
    .onClick(() => {
      this.expandMetric = key;
    })
  }
}
