import { Api } from '../data/Api';
import { ApiClient } from '../data/ApiClient';
import {
  Dashboard,
  DashboardPanel,
  DashLogLine,
  DashTarget,
  DashVar,
  PanelInstantResponse,
  PanelLogsResponse,
  PanelQueryReq,
  PanelRangeResponse,
  VarValuesReq
} from '../data/models/Models';
import {
  EmptyHint,
  ErrorBanner,
  KeyValueRow,
  LoadingBlock,
  PageHeader,
  SectionHeader
} from '../components/CommonUI';
import { ChartPoint, HistoryChart } from '../components/HistoryChart';
import { NavUtil } from '../common/NavUtil';
import { AppTheme } from '../common/Routes';


interface VarChip {
  name: string;
  label: string;
  type: string;
  options: string[];
  selected: string;
}

interface PanelView {
  key: string;
  title: string;
  type: string;
  expr: string;
  valueText: string;
  rows: string[];
  points: ChartPoint[];
  error: string;
  kind: string; // graph | stat | table | logs | text | other
}

@Entry
@Component
struct DashboardViewPage {
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

  @State dashboardId: string = '';
  @State title: string = '仪表盘';
  @State panels: PanelView[] = [];
  @State varChips: VarChip[] = [];
  @State error: string = '';
  @State loading: boolean = false;
  @State rangeIdx: number = 1; // 0=1h 1=6h 2=24h 3=3d 4=7d
  @State openVarName: string = '';
  private rangeLabels: string[] = ['1小时', '6小时', '24小时', '3天', '7天'];
  private datasource: string = '';
  private selectedVars: Record<string, string> = {};
  private rawPanels: DashboardPanel[] = [];

  aboutToAppear(): void {
    this.dashboardId = NavUtil.paramString('dashboardId', '') || NavUtil.paramString('id', '');
    void this.load();
  }

  private rangeSeconds(): number {
    if (this.rangeIdx === 0) {
      return 3600;
    }
    if (this.rangeIdx === 1) {
      return 6 * 3600;
    }
    if (this.rangeIdx === 2) {
      return 24 * 3600;
    }
    if (this.rangeIdx === 3) {
      return 3 * 24 * 3600;
    }
    return 7 * 24 * 3600;
  }

  private targetExprs(targets?: DashTarget[]): string[] {
    const out: string[] = [];
    if (!targets) {
      return out;
    }
    for (let i = 0; i < targets.length; i++) {
      const e = (targets[i].expr || '').trim();
      if (e) {
        out.push(e);
      }
    }
    return out;
  }

  private pickField(obj: ESObject, keys: string[]): string {
    const text = JSON.stringify(obj);
    for (let i = 0; i < keys.length; i++) {
      const got = this.pickJsonString(text, keys[i]);
      if (got) {
        return got;
      }
    }
    return '';
  }

  private labelsName(labels: ESObject | undefined): string {
    if (!labels) {
      return '';
    }
    const text = JSON.stringify(labels);
    const prefer = ['__name__', 'job', 'instance', 'host', 'hostname'];
    for (let i = 0; i < prefer.length; i++) {
      const got = this.pickJsonString(text, prefer[i]);
      if (got) {
        return got;
      }
    }
    return '';
  }

  private pickJsonString(jsonText: string, key: string): string {
    const needle = `"${key}"`;
    const idx = jsonText.indexOf(needle);
    if (idx < 0) {
      return '';
    }
    let i = idx + needle.length;
    while (i < jsonText.length && (jsonText.charAt(i) === ' ' || jsonText.charAt(i) === ':')) {
      i++;
    }
    if (i >= jsonText.length) {
      return '';
    }
    if (jsonText.charAt(i) === '"') {
      i++;
      let out = '';
      while (i < jsonText.length && jsonText.charAt(i) !== '"') {
        if (jsonText.charAt(i) === '\\' && i + 1 < jsonText.length) {
          i++;
        }
        out += jsonText.charAt(i);
        i++;
      }
      return out;
    }
    let out = '';
    while (i < jsonText.length && jsonText.charAt(i) !== ',' && jsonText.charAt(i) !== '}') {
      out += jsonText.charAt(i);
      i++;
    }
    return out.trim();
  }

