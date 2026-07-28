import { AppTheme } from '../common/Routes';
import { Sample } from '../data/models/Models';

export interface ChartPoint {
  t: number;
  v: number;
}

interface ChartScaled {
  v: number;
  suffix: string;
}

@Component
export struct HistoryChart {
  @StorageLink('aiops_theme_rev') @Watch('onTheme') themeRev: number = 0;

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

  @Prop title: string = '';
  @Prop unit: string = '%';
  /** Empty falls back to theme accent (do not init from @StorageLink). */
  @Prop color: string = '';
  @Prop @Watch('onPointsChange') points: ChartPoint[] = [];
  @Prop heightVp: number = 120;
  /** When false, chart is static (no scrub/zoom gestures). */
  @Prop interactive: boolean = true;

  @State private scrubActive: boolean = false;
  @State private scrubIndex: number = -1;
  @State private scrubTimeLabel: string = '';
  @State private scrubValueLabel: string = '';
  @State private viewportTick: number = 0;
  @State private hintText: string = '拖动读数 · 双指缩放 · 双击放大';

  private settings: RenderingContextSettings = new RenderingContextSettings(true);
  private ctx: CanvasRenderingContext2D = new CanvasRenderingContext2D(this.settings);
  private canvasW: number = 0;
  private canvasH: number = 0;

  private fullStart: number = 0;
  private fullEnd: number = 1;
  private viewportStart: number = 0;
  private viewportEnd: number = 1;
  private sorted: ChartPoint[] = [];
  private pinchBaseSpan: number = 1;
  private pinchBaseCenter: number = 0;
  private lastPanX: number = 0;
  private scrubOriginX: number = 0;
  private panMode: string = 'scrub'; // 'scrub' | 'viewport'
  private pointsSig: string = '';

  private strokeColor(): string {
    return this.color.length > 0 ? this.color : this.tAccent;
  }

  aboutToAppear(): void {
    this.rebuildSorted(true);
  }

  private onTheme(): void {
    this.draw();
  }

  private onPointsChange(): void {
    this.rebuildSorted(true);
    this.draw();
  }

  private rebuildSorted(resetViewport: boolean): void {
    const raw: ChartPoint[] = this.points ?? [];
    const next: ChartPoint[] = [];
    for (let i = 0; i < raw.length; i++) {
      const t = HistoryChart.normalizeTs(raw[i].t);
      const v = raw[i].v;
      if (!(t > 0) || !HistoryChart.isFiniteNum(v)) {
        continue;
      }
      next.push({ t: t, v: v });
    }
    next.sort((a: ChartPoint, b: ChartPoint) => a.t - b.t);
    this.sorted = next;
    const sig = `${next.length}:${next.length > 0 ? next[0].t : 0}:${next.length > 0 ? next[next.length - 1].t : 0}`;
    if (next.length >= 2) {
      this.fullStart = next[0].t;
      this.fullEnd = Math.max(next[next.length - 1].t, this.fullStart + 1);
    } else {
      this.fullStart = 0;
      this.fullEnd = 1;
    }
    if (resetViewport || sig !== this.pointsSig) {
      this.viewportStart = this.fullStart;
      this.viewportEnd = this.fullEnd;
      this.scrubActive = false;
      this.scrubIndex = -1;
      this.scrubTimeLabel = '';
      this.scrubValueLabel = '';
    } else {
      this.clampViewport();
    }
    this.pointsSig = sig;
  }

  static isFiniteNum(n: number): boolean {
    return n === n && n !== Infinity && n !== -Infinity;
  }

  static normalizeTs(timestamp: number): number {
    if (!HistoryChart.isFiniteNum(timestamp) || timestamp <= 0) {
      return 0;
    }
    if (timestamp <= 999_999_999_999) {
      return timestamp * 1000;
    }
    return timestamp;
  }

  private fullSpan(): number {
    return Math.max(1, this.fullEnd - this.fullStart);
  }

  private viewSpan(): number {
    return Math.max(1, this.viewportEnd - this.viewportStart);
  }

  private minSpan(): number {
    return Math.max(this.fullSpan() / 100, 1000);
  }

  private setViewport(center: number, span: number): void {
    const full = this.fullSpan();
    const minS = Math.min(this.minSpan(), full);
    let target = Math.min(full, Math.max(minS, span));
    let start = center - target / 2;
    let end = center + target / 2;
    if (start < this.fullStart) {
      end += this.fullStart - start;
      start = this.fullStart;
    }
    if (end > this.fullEnd) {
      start -= end - this.fullEnd;
      end = this.fullEnd;
    }
    this.viewportStart = Math.max(this.fullStart, start);
    this.viewportEnd = Math.min(this.fullEnd, end);
    this.viewportTick = this.viewportTick + 1;
  }

