import { Api } from '../data/Api';
import { ApiClient } from '../data/ApiClient';
import {
  AiCallStat,
  AiStatsResponse,
  AiTaskAgg,
  AiUsageHistoryPoint,
  AiUserUsageRow
} from '../data/models/Models';
import {
  ErrorBanner,
  FilterChips,
  PageHeader
} from '../components/CommonUI';
import { ChartPoint, HistoryChart } from '../components/HistoryChart';
import { NavUtil } from '../common/NavUtil';
import { AppTheme } from '../common/Routes';


interface TaskRow {
  task: string;
  count: number;
  fail: number;
  avgMs: number;
}

interface MetricPair {
  label: string;
  value: string;
}

/**
 * Android CostManagementScreen 1:1
 * GET ai/stats · ai/usage/history · ai/usage/by-user
 */
@Entry
@Component
struct CostManagementPage {
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

  /** 0=1天 1=7天 2=30天 3=90天 — 对齐 Android 近 24h / 7 / 30 / 90 */
  @State daysIdx: number = 1;
  private dayOptions: number[] = [1, 7, 30, 90];
  private dayLabels: string[] = ['近 24h', '近 7 天', '近 30 天', '近 90 天'];

  @State loading: boolean = true;
  @State error: string = '';
  @State stats: AiStatsResponse | null = null;
  @State points: AiUsageHistoryPoint[] = [];
  @State users: AiUserUsageRow[] = [];
  @State currency: string = 'CNY';

  aboutToAppear(): void {
    void this.reload();
  }

  private selectedDays(): number {
    return this.dayOptions[this.daysIdx] || 7;
  }

  private async reload(): Promise<void> {
    this.loading = true;
    this.error = '';
    const days = this.selectedDays();
    const to = Math.floor(Date.now() / 1000);
    const from = to - days * 86400;
    try {
      const s = await Api.aiStats(days);
      const h = await Api.aiUsageHistory(from, to);
      const u = await Api.aiUsageByUser(from, to, 15);
      this.stats = s;
      this.points = h.points ?? [];
      this.users = u.users ?? [];
      const c1 = (s.cost_currency || '').trim();
      const c2 = (h.cost_currency || '').trim();
      const c3 = (u.cost_currency || '').trim();
      this.currency = c1 || c2 || c3 || 'CNY';
    } catch (e) {
      this.error = ApiClient.formatError(e as Object);
      if (!this.stats) {
        this.stats = null;
      }
    } finally {
      this.loading = false;
    }
  }

  private totalCalls(): number {
    const s = this.stats;
    if (!s) {
      return 0;
    }
    return Number(s.total ?? s.total_calls ?? 0);
  }

  private failRatePct(): string {
    const s = this.stats;
    if (!s) {
      return '0.0';
    }
    const total = this.totalCalls();
    if (total <= 0) {
      return '0.0';
    }
    const rate = Number(s.fail_rate ?? 0) * 100;
    return rate.toFixed(1);
  }

  private avgLatency(): number {
    return Number(this.stats?.avg_latency_ms ?? 0);
  }

  private tokensTotal(): number {
    const s = this.stats;
    if (!s) {
      return 0;
    }
    return Number(s.approx_tokens_total ?? s.total_tokens ?? 0);
  }

  private costTotal(): number {
    const s = this.stats;
    if (!s) {
      return 0;
    }
    return Number(s.cost_total ?? s.total_cost ?? 0);
  }

  private money(n: number): string {
    return `${n.toFixed(4)} ${this.currency}`;
  }

  private storageLabel(): string {
    return this.stats?.persisted === true ? 'PostgreSQL' : '进程内';
  }

  private metricPairs(): MetricPair[] {
    return [
      { label: '调用次数', value: `${this.totalCalls()}` },
      { label: '失败率', value: `${this.failRatePct()}%` },
      { label: '平均延迟', value: `${this.avgLatency()} ms` },
      { label: 'Token', value: `${this.tokensTotal()}` },
      { label: '估算费用', value: this.money(this.costTotal()) },
      { label: '存储', value: this.storageLabel() }
    ];
  }

  private pointTs(p: AiUsageHistoryPoint): number {
    return Number(p.timestamp ?? p.ts ?? 0);
  }

  private chartOf(kind: string): ChartPoint[] {
    const out: ChartPoint[] = [];
    for (let i = 0; i < this.points.length; i++) {
      const p = this.points[i];
      let v = 0;
      if (kind === 'calls') {
        v = Number(p.calls ?? 0);
      } else if (kind === 'tokens') {
        v = Number(p.tokens ?? 0);
      } else if (kind === 'latency') {
        v = Number(p.avg_latency_ms ?? 0);
      } else {
        v = Number(p.cost ?? 0);
      }
      out.push({ t: this.pointTs(p), v: v });
    }
    return out;
  }

