import { AppTheme } from '../common/Routes';

/** Soft pastel fill = tint @ ~14% alpha (ARGB), matches Android tint.copy(alpha=0.14f). */
export function pastelOf(tint: string): string {
  const hex = tint.replace('#', '');
  if (hex.length === 6) {
    return `#24${hex}`;
  }
  return tint;
}

/**
 * Canvas icons approximating Android Material Icons used on Dashboard / BottomNav.
 * Avoids emoji and SDK-specific SymbolGlyph names.
 */
@Component
export struct DrawIcon {
  @StorageLink('aiops_theme_rev') @Watch('repaint') themeRev: number = 0;

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


  @Prop @Watch('repaint') kind: string = 'house';
  @Prop @Watch('repaint') tint: string = '#4F7FFF';
  @Prop iconSize: number = 22;
  private settings: RenderingContextSettings = new RenderingContextSettings(true);
  private ctx: CanvasRenderingContext2D = new CanvasRenderingContext2D(this.settings);

  private repaint(): void {
    this.paint();
  }

  private paint(): void {
    const c = this.ctx;
    const s = this.iconSize;
    c.clearRect(0, 0, s, s);
    c.strokeStyle = this.tint;
    c.fillStyle = this.tint;
    c.lineWidth = Math.max(1.5, s * 0.08);
    c.lineCap = 'round';
    c.lineJoin = 'round';
    const k = this.kind;
    if (k === 'house') {
      c.beginPath();
      c.moveTo(s * 0.5, s * 0.18);
      c.lineTo(s * 0.18, s * 0.48);
      c.lineTo(s * 0.18, s * 0.82);
      c.lineTo(s * 0.82, s * 0.82);
      c.lineTo(s * 0.82, s * 0.48);
      c.closePath();
      c.stroke();
      c.fillRect(s * 0.42, s * 0.55, s * 0.16, s * 0.27);
    } else if (k === 'speed') {
      c.beginPath();
      c.arc(s * 0.5, s * 0.55, s * 0.32, Math.PI * 0.85, Math.PI * 0.15, false);
      c.stroke();
      c.beginPath();
      c.moveTo(s * 0.5, s * 0.55);
      c.lineTo(s * 0.72, s * 0.32);
      c.stroke();
      c.beginPath();
      c.arc(s * 0.5, s * 0.55, s * 0.05, 0, Math.PI * 2);
      c.fill();
    } else if (k === 'warning') {
      c.beginPath();
      c.moveTo(s * 0.5, s * 0.14);
      c.lineTo(s * 0.86, s * 0.82);
      c.lineTo(s * 0.14, s * 0.82);
      c.closePath();
      c.stroke();
      c.fillRect(s * 0.46, s * 0.38, s * 0.08, s * 0.22);
      c.beginPath();
      c.arc(s * 0.5, s * 0.7, s * 0.045, 0, Math.PI * 2);
      c.fill();
    } else if (k === 'build') {
      c.beginPath();
      c.moveTo(s * 0.22, s * 0.78);
      c.lineTo(s * 0.55, s * 0.45);
      c.lineTo(s * 0.68, s * 0.58);
      c.lineTo(s * 0.35, s * 0.9);
      c.closePath();
      c.stroke();
      c.beginPath();
      c.arc(s * 0.7, s * 0.3, s * 0.16, 0, Math.PI * 2);
      c.stroke();
    } else if (k === 'spark') {
      c.beginPath();
      c.moveTo(s * 0.5, s * 0.12);
      c.lineTo(s * 0.58, s * 0.42);
      c.lineTo(s * 0.88, s * 0.5);
      c.lineTo(s * 0.58, s * 0.58);
      c.lineTo(s * 0.5, s * 0.88);
      c.lineTo(s * 0.42, s * 0.58);
      c.lineTo(s * 0.12, s * 0.5);
      c.lineTo(s * 0.42, s * 0.42);
      c.closePath();
      c.fill();
    } else if (k === 'refresh') {
      c.beginPath();
      c.arc(s * 0.5, s * 0.5, s * 0.28, Math.PI * 0.15, Math.PI * 1.55, false);
      c.stroke();
      c.beginPath();
      c.moveTo(s * 0.72, s * 0.22);
      c.lineTo(s * 0.72, s * 0.42);
      c.lineTo(s * 0.52, s * 0.42);
      c.stroke();
    } else if (k === 'settings') {
      c.beginPath();
      c.arc(s * 0.5, s * 0.5, s * 0.14, 0, Math.PI * 2);
      c.stroke();
      for (let i = 0; i < 6; i++) {
        const a = (i / 6) * Math.PI * 2;
        c.beginPath();
        c.moveTo(s * 0.5 + Math.cos(a) * s * 0.22, s * 0.5 + Math.sin(a) * s * 0.22);
        c.lineTo(s * 0.5 + Math.cos(a) * s * 0.36, s * 0.5 + Math.sin(a) * s * 0.36);
        c.stroke();
      }
    } else if (k === 'add') {
      c.beginPath();
      c.moveTo(s * 0.5, s * 0.2);
      c.lineTo(s * 0.5, s * 0.8);
      c.moveTo(s * 0.2, s * 0.5);
      c.lineTo(s * 0.8, s * 0.5);
      c.stroke();
    } else if (k === 'hardware' || k === 'memory') {
      c.strokeRect(s * 0.22, s * 0.28, s * 0.56, s * 0.44);
      c.fillRect(s * 0.34, s * 0.4, s * 0.12, s * 0.2);
      c.fillRect(s * 0.54, s * 0.4, s * 0.12, s * 0.2);
      for (let i = 0; i < 4; i++) {
        c.fillRect(s * 0.28 + i * s * 0.12, s * 0.2, s * 0.06, s * 0.08);
        c.fillRect(s * 0.28 + i * s * 0.12, s * 0.72, s * 0.06, s * 0.08);
      }
    } else if (k === 'hyperv' || k === 'dvr') {
      c.strokeRect(s * 0.18, s * 0.22, s * 0.64, s * 0.48);
      c.beginPath();
      c.moveTo(s * 0.35, s * 0.78);
      c.lineTo(s * 0.65, s * 0.78);
      c.stroke();
      c.fillRect(s * 0.45, s * 0.7, s * 0.1, s * 0.08);
    } else if (k === 'netflow' || k === 'lan') {
      c.beginPath();
      c.arc(s * 0.5, s * 0.28, s * 0.1, 0, Math.PI * 2);
      c.stroke();
      c.beginPath();
      c.arc(s * 0.28, s * 0.7, s * 0.1, 0, Math.PI * 2);
      c.stroke();
      c.beginPath();
      c.arc(s * 0.72, s * 0.7, s * 0.1, 0, Math.PI * 2);
      c.stroke();
      c.beginPath();
      c.moveTo(s * 0.5, s * 0.38);
      c.lineTo(s * 0.32, s * 0.62);
      c.moveTo(s * 0.5, s * 0.38);
      c.lineTo(s * 0.68, s * 0.62);
      c.stroke();
    } else if (k === 'probe' || k === 'language') {
      c.beginPath();
      c.arc(s * 0.5, s * 0.5, s * 0.32, 0, Math.PI * 2);
      c.stroke();
      c.beginPath();
      c.ellipse(s * 0.5, s * 0.5, s * 0.14, s * 0.32, 0, 0, Math.PI * 2);
      c.stroke();
      c.beginPath();
      c.moveTo(s * 0.18, s * 0.5);
      c.lineTo(s * 0.82, s * 0.5);
      c.stroke();
    } else if (k === 'hosts' || k === 'dns') {
      c.strokeRect(s * 0.28, s * 0.18, s * 0.44, s * 0.2);
      c.strokeRect(s * 0.28, s * 0.4, s * 0.44, s * 0.2);
      c.strokeRect(s * 0.28, s * 0.62, s * 0.44, s * 0.2);
      c.fillRect(s * 0.34, s * 0.25, s * 0.06, s * 0.06);
      c.fillRect(s * 0.34, s * 0.47, s * 0.06, s * 0.06);
      c.fillRect(s * 0.34, s * 0.69, s * 0.06, s * 0.06);
    } else if (k === 'logs') {
      c.beginPath();
      c.arc(s * 0.42, s * 0.42, s * 0.22, 0, Math.PI * 2);
      c.stroke();
      c.beginPath();
      c.moveTo(s * 0.58, s * 0.58);
      c.lineTo(s * 0.82, s * 0.82);
      c.stroke();
    } else if (k === 'dashboards') {
      c.beginPath();
      c.moveTo(s * 0.18, s * 0.7);
      c.lineTo(s * 0.38, s * 0.45);
      c.lineTo(s * 0.55, s * 0.58);
      c.lineTo(s * 0.82, s * 0.28);
      c.stroke();
      c.beginPath();
      c.moveTo(s * 0.7, s * 0.22);
      c.lineTo(s * 0.82, s * 0.22);
      c.lineTo(s * 0.82, s * 0.34);
      c.stroke();
    } else if (k === 'ai') {
      c.beginPath();
      c.moveTo(s * 0.5, s * 0.12);
      c.lineTo(s * 0.58, s * 0.42);
      c.lineTo(s * 0.88, s * 0.5);
      c.lineTo(s * 0.58, s * 0.58);
      c.lineTo(s * 0.5, s * 0.88);
      c.lineTo(s * 0.42, s * 0.58);
      c.lineTo(s * 0.12, s * 0.5);
      c.lineTo(s * 0.42, s * 0.42);
      c.closePath();
      c.fill();
    } else if (k === 'lock') {
      c.strokeRect(s * 0.28, s * 0.45, s * 0.44, s * 0.36);
      c.beginPath();
      c.arc(s * 0.5, s * 0.42, s * 0.16, Math.PI, 0, false);
      c.stroke();
    } else if (k === 'eye' || k === 'eye_off') {
      c.beginPath();
      c.ellipse(s * 0.5, s * 0.5, s * 0.32, s * 0.2, 0, 0, Math.PI * 2);
      c.stroke();
      c.beginPath();
      c.arc(s * 0.5, s * 0.5, s * 0.08, 0, Math.PI * 2);
      c.fill();
      if (k === 'eye_off') {
        c.beginPath();
        c.moveTo(s * 0.22, s * 0.78);
        c.lineTo(s * 0.78, s * 0.22);
        c.stroke();
      }
    } else if (k === 'back') {
      c.beginPath();
      c.moveTo(s * 0.62, s * 0.22);
      c.lineTo(s * 0.32, s * 0.5);
      c.lineTo(s * 0.62, s * 0.78);
      c.stroke();
    } else if (k === 'person') {
      c.beginPath();
      c.arc(s * 0.5, s * 0.32, s * 0.14, 0, Math.PI * 2);
      c.stroke();
      c.beginPath();
      c.arc(s * 0.5, s * 0.82, s * 0.28, Math.PI * 1.15, Math.PI * 1.85, false);
      c.stroke();
    } else if (k === 'mic') {
      c.strokeRect(s * 0.4, s * 0.18, s * 0.2, s * 0.4);
      c.beginPath();
      c.arc(s * 0.5, s * 0.58, s * 0.2, 0, Math.PI, false);
      c.stroke();
      c.beginPath();
      c.moveTo(s * 0.5, s * 0.78);
      c.lineTo(s * 0.5, s * 0.88);
      c.stroke();
    } else if (k === 'attach') {
      c.beginPath();
      c.moveTo(s * 0.62, s * 0.28);
      c.lineTo(s * 0.38, s * 0.52);
      c.quadraticCurveTo(s * 0.28, s * 0.62, s * 0.38, s * 0.72);
      c.quadraticCurveTo(s * 0.48, s * 0.82, s * 0.58, s * 0.72);
      c.lineTo(s * 0.72, s * 0.58);
      c.stroke();
    } else if (k === 'send') {
      // Upward send arrow (not left chevron / side paper-plane)
      c.beginPath();
      c.moveTo(s * 0.5, s * 0.16);
      c.lineTo(s * 0.78, s * 0.46);
      c.lineTo(s * 0.58, s * 0.46);
      c.lineTo(s * 0.58, s * 0.84);
      c.lineTo(s * 0.42, s * 0.84);
      c.lineTo(s * 0.42, s * 0.46);
      c.lineTo(s * 0.22, s * 0.46);
      c.closePath();
      c.fill();
    } else if (k === 'stop') {
      c.fillRect(s * 0.3, s * 0.3, s * 0.4, s * 0.4);
    } else {
      c.beginPath();
      c.arc(s * 0.5, s * 0.5, s * 0.2, 0, Math.PI * 2);
      c.fill();
    }
  }