  private clampViewport(): void {
    this.setViewport((this.viewportStart + this.viewportEnd) / 2, this.viewSpan());
  }

  private padL(): number {
    return 36;
  }

  private padR(): number {
    return 10;
  }

  private padT(): number {
    return 12;
  }

  private padB(): number {
    return 22;
  }

  private plotLeft(): number {
    return this.padL();
  }

  private plotRight(): number {
    return this.canvasW - this.padR();
  }

  private xForTs(ts: number): number {
    const left = this.plotLeft();
    const right = this.plotRight();
    const span = this.viewSpan();
    return left + ((ts - this.viewportStart) / span) * (right - left);
  }

  private tsForX(x: number): number {
    const left = this.plotLeft();
    const right = this.plotRight();
    const w = Math.max(1, right - left);
    const ratio = Math.min(1, Math.max(0, (x - left) / w));
    return this.viewportStart + ratio * this.viewSpan();
  }

  private nearestIndex(targetTs: number): number {
    const data = this.sorted;
    if (data.length === 0) {
      return -1;
    }
    let best = 0;
    let bestDist = Math.abs(data[0].t - targetTs);
    for (let i = 1; i < data.length; i++) {
      const d = Math.abs(data[i].t - targetTs);
      if (d < bestDist) {
        bestDist = d;
        best = i;
      }
    }
    return best;
  }

  private formatTimestamp(ts: number): string {
    const d = new Date(ts);
    const y = d.getFullYear();
    const mo = HistoryChart.pad2(d.getMonth() + 1);
    const day = HistoryChart.pad2(d.getDate());
    const h = HistoryChart.pad2(d.getHours());
    const mi = HistoryChart.pad2(d.getMinutes());
    const s = HistoryChart.pad2(d.getSeconds());
    return `${y}-${mo}-${day} ${h}:${mi}:${s}`;
  }

  private formatAxisTime(ts: number): string {
    const d = new Date(ts);
    const h = HistoryChart.pad2(d.getHours());
    const mi = HistoryChart.pad2(d.getMinutes());
    const s = HistoryChart.pad2(d.getSeconds());
    if (this.fullSpan() >= 86_400_000) {
      const mo = HistoryChart.pad2(d.getMonth() + 1);
      const day = HistoryChart.pad2(d.getDate());
      return `${mo}-${day} ${h}:${mi}`;
    }
    return `${h}:${mi}:${s}`;
  }

  private formatValue(value: number): string {
    const u = (this.unit || '').trim();
    if (u === '%') {
      return `${value.toFixed(1)}%`;
    }
    const scaled = HistoryChart.scaleValue(value, u);
    let num = scaled.v.toFixed(2);
    if (Math.abs(scaled.v) >= 100) {
      num = scaled.v.toFixed(0);
    } else if (Math.abs(scaled.v) >= 10) {
      num = scaled.v.toFixed(1);
    }
    if (scaled.suffix.length === 0) {
      return num;
    }
    return `${num} ${scaled.suffix}`;
  }

  private formatCompact(value: number): string {
    const u = (this.unit || '').trim();
    if (u === '%') {
      return value.toFixed(0);
    }
    const base = (u === 'B' || u === 'bytes' || u === 'Bps') ? 1024 : 1000;
    const prefixes: string[] = ['', 'K', 'M', 'G', 'T'];
    let v = value;
    let i = 0;
    while (Math.abs(v) >= base && i < prefixes.length - 1) {
      v /= base;
      i++;
    }
    const num = Math.abs(v) >= 100 ? v.toFixed(0) : v.toFixed(1);
    return num + prefixes[i];
  }

  static pad2(n: number): string {
    return n < 10 ? `0${n}` : `${n}`;
  }