  private taskRows(): TaskRow[] {
    const raw = this.stats?.by_task;
    if (!raw) {
      return [];
    }
    const keys: string[] = Object.keys(raw as Object);
    const rows: TaskRow[] = [];
    for (let i = 0; i < keys.length; i++) {
      const task = keys[i];
      const agg: AiTaskAgg | undefined = (raw as Record<string, AiTaskAgg>)[task];
      if (!agg) {
        continue;
      }
      rows.push({
        task: task,
        count: Number(agg.count ?? 0),
        fail: Number(agg.fail ?? 0),
        avgMs: Number(agg.avg_ms ?? 0)
      });
    }
    rows.sort((a: TaskRow, b: TaskRow) => b.count - a.count);
    return rows;
  }

  private recentCalls(): AiCallStat[] {
    const list = this.stats?.recent ?? [];
    return list.slice(0, 10);
  }

  private formatCallTime(ts: number): string {
    if (!ts || ts <= 0) {
      return '-';
    }
    const d = new Date(ts * 1000);
    const mm = this.pad2(d.getMonth() + 1);
    const dd = this.pad2(d.getDate());
    const hh = this.pad2(d.getHours());
    const mi = this.pad2(d.getMinutes());
    return `${mm}-${dd} ${hh}:${mi}`;
  }

  private pad2(n: number): string {
    return n < 10 ? `0${n}` : `${n}`;
  }

  private userActor(u: AiUserUsageRow): string {
    const a = (u.actor || u.username || '').trim();
    return a.length > 0 ? a : '(system)';
  }