  private seriesToPoints(seriesItem: ESObject): ChartPoint[] {
    const text = JSON.stringify(seriesItem);
    const points: ChartPoint[] = [];
    // 优先解析 "points":[[ts,val],...] 或 "values":[[ts,"val"],...]
    const keys = ['points', 'values'];
    for (let k = 0; k < keys.length; k++) {
      const arrStartKey = `"${keys[k]}"`;
      const keyIdx = text.indexOf(arrStartKey);
      if (keyIdx < 0) {
        continue;
      }
      let i = text.indexOf('[', keyIdx + arrStartKey.length);
      if (i < 0) {
        continue;
      }
      // 扫描成对 [ts, val]
      let depth = 0;
      let pairStart = -1;
      for (; i < text.length; i++) {
        const ch = text.charAt(i);
        if (ch === '[') {
          depth++;
          if (depth === 2) {
            pairStart = i + 1;
          }
        } else if (ch === ']') {
          if (depth === 2 && pairStart >= 0) {
            const pair = text.substring(pairStart, i);
            const parts = pair.split(',');
            if (parts.length >= 2) {
              let t = Number((parts[0] || '').replace(/"/g, '').trim());
              const v = Number((parts[1] || '').replace(/"/g, '').trim());
              if (t > 1e12) {
                t = Math.floor(t / 1000);
              }
              if (t > 0 && !isNaN(v)) {
                points.push({ t: t, v: v });
              }
            }
            pairStart = -1;
          }
          depth--;
          if (depth <= 0) {
            break;
          }
        }
      }
      if (points.length > 0) {
        return points;
      }
    }
    return points;
  }

  private panelKind(type: string): string {
    const t = (type || '').toLowerCase().trim();
    if (t === 'text') {
      return 'text';
    }
    if (t === 'logs') {
      return 'logs';
    }
    if (t === 'table') {
      return 'table';
    }
    if (t === 'timeseries' || t === 'graph' || t === 'state-timeline' || t === 'statetimeline' ||
      t === 'heatmap' || t === 'line' || t === 'area') {
      return 'graph';
    }
    if (t === 'stat') {
      return 'stat';
    }
    if (t === 'gauge' || t === 'bargauge' || t === 'piechart' || t === 'pie' ||
      t === 'barchart' || t === 'bar' || t === 'histogram') {
      return 'instant';
    }
    if (t === 'unsupported' || t === 'alertlist') {
      return 'other';
    }
    return 'other';
  }

  private displayOpt(o: string): string {
    if (o === '$__all' || o.toLowerCase() === 'all') {
      return '全部';
    }
    return o;
  }

  private async initVars(d: Dashboard): Promise<void> {
    const vars = (d.vars ?? []).filter((v: DashVar) => (v.type || '') !== 'constant');
    const chips: VarChip[] = [];
    const sel: Record<string, string> = {};
    for (let i = 0; i < vars.length; i++) {
      const v = vars[i];
      let opts: string[] = [];
      let selected = v.current || '';
      if ((v.type || '') === 'custom') {
        opts = v.options ? v.options.slice() : [];
        if (v.include_all) {
          opts = ['$__all'].concat(opts);
        }
        if (!selected) {
          selected = v.include_all ? '$__all' : (opts.length > 0 ? opts[0] : '');
        }
      } else if ((v.type || '') === 'query') {
        selected = v.current || (v.include_all ? '$__all' : '');
        try {
          const req: VarValuesReq = {
            name: v.name,
            label: v.label,
            type: v.type,
            query: v.query,
            options: v.options,
            current: v.current,
            multi: v.multi,
            include_all: v.include_all,
            datasource: d.datasource || ''
          };
          const res = await Api.dashboardVarValues(req);
          opts = res.values ?? [];
          if (v.include_all) {
            opts = ['$__all'].concat(opts);
          }
          if (!selected && opts.length > 0) {
            selected = opts[0];
          }
        } catch (_e) {
          opts = [];
        }
      } else {
        selected = v.current || '';
        opts = v.options ? v.options.slice() : [];
      }
      sel[v.name] = selected;
      chips.push({
        name: v.name,
        label: v.label || v.name,
        type: v.type || '',
        options: opts,
        selected: selected
      });
    }
    this.selectedVars = sel;
    this.varChips = chips;
  }

  private async setVar(name: string, value: string): Promise<void> {
    if (this.selectedVars[name] === value) {
      this.openVarName = '';
      return;
    }
    const next: Record<string, string> = {};
    const keys = Object.keys(this.selectedVars);
    for (let i = 0; i < keys.length; i++) {
      next[keys[i]] = this.selectedVars[keys[i]];
    }
    next[name] = value;
    this.selectedVars = next;
    const chips = this.varChips.slice();
    for (let i = 0; i < chips.length; i++) {
      if (chips[i].name === name) {
        chips[i] = {
          name: chips[i].name,
          label: chips[i].label,
          type: chips[i].type,
          options: chips[i].options,
          selected: value
        };
      }
    }
    this.varChips = chips;
    this.openVarName = '';
    await this.queryAll();
  }

  private async queryPanel(p: DashboardPanel, index: number): Promise<PanelView> {
    const title = p.title || `面板 ${index + 1}`;
    const type = p.type || 'stat';
    const kind = this.panelKind(type);
    const exprs = this.targetExprs(p.targets);
    const empty: PanelView = {
      key: `${index}-${p.id ?? title}`,
      title: title,
      type: type,
      expr: exprs.join(' | '),
      valueText: '',
      rows: [],
      points: [],
      error: '',
      kind: kind
    };
    if (kind === 'text') {
      empty.valueText = p.text || '(空)';
      return empty;
    }
    if (kind === 'other') {
      empty.valueText = type === 'unsupported'
        ? `暂不支持：${p.raw_type || type}`
        : `移动端暂不支持「${p.raw_type || type}」`;
      return empty;
    }
    if (exprs.length === 0) {
      empty.error = kind === 'logs' ? '未配置 LogQL 查询' : '未配置查询';
      return empty;
    }
    const to = Math.floor(Date.now() / 1000);
    const from = to - this.rangeSeconds();
    const ds = (p.datasource || this.datasource || '').trim();
    try {
      if (kind === 'logs') {
        const allLines: string[] = [];
        let available = true;
        let err = '';
        for (let i = 0; i < exprs.length; i++) {
          const req: PanelQueryReq = {
            expr: exprs[i],
            from: from,
            to: to,
            vars: this.selectedVars,
            datasource: ds,
            limit: 200
          };
          const res: PanelLogsResponse = await Api.dashboardQueryLogs(req);
          if (res.available === false) {
            available = false;
          }
          if (res.error) {
            err = res.error;
          }
          const lines: DashLogLine[] = res.lines ?? [];
          for (let j = 0; j < lines.length; j++) {
            const ln = lines[j];
            const ts = ln.ts_ms ? new Date(ln.ts_ms).toLocaleTimeString() : '';
            allLines.push(`${ts} ${ln.line || ''}`.trim());
          }
        }
        if (!available) {
          empty.error = '该面板需选择一个 Loki 数据源';
          return empty;
        }
        if (err && allLines.length === 0) {
          empty.error = err;
          return empty;
        }
        empty.rows = allLines.slice(0, 80);
        empty.valueText = allLines.length > 0 ? `${allLines.length} 条日志` : '该范围无日志';
        empty.kind = 'logs';
        return empty;
      }

      if (kind === 'graph' || kind === 'stat') {
        const allPoints: ChartPoint[] = [];
        const rows: string[] = [];
        for (let i = 0; i < exprs.length; i++) {
          const req: PanelQueryReq = {
            expr: exprs[i],
            from: from,
            to: to,
            vars: this.selectedVars,
            datasource: ds
          };
          const res: PanelRangeResponse = await Api.dashboardQuery(req);
          if (res.available === false) {
            empty.error = res.error || '数据源不可用';
            return empty;
          }
          if (res.error) {
            empty.error = res.error;
            return empty;
          }
          const series: ESObject[] = res.series ? res.series : [];
          for (let s = 0; s < series.length; s++) {
            const pts = this.seriesToPoints(series[s]);
            if (pts.length > 0 && allPoints.length === 0) {
              for (let pi = 0; pi < pts.length; pi++) {
                allPoints.push(pts[pi]);
              }
            }
            const labelsText = JSON.stringify(series[s]);
            const name = this.pickJsonString(labelsText, '__name__') ||
              this.pickJsonString(labelsText, 'job') ||
              this.pickJsonString(labelsText, 'instance') ||
              this.pickJsonString(labelsText, 'host') ||
              `系列 ${s + 1}`;
            const last = pts.length > 0 ? pts[pts.length - 1].v : undefined;
            rows.push(last !== undefined ? `${name}: ${last}` : name);
          }
        }
        empty.points = allPoints;
        empty.rows = rows.slice(0, 8);
        if (kind === 'stat') {
          if (allPoints.length > 0) {
            empty.valueText = `${allPoints[allPoints.length - 1].v}`;
          } else if (rows.length > 0) {
            empty.valueText = rows[0];
          } else {
            empty.valueText = '无数据';
          }
        } else {
          empty.valueText = allPoints.length > 0 ? `${allPoints.length} 点` : (rows.length > 0 ? `${rows.length} 个系列` : '无数据点');
        }
        return empty;
      }

      // instant / table
      const rows: string[] = [];
      let firstVal = '';
      for (let i = 0; i < exprs.length; i++) {
        const req: PanelQueryReq = {
          expr: exprs[i],
          from: from,
          to: to,
          vars: this.selectedVars,
          datasource: ds
        };
        const inst: PanelInstantResponse = await Api.dashboardQueryInstant(req);
        if (inst.available === false) {
          empty.error = inst.error || '数据源不可用';
          return empty;
        }
        if (inst.error) {
          empty.error = inst.error;
          return empty;
        }
        if (inst.value !== undefined && inst.value !== null) {
          firstVal = `${inst.value}`;
          rows.push(firstVal);
          continue;
        }
        const series: ESObject[] = inst.series ? inst.series : (inst.values ? inst.values : []);
        for (let s = 0; s < Math.min(series.length, 20); s++) {
          const serText = JSON.stringify(series[s]);
          const name = this.pickJsonString(serText, '__name__') ||
            this.pickJsonString(serText, 'job') ||
            this.pickJsonString(serText, 'instance') ||
            `系列 ${s + 1}`;
          const val = this.pickJsonString(serText, 'value') ||
            this.pickJsonString(serText, 'Value') ||
            this.pickJsonString(serText, 'v') ||
            this.pickJsonString(serText, 'last') || '—';
          if (!firstVal) {
            firstVal = val;
          }
          rows.push(`${name}: ${val}`);
        }
      }
      empty.rows = rows;
      empty.valueText = firstVal || (rows.length > 0 ? `${rows.length} 行` : '无数据点');
      empty.kind = kind === 'table' ? 'table' : 'instant';
      return empty;
    } catch (e) {
      empty.error = ApiClient.formatError(e as Object);
      return empty;
    }
  }

  private async queryAll(): Promise<void> {
    const raw = this.rawPanels;
    const views: PanelView[] = [];
    for (let i = 0; i < raw.length; i++) {
      views.push(await this.queryPanel(raw[i], i));
    }
    this.panels = views;
  }

  private async load(): Promise<void> {
    if (!this.dashboardId) {
      this.error = '缺少仪表盘 ID';
      return;
    }
    this.loading = true;
    this.error = '';
    try {
      const d: Dashboard = await Api.dashboard(this.dashboardId);
      this.title = d.name || d.title || d.id;
      this.datasource = d.datasource || '';
      this.rawPanels = d.panels ?? [];
      await this.initVars(d);
      await this.queryAll();
    } catch (e) {
      this.error = ApiClient.formatError(e as Object);
    } finally {
      this.loading = false;
    }
  }

  @Builder
  varPicker(chip: VarChip) {
    Column() {
      Text(`${chip.label}: ${this.displayOpt(chip.selected) || '—'}`)
        .fontSize(AppTheme.fontCaption)
        .fontColor(this.openVarName === chip.name ? this.tOnAccent : this.tText)
        .padding({ left: 12, right: 12, top: 6, bottom: 6 })
        .backgroundColor(this.openVarName === chip.name ? this.tAccent : this.tElevated)
        .borderRadius(AppTheme.rSm)
        .onClick(() => {
          this.openVarName = this.openVarName === chip.name ? '' : chip.name;
        })
      if (this.openVarName === chip.name) {
        Column() {
          if (chip.options.length === 0) {
            Text('无候选值')
              .fontSize(AppTheme.fontCaption)
              .fontColor(this.tMuted)
              .padding(AppTheme.spaceSm)
          }
          ForEach(chip.options, (opt: string) => {
            Text(this.displayOpt(opt))
              .fontSize(AppTheme.fontSmall)
              .fontColor(opt === chip.selected ? this.tAccent : this.tText)
              .width('100%')
              .padding({ left: 10, right: 10, top: 8, bottom: 8 })
              .onClick(() => {
                void this.setVar(chip.name, opt);
              })
          }, (opt: string) => `${chip.name}-${opt}`)
        }
        .width(180)
        .constraintSize({ maxHeight: 220 })
        .backgroundColor(this.tCard)
        .borderRadius(AppTheme.rXs)
        .border({ width: 1, color: this.tBorderSoft })
        .margin({ top: 4 })
      }
    }
    .alignItems(HorizontalAlign.Start)
  }

  build() {
    Column() {
      PageHeader({
        title: this.title,
        showBack: true,
        onBack: () => NavUtil.back(),
        trailing: '刷新',
        onTrailing: () => {
          void this.queryAll();
        }
      })
      Text(`${this.panels.length} 个面板 · 只读`)
        .fontSize(AppTheme.fontCaption)
        .fontColor(this.tMuted)
        .padding({ left: 16, bottom: 6 })
      ErrorBanner({ message: this.error })

      Scroll() {
        Row({ space: 8 }) {
          ForEach(this.rangeLabels, (lab: string, i: number) => {
            Text(lab)
              .fontSize(AppTheme.fontCaption)
              .fontColor(this.rangeIdx === i ? this.tOnAccent : this.tMuted)
              .padding({ left: 12, right: 12, top: 6, bottom: 6 })
              .backgroundColor(this.rangeIdx === i ? this.tAccent : this.tElevated)
              .borderRadius(AppTheme.rSm)
              .onClick(() => {
                this.rangeIdx = i;
                void this.queryAll();
              })
          }, (lab: string, i: number) => `rg-${i}`)
        }
      }
      .scrollable(ScrollDirection.Horizontal)
      .scrollBar(BarState.Off)
      .padding({ left: 16, right: 16, bottom: 8 })

      if (this.varChips.length > 0) {
        Scroll() {
          Row({ space: 8 }) {
            ForEach(this.varChips, (c: VarChip) => {
              this.varPicker(c)
            }, (c: VarChip) => c.name)
          }
        }
        .scrollable(ScrollDirection.Horizontal)
        .scrollBar(BarState.Off)
        .padding({ left: 16, right: 16, bottom: 8 })
      }

      if (this.loading && this.panels.length === 0) {
        LoadingBlock({ text: '查询面板…' })
      } else if (!this.loading && this.panels.length === 0) {
        EmptyHint({ text: '暂无面板' })
      }
      Scroll() {
        Column() {
          ForEach(this.panels, (p: PanelView) => {
            Column() {
              SectionHeader({ title: p.title })
              KeyValueRow({ label: '类型', value: p.type || '—' })
                .padding({ left: 16, right: 16 })
              if (p.expr && p.kind !== 'text') {
                Text(p.expr)
                  .fontSize(AppTheme.fontMicro)
                  .fontFamily('monospace')
                  .fontColor(this.tMuted)
                  .padding({ left: 16, right: 16, top: 4 })
                  .maxLines(2)
                  .textOverflow({ overflow: TextOverflow.Ellipsis })
              }
              if (p.error) {
                Text(p.error)
                  .fontSize(AppTheme.fontCaption)
                  .fontColor(this.tDanger)
                  .padding({ left: 16, right: 16, top: 6, bottom: 8 })
              } else if (p.kind === 'text') {
                Text(p.valueText || '—')
                  .fontSize(AppTheme.fontSmall)
                  .fontColor(this.tText)
                  .padding({ left: 16, right: 16, top: 8, bottom: 12 })
              } else if (p.kind === 'graph' && p.points.length > 1) {
                HistoryChart({
                  title: '',
                  unit: '',
                  color: this.tAccent,
                  points: p.points,
                  heightVp: 110
                })
                  .padding({ left: 12, right: 12, bottom: 8 })
                if (p.rows.length > 0) {
                  ForEach(p.rows.slice(0, 4), (line: string, ri: number) => {
                    Text(line)
                      .fontSize(AppTheme.fontMicro)
                      .fontColor(this.tMuted)
                      .padding({ left: 16, right: 16, top: 2 })
                  }, (line: string, ri: number) => `${p.key}-leg-${ri}`)
                  Blank().height(8)
                }
              } else if (p.kind === 'stat') {
                Text(p.valueText || '—')
                  .fontSize(AppTheme.fontHero)
                  .fontWeight(FontWeight.Bold)
                  .fontColor(this.tAccent)
                  .padding({ left: 16, right: 16, top: 8, bottom: 8 })
                if (p.points.length > 1) {
                  HistoryChart({
                    title: '',
                    unit: '',
                    color: this.tAccent,
                    points: p.points,
                    heightVp: 56
                  })
                    .padding({ left: 12, right: 12, bottom: 8 })
                }
              } else if (p.kind === 'logs' || p.kind === 'table' || p.rows.length > 0) {
                ForEach(p.rows, (line: string, ri: number) => {
                  if (p.kind === 'logs') {
                    Text(line)
                      .fontSize(AppTheme.fontMicro)
                      .fontFamily('monospace')
                      .fontColor(this.tText)
                      .padding({ left: 16, right: 16, top: 3 })
                      .maxLines(3)
                      .textOverflow({ overflow: TextOverflow.Ellipsis })
                  } else {
                    Text(line)
                      .fontSize(AppTheme.fontSmall)
                      .fontColor(this.tText)
                      .padding({ left: 16, right: 16, top: 3 })
                      .maxLines(3)
                      .textOverflow({ overflow: TextOverflow.Ellipsis })
                  }
                }, (line: string, ri: number) => `${p.key}-r-${ri}`)
                if (p.rows.length === 0) {
                  Text(p.valueText || '—')
                    .fontSize(AppTheme.fontSmall)
                    .fontColor(this.tMuted)
                    .padding({ left: 16, right: 16, top: 8, bottom: 12 })
                } else {
                  Blank().height(8)
                }
              } else {
                Text(p.valueText || '—')
                  .fontSize(AppTheme.fontDisplay)
                  .fontWeight(FontWeight.Bold)
                  .fontColor(this.tAccent)
                  .padding({ left: 16, right: 16, top: 8, bottom: 12 })
              }
            }
            .width('100%')
            .margin({ left: 16, right: 16, bottom: 10 })
            .backgroundColor(this.tCard)
            .borderRadius(AppTheme.rSm)
            .border({ width: 1, color: this.tBorderSoft })
          }, (p: PanelView) => p.key)
          Blank().height(24)
        }
      }
      .layoutWeight(1)
      .scrollBar(BarState.Off)
    }
    .width('100%')
    .height('100%')
    .backgroundColor(this.tBg)
  }
}