  static scaleValue(value: number, unit: string): ChartScaled {
    const steps = (base: number, units: string[]): ChartScaled => {
      let v = value;
      let i = 0;
      while (Math.abs(v) >= base && i < units.length - 1) {
        v /= base;
        i++;
      }
      return { v: v, suffix: units[i] };
    };
    if (unit === 'bps') {
      return steps(1000, ['bps', 'Kbps', 'Mbps', 'Gbps', 'Tbps']);
    }
    if (unit === 'Bps') {
      return steps(1024, ['Bps', 'KBps', 'MBps', 'GBps', 'TBps']);
    }
    if (unit === 'B' || unit === 'bytes') {
      return steps(1024, ['B', 'KB', 'MB', 'GB', 'TB']);
    }
    if (unit.length === 0) {
      return { v: value, suffix: '' };
    }
    if (Math.abs(value) >= 1000) {
      const r = steps(1000, ['', 'K', 'M', 'G', 'T']);
      return { v: r.v, suffix: r.suffix + unit };
    }
    return { v: value, suffix: unit };
  }

  private applyScrubAtX(x: number): void {
    if (this.sorted.length < 2) {
      return;
    }
    const idx = this.nearestIndex(this.tsForX(x));
    if (idx < 0) {
      return;
    }
    const pt = this.sorted[idx];
    this.scrubIndex = idx;
    this.scrubActive = true;
    this.scrubTimeLabel = this.formatTimestamp(pt.t);
    this.scrubValueLabel = this.formatValue(pt.v);
    this.draw();
  }

  private zoomAt(centerTs: number, factor: number): void {
    this.setViewport(centerTs, this.viewSpan() / factor);
    this.draw();
  }

  private zoomOut(): void {
    this.setViewport((this.viewportStart + this.viewportEnd) / 2, this.viewSpan() * 1.8);
    this.draw();
  }

  private zoomIn(): void {
    this.setViewport((this.viewportStart + this.viewportEnd) / 2, this.viewSpan() / 1.8);
    this.draw();
  }

  private resetView(): void {
    this.viewportStart = this.fullStart;
    this.viewportEnd = this.fullEnd;
    this.scrubActive = false;
    this.scrubIndex = -1;
    this.scrubTimeLabel = '';
    this.scrubValueLabel = '';
    this.viewportTick = this.viewportTick + 1;
    this.draw();
  }

  private isZoomed(): boolean {
    return this.viewSpan() < this.fullSpan() * 0.98;
  }