  build() {
    Column() {
      PageHeader({
        title: '成本管理',
        showBack: true,
        onBack: () => NavUtil.back(),
        trailingIcon: 'refresh',
        showLoading: this.loading,
        onTrailing: () => {
          if (!this.loading) {
            void this.reload();
          }
        }
      })

      if (this.error && !this.stats) {
        ErrorBanner({
          message: this.error,
          showRetry: true,
          onRetry: () => {
            void this.reload();
          }
        })
      } else if (this.error) {
        ErrorBanner({ message: this.error })
      }

      Scroll() {
        Column({ space: 14 }) {
          Text('调用次数 · 延迟 · Token · 费用（与 Web AI 观测同步，PostgreSQL 永久保存）')
            .fontSize(AppTheme.fontCaption)
            .fontColor(this.tMuted)
            .width('100%')
            .lineHeight(18)

          FilterChips({
            labels: this.dayLabels,
            selected: $daysIdx,
            variant: 'filter',
            onSelect: (_i: number) => {
              void this.reload();
            }
          })

          if (this.loading && !this.stats) {
            Column() {
              LoadingProgress().width(28).height(28).color(this.tAccent)
            }
            .width('100%')
            .height(160)
            .justifyContent(FlexAlign.Center)
          } else if (this.stats || !this.error) {
            // 指标 2 列
            Flex({ wrap: FlexWrap.Wrap }) {
              ForEach(this.metricPairs(), (m: MetricPair, i: number) => {
                Column() {
                  Text(m.label)
                    .fontSize(AppTheme.fontMicro)
                    .fontColor(this.tMuted)
                  Text(m.value)
                    .fontSize(AppTheme.fontSubtitle)
                    .fontWeight(FontWeight.Bold)
                    .fontColor(this.tText)
                    .margin({ top: 4 })
                    .maxLines(2)
                    .textOverflow({ overflow: TextOverflow.Ellipsis })
                }
                .width('48%')
                .padding({ left: 12, right: 12, top: 10, bottom: 10 })
                .margin({ right: i % 2 === 0 ? '4%' : 0, bottom: 10 })
                .backgroundColor(this.tCard)
                .borderRadius(AppTheme.rSm)
                .alignItems(HorizontalAlign.Start)
              }, (m: MetricPair, i: number) => `m-${i}-${m.label}`)
            }
            .width('100%')

            if (this.points.length > 0) {
              Text('历史趋势')
                .fontSize(AppTheme.fontBody)
                .fontWeight(FontWeight.Medium)
                .fontColor(this.tText)
                .width('100%')

              HistoryChart({
                title: '调用次数',
                unit: '次',
                color: '#4C8DFF',
                points: this.chartOf('calls'),
                heightVp: 120
              })
              HistoryChart({
                title: 'Token',
                unit: 'tok',
                color: '#22C55E',
                points: this.chartOf('tokens'),
                heightVp: 120
              })
              HistoryChart({
                title: '平均延迟',
                unit: 'ms',
                color: '#EAB308',
                points: this.chartOf('latency'),
                heightVp: 120
              })
              HistoryChart({
                title: `费用 (${this.currency})`,
                unit: this.currency,
                color: '#F97316',
                points: this.chartOf('cost'),
                heightVp: 120
              })
            } else {
              Text('暂无历史曲线（完成若干 AI 调用后出现）')
                .fontSize(AppTheme.fontCaption)
                .fontColor(this.tMuted)
                .width('100%')
            }

            if (this.users.length > 0) {
              Column({ space: 4 }) {
                Text('用户成本排行')
                  .fontSize(AppTheme.fontBody)
                  .fontWeight(FontWeight.Medium)
                  .fontColor(this.tText)
                  .width('100%')
                  .margin({ bottom: 4 })
                ForEach(this.users, (u: AiUserUsageRow, idx: number) => {
                  Row() {
                    Column() {
                      Text(this.userActor(u))
                        .fontSize(AppTheme.fontSmall)
                        .fontWeight(FontWeight.Medium)
                        .fontColor(this.tText)
                      Text(`${u.calls ?? 0} 次 · ${u.tokens ?? 0} tok`)
                        .fontSize(AppTheme.fontMicro)
                        .fontColor(this.tMuted)
                        .margin({ top: 2 })
                    }
                    .alignItems(HorizontalAlign.Start)
                    .layoutWeight(1)
                    Text(this.money(Number(u.cost ?? 0)))
                      .fontSize(AppTheme.fontCaption)
                      .fontFamily('monospace')
                      .fontColor('#F97316')
                  }
                  .width('100%')
                  .padding({ top: 6, bottom: 6 })
                }, (u: AiUserUsageRow, idx: number) => `u-${idx}-${this.userActor(u)}`)
              }
              .width('100%')
              .padding(AppTheme.cardPad)
              .backgroundColor(this.tCard)
              .borderRadius(AppTheme.rMd)
            }

            if (this.taskRows().length > 0) {
              Column({ space: 4 }) {
                Text('按任务')
                  .fontSize(AppTheme.fontBody)
                  .fontWeight(FontWeight.Medium)
                  .fontColor(this.tText)
                  .width('100%')
                  .margin({ bottom: 4 })
                ForEach(this.taskRows(), (r: TaskRow) => {
                  Row() {
                    Text(r.task || '(unknown)')
                      .fontSize(AppTheme.fontCaption)
                      .fontFamily('monospace')
                      .fontColor(this.tText)
                      .layoutWeight(1)
                      .maxLines(1)
                      .textOverflow({ overflow: TextOverflow.Ellipsis })
                    Text(`${r.count} / 失败 ${r.fail} / ${r.avgMs}ms`)
                      .fontSize(AppTheme.fontMicro)
                      .fontColor(this.tMuted)
                  }
                  .width('100%')
                  .padding({ top: 5, bottom: 5 })
                }, (r: TaskRow) => `t-${r.task}`)
              }
              .width('100%')
              .padding(AppTheme.cardPad)
              .backgroundColor(this.tCard)
              .borderRadius(AppTheme.rMd)
            }

            if (this.recentCalls().length > 0) {
              Column({ space: 4 }) {
                Text('最近调用')
                  .fontSize(AppTheme.fontBody)
                  .fontWeight(FontWeight.Medium)
                  .fontColor(this.tText)
                  .width('100%')
                  .margin({ bottom: 4 })
                ForEach(this.recentCalls(), (r: AiCallStat, idx: number) => {
                  Column() {
                    Row() {
                      Text(r.ok === false ? 'FAIL' : 'OK')
                        .fontSize(AppTheme.fontMicro)
                        .fontWeight(FontWeight.Bold)
                        .fontColor(r.ok === false ? this.tDanger : '#22C55E')
                      Text(r.task || '-')
                        .fontSize(AppTheme.fontCaption)
                        .fontFamily('monospace')
                        .fontColor(this.tText)
                        .margin({ left: 8 })
                        .layoutWeight(1)
                        .maxLines(1)
                        .textOverflow({ overflow: TextOverflow.Ellipsis })
                      Text(this.formatCallTime(Number(r.ts ?? 0)))
                        .fontSize(AppTheme.fontMicro)
                        .fontColor(this.tMuted)
                    }
                    .width('100%')
                    Text(
                      `${r.latency_ms ?? 0}ms · ≈${r.approx_tokens ?? 0} tok` +
                        ((r.actor || '').trim() ? ` · ${r.actor}` : '') +
                        ((r.error || '').trim() ? ` · ${r.error}` : '')
                    )
                      .fontSize(AppTheme.fontMicro)
                      .fontColor(this.tMuted)
                      .margin({ top: 2 })
                      .width('100%')
                  }
                  .width('100%')
                  .padding({ top: 4, bottom: 4 })
                  .alignItems(HorizontalAlign.Start)
                }, (r: AiCallStat, idx: number) => `r-${idx}-${r.ts ?? 0}-${r.task ?? ''}`)
              }
              .width('100%')
              .padding(AppTheme.cardPad)
              .backgroundColor(this.tCard)
              .borderRadius(AppTheme.rMd)
            }
          }

          Blank().height(24)
        }
        .width('100%')
        .padding(AppTheme.pagePadH)
      }
      .layoutWeight(1)
      .scrollBar(BarState.Off)
      .align(Alignment.Top)
    }
    .width('100%')
    .height('100%')
    .backgroundColor(this.themeRev >= 0 ? this.tBg : this.tBg)
  }
}