  build() {
    Canvas(this.ctx)
      .width(this.iconSize)
      .height(this.iconSize)
      .onReady(() => {
        this.paint();
      })
  }
}

@Component
export struct AppSymbol {
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


  @Prop kind: string = 'house';
  @Prop selected: boolean = false;
  @Prop tint: string = '';
  @Prop iconSize: number = 22;

  private color(): string {
    if (this.tint) {
      return this.tint;
    }
    return this.selected ? this.tAccent : this.tMuted;
  }

  build() {
    DrawIcon({ kind: this.kind, tint: this.color(), iconSize: this.iconSize })
  }
}

@Component
export struct TabBarItem {
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


  @Prop title: string = '';
  @Prop kind: string = 'house';
  @Prop selected: boolean = false;

  build() {
    Column({ space: 3 }) {
      Column() {
        AppSymbol({
          kind: this.kind,
          selected: this.selected,
          tint: this.selected ? this.tAccentOn : '',
          iconSize: 20
        })
      }
      .width(40)
      .height(24)
      .justifyContent(FlexAlign.Center)
      .borderRadius(AppTheme.rSm)
      .backgroundColor(this.selected ? this.tAccentSoft : Color.Transparent)

      Text(this.title)
        .fontSize(AppTheme.fontTiny)
        .fontColor(this.selected ? this.tAccent : this.tMuted)
        .fontWeight(this.selected ? FontWeight.Medium : FontWeight.Regular)
        .maxLines(1)
    }
    .width('100%')
    .height(AppTheme.navBarH)
    .justifyContent(FlexAlign.Center)
  }
}