  private draw(): void {
    const w = this.canvasW;
    const h = this.canvasH;
    if (w <= 0 || h <= 0) {
      return;
    }
    // Ensure sorted data tracks latest @Prop without missing first paint.
    if (this.sorted.length === 0 && this.points && this.points.length > 0) {
      this.rebuildSorted(true);
    }

    this.ctx.clearRect(0, 0, w, h);
    this.ctx.fillStyle = this.tCard;
    this.ctx.fillRect(0, 0, w, h);

    const data = this.sorted;
    if (!data || data.length < 2) {
      this.ctx.fillStyle = this.tMuted;
      this.ctx.font = '12px sans-serif';
      this.ctx.fillText('暂无数据', 12, h / 2);
      return;
    }

    const left = this.padL();
    const right = w - this.padR();
    const top = this.padT();
    const bottom = h - this.padB();
    if (right <= left || bottom <= top) {
      return;
    }

    const visible: ChartPoint[] = [];
    for (let i = 0; i < data.length; i++) {
      if (data[i].t >= this.viewportStart && data[i].t <= this.viewportEnd) {
        visible.push(data[i]);
      }
    }
    // Keep edge continuity: include neighbors just outside viewport.
    if (visible.length === 0) {
      const mid = (this.viewportStart + this.viewportEnd) / 2;
      const idx = this.nearestIndex(mid);
      if (idx >= 0) {
        visible.push(data[idx]);
      }
    } else {
      let firstVis = 0;
      for (let i = 0; i < data.length; i++) {
        if (data[i].t >= this.viewportStart) {
          firstVis = i;
          break;
        }
      }
      let lastVis = data.length - 1;
      for (let i = data.length - 1; i >= 0; i--) {
        if (data[i].t <= this.viewportEnd) {
          lastVis = i;
          break;
        }
      }
      if (firstVis > 0) {
        visible.unshift(data[firstVis - 1]);
      }
      if (lastVis < data.length - 1) {
        visible.push(data[lastVis + 1]);
      }
    }
    if (visible.length === 0) {
      return;
    }

    let minV = visible[0].v;
    let maxV = visible[0].v;
    for (let i = 1; i < visible.length; i++) {
      if (visible[i].v < minV) {
        minV = visible[i].v;
      }
      if (visible[i].v > maxV) {
        maxV = visible[i].v;
      }
    }
    if (Math.abs(maxV - minV) < 0.0001) {
      minV -= 1;
      maxV += 1;
    }
    const pad = (maxV - minV) * 0.12;
    minV -= pad;
    maxV += pad;
    const valueSpan = Math.max(0.0001, maxV - minV);
    const plotH = bottom - top;
    const timeSpan = this.viewSpan();

    // Grid
    this.ctx.strokeStyle = this.tBorderSoft;
    this.ctx.lineWidth = 1;
    for (let g = 0; g < 5; g++) {
      const gy = top + (plotH * g) / 4;
      this.ctx.beginPath();
      this.ctx.moveTo(left, gy);
      this.ctx.lineTo(right, gy);
      this.ctx.stroke();
    }
    for (let g = 0; g < 4; g++) {
      const gx = left + ((right - left) * g) / 3;
      this.ctx.beginPath();
      this.ctx.moveTo(gx, top);
      this.ctx.lineTo(gx, bottom);
      this.ctx.stroke();
    }

    // Series path
    this.ctx.strokeStyle = this.strokeColor();
    this.ctx.lineWidth = 2;
    this.ctx.lineJoin = 'round';
    this.ctx.lineCap = 'round';
    this.ctx.beginPath();
    for (let i = 0; i < visible.length; i++) {
      const x = left + ((visible[i].t - this.viewportStart) / timeSpan) * (right - left);
      const y = bottom - ((visible[i].v - minV) / valueSpan) * plotH;
      if (i === 0) {
        this.ctx.moveTo(x, y);
      } else {
        this.ctx.lineTo(x, y);
      }
    }
    this.ctx.stroke();

    if (visible.length <= 80) {
      this.ctx.fillStyle = this.strokeColor();
      for (let i = 0; i < visible.length; i++) {
        const x = left + ((visible[i].t - this.viewportStart) / timeSpan) * (right - left);
        const y = bottom - ((visible[i].v - minV) / valueSpan) * plotH;
        this.ctx.beginPath();
        this.ctx.arc(x, y, 2.2, 0, Math.PI * 2);
        this.ctx.fill();
      }
    }

    // Y labels
    this.ctx.fillStyle = this.tMuted;
    this.ctx.font = '10px sans-serif';
    this.ctx.textAlign = 'left';
    for (let g = 0; g < 5; g++) {
      const value = maxV - (valueSpan * g) / 4;
      const gy = top + (plotH * g) / 4;
      this.ctx.fillText(this.formatCompact(value), 2, gy + 3);
    }

    // X labels
    this.ctx.textAlign = 'center';
    for (let g = 0; g < 3; g++) {
      const ts = this.viewportStart + (timeSpan * g) / 2;
      const gx = left + ((right - left) * g) / 2;
      this.ctx.fillText(this.formatAxisTime(ts), gx, h - 6);
    }
    this.ctx.textAlign = 'left';

    // Crosshair when scrubbing
    if (this.interactive && this.scrubActive && this.scrubIndex >= 0 && this.scrubIndex < data.length) {
      const sel = data[this.scrubIndex];
      if (sel.t >= this.viewportStart && sel.t <= this.viewportEnd) {
        const sx = left + ((sel.t - this.viewportStart) / timeSpan) * (right - left);
        const sy = bottom - ((sel.v - minV) / valueSpan) * plotH;
        this.ctx.strokeStyle = this.strokeColor();
        this.ctx.globalAlpha = 0.65;
        this.ctx.lineWidth = 1;
        this.ctx.beginPath();
        this.ctx.moveTo(sx, top);
        this.ctx.lineTo(sx, bottom);
        this.ctx.stroke();
        this.ctx.globalAlpha = 0.35;
        this.ctx.beginPath();
        this.ctx.moveTo(left, sy);
        this.ctx.lineTo(right, sy);
        this.ctx.stroke();
        this.ctx.globalAlpha = 1;
        this.ctx.fillStyle = this.tCard;
        this.ctx.beginPath();
        this.ctx.arc(sx, sy, 6, 0, Math.PI * 2);
        this.ctx.fill();
        this.ctx.fillStyle = this.strokeColor();
        this.ctx.beginPath();
        this.ctx.arc(sx, sy, 4, 0, Math.PI * 2);
        this.ctx.fill();
      }
    }
  }

  private fingerLocalX(event: GestureEvent, fallback: number): number {
    const fingers = event.fingerList;
    if (fingers && fingers.length > 0) {
      return fingers[0].localX;
    }
    return fallback;
  }

  private pinchMidX(event: PinchGestureEvent): number {
    const fingers = event.fingerList;
    if (fingers && fingers.length >= 2) {
      return (fingers[0].localX + fingers[1].localX) / 2;
    }
    if (fingers && fingers.length === 1) {
      return fingers[0].localX;
    }
    return (this.plotLeft() + this.plotRight()) / 2;
  }

  build() {
    Column() {
      if (this.title) {
        Text(this.title)
          .fontSize(AppTheme.fontSmall)
          .fontColor(this.tText)
          .width('100%')
          .margin({ bottom: 6 })
      }

      if (this.interactive && this.scrubActive) {
        Row() {
          Text(this.scrubTimeLabel)
            .fontSize(AppTheme.fontCaption)
            .fontWeight(FontWeight.Bold)
            .fontColor(this.tText)
            .layoutWeight(1)
          Text(this.scrubValueLabel)
            .fontSize(AppTheme.fontLead)
            .fontWeight(FontWeight.Bold)
            .fontColor(this.strokeColor())
        }
        .width('100%')
        .padding({ left: 12, right: 12, top: 8, bottom: 8 })
        .backgroundColor(this.tAccentSoft)
        .borderRadius(AppTheme.rSm)
        .margin({ bottom: 6 })
      }

      if (this.interactive) {
        Canvas(this.ctx)
          .width('100%')
          .height(this.heightVp)
          .borderRadius(AppTheme.rSm)
          .onReady(() => {
            this.rebuildSorted(false);
            this.draw();
          })
          .onAreaChange((_o: Area, n: Area) => {
            this.canvasW = Number(n.width);
            this.canvasH = Number(n.height);
            this.draw();
          })
          .priorityGesture(
            GestureGroup(GestureMode.Exclusive,
              GestureGroup(GestureMode.Parallel,
                PinchGesture({ fingers: 2, distance: 6 })
                  .onActionStart((event: PinchGestureEvent) => {
                    this.pinchBaseSpan = this.viewSpan();
                    this.pinchBaseCenter = this.tsForX(this.pinchMidX(event));
                    this.panMode = 'viewport';
                  })
                  .onActionUpdate((event: PinchGestureEvent) => {
                    if (!event || !HistoryChart.isFiniteNum(event.scale) || event.scale <= 0) {
                      return;
                    }
                    this.setViewport(this.tsForX(this.pinchMidX(event)), this.pinchBaseSpan / event.scale);
                    this.draw();
                  })
                  .onActionEnd(() => {
                    this.panMode = 'scrub';
                  }),
                PanGesture({ fingers: 2, direction: PanDirection.Horizontal, distance: 4 })
                  .onActionStart((event: GestureEvent) => {
                    this.lastPanX = event.offsetX;
                    this.panMode = 'viewport';
                  })
                  .onActionUpdate((event: GestureEvent) => {
                    const dx = event.offsetX - this.lastPanX;
                    this.lastPanX = event.offsetX;
                    const shift = -(dx / Math.max(1, this.canvasW)) * this.viewSpan();
                    this.setViewport((this.viewportStart + this.viewportEnd) / 2 + shift, this.viewSpan());
                    this.draw();
                  })
                  .onActionEnd(() => {
                    this.panMode = 'scrub';
                  })
              ),
              PanGesture({ fingers: 1, direction: PanDirection.Horizontal, distance: 2 })
                .onActionStart((event: GestureEvent) => {
                  this.panMode = 'scrub';
                  this.scrubOriginX = this.fingerLocalX(event, (this.plotLeft() + this.plotRight()) / 2);
                  this.applyScrubAtX(this.scrubOriginX);
                })
                .onActionUpdate((event: GestureEvent) => {
                  this.applyScrubAtX(this.fingerLocalX(event, this.scrubOriginX + event.offsetX));
                }),
              TapGesture({ count: 2 })
                .onAction((event: GestureEvent) => {
                  const x = this.fingerLocalX(event, (this.plotLeft() + this.plotRight()) / 2);
                  if (this.isZoomed() && this.viewSpan() < this.fullSpan() / 3) {
                    this.resetView();
                  } else {
                    this.zoomAt(this.tsForX(x), 1.8);
                  }
                }),
              TapGesture({ count: 1 })
                .onAction((event: GestureEvent) => {
                  this.applyScrubAtX(this.fingerLocalX(event, (this.plotLeft() + this.plotRight()) / 2));
                })
            )
          )
      } else {
        Canvas(this.ctx)
          .width('100%')
          .height(this.heightVp)
          .borderRadius(AppTheme.rSm)
          .hitTestBehavior(HitTestMode.None)
          .onReady(() => {
            this.rebuildSorted(false);
            this.draw();
          })
          .onAreaChange((_o: Area, n: Area) => {
            this.canvasW = Number(n.width);
            this.canvasH = Number(n.height);
            this.draw();
          })
      }

      if (this.interactive) {
        Row() {
          Text(this.hintText)
            .fontSize(AppTheme.fontMicro)
            .fontColor(this.tMuted)
            .layoutWeight(1)
          Text('−')
            .fontSize(16)
            .fontWeight(FontWeight.Bold)
            .fontColor(this.tMuted)
            .padding({ left: 10, right: 10, top: 4, bottom: 4 })
            .onClick(() => {
              this.zoomOut();
            })
          Text('+')
            .fontSize(16)
            .fontWeight(FontWeight.Bold)
            .fontColor(this.strokeColor())
            .padding({ left: 10, right: 10, top: 4, bottom: 4 })
            .onClick(() => {
              this.zoomIn();
            })
          Text('⟲')
            .fontSize(14)
            .fontColor(this.tMuted)
            .padding({ left: 10, right: 4, top: 4, bottom: 4 })
            .onClick(() => {
              this.resetView();
            })
        }
        .width('100%')
        .margin({ top: 4 })
        .alignItems(VerticalAlign.Center)
      }
    }
    .width('100%')
  }
}