@Component
export struct HeaderIconButton {
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


  @Prop kind: string = 'settings';
  @Prop label: string = '';
  onTap: () => void = () => {};

  build() {
    if (this.label) {
      Row({ space: 4 }) {
        AppSymbol({ kind: this.kind, tint: this.tMuted, iconSize: 18 })
        Text(this.label)
          .fontSize(AppTheme.fontCaption)
          .fontWeight(FontWeight.Medium)
          .fontColor(this.tAccent)
      }
      .padding({ left: 8, right: 8, top: 6, bottom: 6 })
      .borderRadius(AppTheme.rXs)
      .onClick(() => this.onTap())
    } else {
      Column() {
        AppSymbol({ kind: this.kind, tint: this.tMuted, iconSize: 20 })
      }
      .width(AppTheme.headerIcon)
      .height(AppTheme.headerIcon)
      .justifyContent(FlexAlign.Center)
      .borderRadius(AppTheme.rXs)
      .onClick(() => this.onTap())
    }
  }
}

@Component
export struct QuickTile {
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


  @Prop title: string = '';
  @Prop iconKind: string = 'hardware';
  @Prop tint: string = '#4F7FFF';
  onTap: () => void = () => {};

  build() {
    Column({ space: 5 }) {
      Column() {
        DrawIcon({ kind: this.iconKind, tint: this.tint, iconSize: 21 })
      }
      .width(42)
      .height(42)
      .justifyContent(FlexAlign.Center)
      .borderRadius(AppTheme.rSm)
      .backgroundColor(pastelOf(this.tint))

      Text(this.title)
        .fontSize(AppTheme.fontTiny)
        .fontColor(this.tText)
        .maxLines(1)
        .textAlign(TextAlign.Center)
    }
    .width('100%')
    .padding({ top: 3, bottom: 3 })
    .alignItems(HorizontalAlign.Center)
    .onClick(() => this.onTap())
  }
}