@Component
export struct TimeRangeChips {
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

  @Link rangeKey: string;
  onChange: (key: string) => void = (_k: string) => {};
  /** Android ChartTimeRange 全量 */
  private keys: string[] = ['1h', '3h', '6h', '12h', '1d', '3d', '7d', '14d'];
  private labels: string[] = ['1小时', '3小时', '6小时', '12小时', '1天', '3天', '7天', '14天'];

  build() {
    Scroll() {
      Row({ space: 8 }) {
        ForEach(this.keys, (k: string, i: number) => {
          Text(this.labels[i])
            .fontSize(AppTheme.fontCaption)
            .fontColor(this.rangeKey === k ? this.tOnAccent : this.tMuted)
            .padding({ left: 12, right: 12, top: 6, bottom: 6 })
            .backgroundColor(this.rangeKey === k ? this.tAccent : this.tElevated)
            .borderRadius(AppTheme.rSm)
            .border({
              width: 1,
              color: this.rangeKey === k ? this.tAccent : this.tBorderSoft
            })
            .onClick(() => {
              this.rangeKey = k;
              this.onChange(k);
            })
        }, (k: string) => k)
      }
    }
    .scrollable(ScrollDirection.Horizontal)
    .scrollBar(BarState.Off)
    .width('100%')
  }
}

export function rangeToSeconds(key: string): number {
  if (key === '3h') {
    return 3 * 3600;
  }
  if (key === '6h') {
    return 6 * 3600;
  }
  if (key === '12h') {
    return 12 * 3600;
  }
  if (key === '1d' || key === '24h') {
    return 24 * 3600;
  }
  if (key === '3d') {
    return 3 * 86400;
  }
  if (key === '7d') {
    return 7 * 86400;
  }
  if (key === '14d') {
    return 14 * 86400;
  }
  return 3600;
}

export function samplesToChartPoints(samples: Sample[], field: string): ChartPoint[] {
  const out: ChartPoint[] = [];
  for (let i = 0; i < samples.length; i++) {
    const s = samples[i];
    const t = Number(s.timestamp ?? 0);
    let v = 0;
    if (field === 'cpu_percent') {
      v = Number(s.cpu_percent ?? 0);
    } else if (field === 'mem_percent') {
      v = Number(s.mem_percent ?? 0);
    } else if (field === 'disk_percent') {
      v = Number(s.disk_percent ?? 0);
    } else if (field === 'gpu_percent' || field === 'gpu_util') {
      if (!(s.gpus && s.gpus.length > 0)) {
        continue;
      }
      let maxv = 0;
      for (let g = 0; g < s.gpus.length; g++) {
        const u = Number(s.gpus[g].util_percent ?? 0);
        if (u > maxv) {
          maxv = u;
        }
      }
      v = maxv;
    } else if (field === 'gpu_mem') {
      if (!(s.gpus && s.gpus.length > 0)) {
        continue;
      }
      let maxv = 0;
      for (let g = 0; g < s.gpus.length; g++) {
        const u = Number(s.gpus[g].mem_percent ?? 0);
        if (u > maxv) {
          maxv = u;
        }
      }
      v = maxv;
    }
    if (t > 0) {
      out.push({ t: t, v: v });
    }
  }
  return out;
}
