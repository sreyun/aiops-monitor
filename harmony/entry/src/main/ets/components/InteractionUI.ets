import { AppTheme } from '../common/Routes';
import { FormatUtil } from '../common/FormatUtil';
import { Alert } from '../data/models/Models';
import { Api } from '../data/Api';
import { ApiClient, StreamUiThrottle } from '../data/ApiClient';

/** Ephemeral success/error toast (Android Snackbar parity). */
@Component
export struct ToastHost {
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


  @Prop message: string = '';
  @Prop isError: boolean = false;

  build() {
    if (this.message) {
      Row() {
        Text(this.message)
          .fontSize(AppTheme.fontCaption)
          .fontWeight(FontWeight.Medium)
          .fontColor(this.tOnAccent)
          .maxLines(2)
          .textOverflow({ overflow: TextOverflow.Ellipsis })
          .textAlign(TextAlign.Center)
          .layoutWeight(1)
      }
      .width('100%')
      .padding({ left: AppTheme.spaceLg, right: AppTheme.spaceLg, top: 12, bottom: 12 })
      .backgroundColor(this.isError ? this.tDanger : this.tAccent)
      .borderRadius(AppTheme.rMd)
      .shadow({ radius: 12, color: this.tShadow, offsetY: 4 })
      .margin({ left: AppTheme.spaceXl, right: AppTheme.spaceXl })
      .position({ bottom: 96 })
      .zIndex(200)
    }
  }
}

export class ToastController {
  static delayClear(setter: (v: string) => void, msg: string, ms: number = 2200): void {
    setTimeout(() => {
      setter('');
    }, ms);
  }
}

@Component
export struct WeatherHero {
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


  greeting: string = '';
  dateText: string = '';
  weatherLine: string = '';
  tempText: string = '';
  onlineText: string = '0/0';
  criticalText: string = '0';
  warningText: string = '0';
  onOnline: () => void = () => {};
  onCritical: () => void = () => {};
  onWarning: () => void = () => {};

  build() {
    Column({ space: 8 }) {
      Row() {
        Column() {
          Text(this.greeting || '你好')
            .fontSize(AppTheme.fontTitle)
            .fontWeight(FontWeight.Bold)
            .fontColor(Color.White)
          Text(this.dateText)
            .fontSize(AppTheme.fontMicro)
            .fontColor('#FFFFFFCC')
            .margin({ top: 4 })
        }
        .alignItems(HorizontalAlign.Start)
        .layoutWeight(1)

        if (this.tempText && this.tempText !== '—') {
          Column() {
            Text(this.tempText)
              .fontSize(AppTheme.fontHero)
              .fontWeight(FontWeight.Bold)
              .fontColor(Color.White)
            if (this.weatherLine) {
              Text(this.weatherLine)
                .fontSize(AppTheme.fontTiny)
                .fontColor('#FFFFFFD9')
                .margin({ top: 2 })
            }
          }
          .alignItems(HorizontalAlign.Center)
        }
      }
      .width('100%')
      .alignItems(VerticalAlign.Top)

      Row({ space: 10 }) {
        Column() {
          Text(this.onlineText)
            .fontSize(AppTheme.fontBody)
            .fontWeight(FontWeight.Bold)
            .fontColor(Color.White)
          Text('在线')
            .fontSize(AppTheme.fontTiny)
            .fontColor('#FFFFFFBF')
            .margin({ top: 2 })
        }
        .padding({ left: 3, right: 3, top: 1, bottom: 1 })
        .borderRadius(AppTheme.rChip)
        .onClick(() => this.onOnline())
        Column() {
          Text(this.criticalText)
            .fontSize(AppTheme.fontBody)
            .fontWeight(FontWeight.Bold)
            .fontColor(Color.White)
          Text('严重')
            .fontSize(AppTheme.fontTiny)
            .fontColor('#FFFFFFBF')
            .margin({ top: 2 })
        }
        .padding({ left: 3, right: 3, top: 1, bottom: 1 })
        .borderRadius(AppTheme.rChip)
        .onClick(() => this.onCritical())
        Column() {
          Text(this.warningText)
            .fontSize(AppTheme.fontBody)
            .fontWeight(FontWeight.Bold)
            .fontColor(Color.White)
          Text('警告')
            .fontSize(AppTheme.fontTiny)
            .fontColor('#FFFFFFBF')
            .margin({ top: 2 })
        }
        .padding({ left: 3, right: 3, top: 1, bottom: 1 })
        .borderRadius(AppTheme.rChip)
        .onClick(() => this.onWarning())
      }
      .width('100%')
    }
    .width('100%')
    .padding({
      left: AppTheme.cardPad,
      right: AppTheme.cardPad,
      top: AppTheme.spaceMd,
      bottom: AppTheme.spaceMd
    })
    .linearGradient({
      angle: 135,
      colors: [[AppTheme.brand, 0], ['#6AA1FF', 0.5], ['#67C7F0', 1]]
    })
    .borderRadius(AppTheme.rMd)
    .shadow({ radius: 10, color: '#334F7FFF', offsetY: 3 })
  }
}

interface OpsKpiItem {
  label: string;
  value: string;
  color: string;
}

@Component
export struct OpsKpiStrip {
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


  @Prop items: OpsKpiItem[] = [];
  onSelect: (i: number) => void = (_i: number) => {};

  build() {
    Row() {
      ForEach(this.items, (it: OpsKpiItem, i: number) => {
        Column() {
          Text(it.value)
            .fontSize(AppTheme.fontTitle)
            .fontWeight(FontWeight.Bold)
            .fontColor(it.color)
          Text(it.label)
            .fontSize(AppTheme.fontMicro)
            .fontColor(this.tMuted)
            .margin({ top: 3 })
            .maxLines(1)
        }
        .layoutWeight(1)
        .padding({ top: AppTheme.spaceMd, bottom: AppTheme.spaceMd })
        .onClick(() => this.onSelect(i))
        if (i < this.items.length - 1) {
          Divider()
            .vertical(true)
            .height(32)
            .color(this.tBorderSoft)
        }
      }, (it: OpsKpiItem, i: number) => `${i}-${it.label}`)
    }
    .width('100%')
    .backgroundColor(this.tCard)
    .borderRadius(AppTheme.rMd)
    .border({ width: AppTheme.hairline, color: this.tBorderSoft })
    .margin({ left: AppTheme.pagePadH, right: AppTheme.pagePadH, top: AppTheme.spaceSm, bottom: AppTheme.spaceXs })
    .shadow({ radius: 6, color: this.tShadow, offsetY: 2 })
  }
}

@Component
export struct CompactHostRow {
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


  title: string = '';
  subtitle: string = '';
  trailing: string = '';
  danger: boolean = false;
  onTap: () => void = () => {};

  build() {
    Row() {
      Column() {
        Text(this.title)
          .fontSize(AppTheme.fontCaption)
          .fontWeight(FontWeight.Medium)
          .fontColor(this.tText)
          .maxLines(1)
          .textOverflow({ overflow: TextOverflow.Ellipsis })
        if (this.subtitle) {
          Text(this.subtitle)
            .fontSize(AppTheme.fontMicro)
            .fontColor(this.tMuted)
            .margin({ top: 2 })
            .maxLines(1)
        }
      }
      .alignItems(HorizontalAlign.Start)
      .layoutWeight(1)
      Text(this.trailing)
        .fontSize(AppTheme.fontMicro)
        .fontWeight(FontWeight.Bold)
        .fontColor(this.danger ? this.tDanger : this.tAccent)
        .padding({ left: 8, right: 8, top: 3, bottom: 3 })
        .backgroundColor(this.danger ? this.tDangerSoft : this.tAccentSoft)
        .borderRadius(AppTheme.rXs)
    }
    .width('100%')
    .padding({ left: AppTheme.spaceMd, right: AppTheme.spaceMd, top: 10, bottom: 10 })
    .backgroundColor(this.tCard)
    .borderRadius(AppTheme.rSm)
    .border({ width: AppTheme.hairline, color: this.tBorderSoft })
    .onClick(() => this.onTap())
  }
}

@Component
export struct AlertCardView {
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


  @Prop alert: Alert = {
    host_id: '',
    hostname: '',
    level: '',
    type: '',
    message: ''
  };
  @Prop busy: boolean = false;
  onAck: () => void = () => {};
  onSilence: () => void = () => {};
  onDiagnose: () => void = () => {};

  private statusZh(st: string): string {
    return FormatUtil.alertStatusZh(st);
  }

  private isAcked(): boolean {
    return (this.alert.status || '').toLowerCase() === 'acknowledged';
  }

  private isSilenced(): boolean {
    return (this.alert.status || '').toLowerCase() === 'silenced';
  }

  private typeLabel(): string {
    const x = (this.alert.type || '').toLowerCase();
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
    return (this.alert.type || '—').toUpperCase();
  }

  private durationText(): string {
    if (!this.alert.since) {
      return FormatUtil.relTime(this.alert.timestamp);
    }
    return FormatUtil.relTime(this.alert.since);
  }

  build() {
    // 左边框代替 height('100%') 色条，避免 ListItem 撑满屏
    Column() {
      Row() {
        Circle({ width: 8, height: 8 })
          .fill(FormatUtil.sevColor(this.alert.level))
          .margin({ right: 8 })
        Text(this.alert.hostname || this.alert.host_id || '—')
          .fontSize(AppTheme.fontSubtitle)
          .fontWeight(FontWeight.Bold)
          .fontColor(this.tText)
          .layoutWeight(1)
          .maxLines(1)
          .textOverflow({ overflow: TextOverflow.Ellipsis })
        if (this.alert.status) {
          Text(this.statusZh(this.alert.status || ''))
            .fontSize(AppTheme.fontTiny)
            .fontColor(FormatUtil.statusColor(this.alert.status || 'active'))
            .padding({ left: 6, right: 6, top: 2, bottom: 2 })
            .backgroundColor(FormatUtil.statusSoft(this.alert.status || 'active'))
            .borderRadius(AppTheme.rChip)
            .margin({ right: 6 })
        }
        Text(FormatUtil.levelLabel(this.alert.level))
          .fontSize(AppTheme.fontTiny)
          .fontWeight(FontWeight.Bold)
          .fontColor(FormatUtil.sevColor(this.alert.level))
          .padding({ left: 6, right: 6, top: 2, bottom: 2 })
          .backgroundColor(FormatUtil.sevSoft(this.alert.level))
          .borderRadius(AppTheme.rChip)
      }
      .width('100%')

      Text(this.alert.message || '')
        .fontSize(AppTheme.fontSmall)
        .fontColor(this.tText)
        .margin({ top: 8 })
        .width('100%')
        .maxLines(3)
        .textOverflow({ overflow: TextOverflow.Ellipsis })

      Flex({ wrap: FlexWrap.Wrap }) {
        Text(this.typeLabel())
          .fontSize(AppTheme.fontTiny)
          .fontColor(this.tAccent)
          .padding({ left: 8, right: 8, top: 3, bottom: 3 })
          .backgroundColor(this.tAccentSoft)
          .borderRadius(AppTheme.rChip)
          .margin({ right: 6, top: 6 })
        if (this.alert.scope) {
          Text((this.alert.scope || '').toUpperCase())
            .fontSize(AppTheme.fontTiny)
            .fontColor(this.tMuted)
            .padding({ left: 8, right: 8, top: 3, bottom: 3 })
            .backgroundColor(this.tElevated)
            .borderRadius(AppTheme.rChip)
            .margin({ right: 6, top: 6 })
        }
        if (this.alert.ip) {
          Text(this.alert.ip)
            .fontSize(AppTheme.fontTiny)
            .fontColor(this.tMuted)
            .padding({ left: 8, right: 8, top: 3, bottom: 3 })
            .backgroundColor(this.tElevated)
            .borderRadius(AppTheme.rChip)
            .margin({ right: 6, top: 6 })
        }
        Text(this.durationText())
          .fontSize(AppTheme.fontTiny)
          .fontColor(this.tMuted)
          .padding({ left: 8, right: 8, top: 3, bottom: 3 })
          .backgroundColor(this.tElevated)
          .borderRadius(AppTheme.rChip)
          .margin({ top: 6 })
      }
      .width('100%')

      Divider()
        .color(this.tBorderSoft)
        .strokeWidth(0.5)
        .margin({ top: 10, bottom: 2 })

      Row({ space: 8 }) {
        if (this.busy) {
          LoadingProgress().width(14).height(14).color(this.tAccent)
        }
        Button(this.isAcked() ? '已确认' : '确认')
          .layoutWeight(1)
          .height(36)
          .fontSize(AppTheme.fontMicro)
          .fontWeight(FontWeight.Bold)
          .enabled(!this.busy && !this.isAcked())
          .opacity(this.busy ? 0.55 : 1)
          .backgroundColor(this.isAcked() ? this.tSuccessSoft : this.tElevated)
          .fontColor(this.isAcked() ? this.tSuccess : this.tText)
          .borderRadius(AppTheme.rXs)
          .onClick(() => this.onAck())
        Button(this.isSilenced() ? '已静默' : '静默')
          .layoutWeight(1)
          .height(36)
          .fontSize(AppTheme.fontMicro)
          .fontWeight(FontWeight.Bold)
          .enabled(!this.busy && !this.isSilenced())
          .opacity(this.busy ? 0.55 : 1)
          .backgroundColor(this.isSilenced() ? this.tWarningSoft : this.tCard)
          .fontColor(this.tWarning)
          .borderRadius(AppTheme.rXs)
          .border({ width: AppTheme.hairline, color: this.tWarning })
          .onClick(() => this.onSilence())
        Button('AI 诊断')
          .layoutWeight(1)
          .height(36)
          .fontSize(AppTheme.fontMicro)
          .fontWeight(FontWeight.Bold)
          .enabled(!this.busy)
          .opacity(this.busy ? 0.55 : 1)
          .backgroundColor(this.tCard)
          .fontColor(this.tAccent)
          .borderRadius(AppTheme.rXs)
          .border({ width: AppTheme.hairline, color: this.tAccent })
          .onClick(() => this.onDiagnose())
      }
      .width('100%')
      .margin({ top: 10 })
      .alignItems(VerticalAlign.Center)
    }
    .width('100%')
    .padding(AppTheme.cardPad)
    .backgroundColor(this.tCard)
    .borderRadius(AppTheme.rMd)
    .borderWidth({ left: 4, right: 1, top: 1, bottom: 1 })
    .borderColor({
      left: FormatUtil.sevColor(this.alert.level),
      right: this.tBorderSoft,
      top: this.tBorderSoft,
      bottom: this.tBorderSoft
    })
    .shadow({ radius: 4, color: this.tShadow, offsetY: 1 })
    .alignItems(HorizontalAlign.Start)
    .clip(true)
  }
}

@Component
export struct DetailSheet {
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


  @Prop visible: boolean = false;
  title: string = '';
  onClose: () => void = () => {};
  @BuilderParam content: () => void = this.defaultBody;

  @Builder
  defaultBody() {
    Text('').height(1)
  }

  build() {
    if (this.visible) {
      Column() {
        Blank()
          .layoutWeight(1)
          .onClick(() => this.onClose())
        Column() {
          Row()
            .width(40)
            .height(4)
            .borderRadius(2)
            .backgroundColor(this.tBorder)
            .opacity(0.7)
            .margin({ bottom: 14 })
          Row() {
            Text(this.title)
              .fontSize(AppTheme.fontSubtitle)
              .fontWeight(FontWeight.Bold)
              .fontColor(this.tText)
              .layoutWeight(1)
              .maxLines(1)
              .textOverflow({ overflow: TextOverflow.Ellipsis })
            Text('关闭')
              .fontSize(AppTheme.fontBody)
              .fontWeight(FontWeight.Medium)
              .fontColor(this.tAccent)
              .padding({ left: 8, top: 4, bottom: 4 })
              .onClick(() => this.onClose())
          }
          .width('100%')
          .padding({ bottom: AppTheme.spaceMd })
          Scroll() {
            this.content()
          }
          .constraintSize({ maxHeight: '68%' })
          .scrollBar(BarState.Off)
          .width('100%')
        }
        .width('100%')
        .padding({ left: AppTheme.pagePadH, right: AppTheme.pagePadH, top: 12, bottom: 32 })
        .backgroundColor(this.tCard)
        .borderRadius({ topLeft: AppTheme.rLg, topRight: AppTheme.rLg })
        .border({ width: AppTheme.hairline, color: this.tBorderSoft })
        .shadow({ radius: 20, color: this.tShadow, offsetY: -4 })
        .alignItems(HorizontalAlign.Center)
      }
      .width('100%')
      .height('100%')
      .backgroundColor(this.tScrim)
      .position({ x: 0, y: 0 })
      .zIndex(90)
    }
  }
}

@Component
export struct FloatingActionButton {
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


  label: string = '';
  onTap: () => void = () => {};

  build() {
    // 不用 Button：系统 Button 在 Stack 里常被拉成全宽灰底条
    Column() {
      Text('+')
        .fontSize(28)
        .fontWeight(FontWeight.Medium)
        .fontColor(this.tOnAccent)
    }
    .width(AppTheme.fabSize)
    .height(AppTheme.fabSize)
    .constraintSize({ minWidth: AppTheme.fabSize, maxWidth: AppTheme.fabSize, minHeight: AppTheme.fabSize, maxHeight: AppTheme.fabSize })
    .justifyContent(FlexAlign.Center)
    .alignItems(HorizontalAlign.Center)
    .borderRadius(AppTheme.fabSize / 2)
    .backgroundColor(this.tAccent)
    .clip(true)
    .shadow({ radius: 8, color: this.tShadow, offsetY: 3 })
    .onClick(() => this.onTap())
  }
}

export function greetingNow(): string {
  const h = new Date().getHours();
  if (h >= 5 && h <= 10) {
    return '早上好';
  }
  if (h >= 11 && h <= 13) {
    return '中午好';
  }
  if (h >= 14 && h <= 18) {
    return '下午好';
  }
  return '晚上好';
}

export function dateZhToday(): string {
  const d = new Date();
  const weeks: string[] = ['星期日', '星期一', '星期二', '星期三', '星期四', '星期五', '星期六'];
  return `${d.getMonth() + 1}月${d.getDate()}日 ${weeks[d.getDay()]}`;
}

/** Host-scoped AI assist sheet (Android AiAssistSheet parity). */
@Component
export struct AiAssistSheet {
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

  @Prop @Watch('onVisible') visible: boolean = false;
  @Prop title: string = 'AI 助手';
  @Prop @Watch('onTaskOrContext') task: string = 'assist';
  @Prop @Watch('onTaskOrContext') context: string = '';
  /** 兼容旧调用，Android AiAssist 不传 input */
  @Prop seedPrompt: string = '';
  hostId: string = '';
  onClose: () => void = () => {};

  @State answer: string = '';
  @State ragHint: string = '';
  @State busy: boolean = false;
  @State err: string = '';
  private autoStarted: boolean = false;
  private lastAutoKey: string = '';
  private runSeq: number = 0;

  aboutToAppear(): void {
    this.maybeAutoRun();
  }

  onVisible(): void {
    if (this.visible) {
      this.maybeAutoRun();
    } else {
      this.runSeq += 1;
      this.autoStarted = false;
      this.lastAutoKey = '';
      this.answer = '';
      this.ragHint = '';
      this.err = '';
      this.busy = false;
    }
  }

  onTaskOrContext(): void {
    if (this.visible) {
      this.autoStarted = false;
      this.maybeAutoRun();
    }
  }

  private maybeAutoRun(): void {
    if (!this.visible || !this.task) {
      return;
    }
    const key = `${this.task}|${this.context}`;
    if (this.autoStarted && this.lastAutoKey === key) {
      return;
    }
    this.autoStarted = true;
    this.lastAutoKey = key;
    void this.runAssist();
  }

  private stop(): void {
    // 仅取消 SSE，保持 sheet 打开（对齐 Copilot stopGen）
    this.runSeq += 1;
    this.busy = false;
    ApiClient.cancelActiveSse();
  }

  private async runAssist(): Promise<void> {
    const seq = ++this.runSeq;
    this.busy = true;
    this.err = '';
    this.answer = '';
    this.ragHint = '';
    const throttle = new StreamUiThrottle((t: string) => {
      if (seq === this.runSeq) {
        this.answer = t;
      }
    });
    try {
      // Android: AiAssistRequest(task, context) — input 默认空串
      const ctx = (this.context || (this.hostId ? `host_id=${this.hostId}` : '')).substring(0, 14000);
      const text = await Api.aiAssist({
        task: this.task || 'assist',
        input: '',
        context: ctx
      }, (chunk: string) => {
        throttle.push(chunk);
      }, (hint: string) => {
        if (seq === this.runSeq) {
          this.ragHint = hint;
        }
      });
      throttle.flush();
      if (seq !== this.runSeq) {
        return;
      }
      this.answer = text || this.answer;
      if (!(this.answer || '').trim()) {
        this.err = 'AI 未返回内容';
      }
    } catch (e) {
      if (seq !== this.runSeq) {
        return;
      }
      this.err = ApiClient.formatError(e as Object);
    } finally {
      if (seq === this.runSeq) {
        this.busy = false;
      }
    }
  }

  build() {
    DetailSheet({
      visible: this.visible,
      title: this.title,
      onClose: () => {
        this.runSeq += 1;
        ApiClient.cancelActiveSse();
        this.onClose();
      }
    }) {
      Column({ space: 10 }) {
        if (this.ragHint) {
          Text(this.ragHint)
            .fontSize(AppTheme.fontMicro)
            .fontColor(this.tMuted)
            .width('100%')
        }

        if (this.busy) {
          Row({ space: 10 }) {
            LoadingProgress().width(16).height(16).color(this.tAccent)
            Text(this.answer ? 'AI 正在分析…' : 'AI 正在分析…')
              .fontSize(AppTheme.fontCaption)
              .fontColor(this.tMuted)
          }
          .width('100%')
        }

        if (this.err && !(this.answer || '').trim()) {
          Text(this.err)
            .fontSize(AppTheme.fontSmall)
            .fontColor(this.tDanger)
            .width('100%')
        } else if ((this.answer || '').trim() || !this.busy) {
          Text(this.answer || (this.busy ? '' : '等待分析结果'))
            .fontSize(AppTheme.fontSmall)
            .fontColor(this.tText)
            .lineHeight(20)
            .width('100%')
        }

        Row() {
          Blank()
          if (this.busy) {
            Text('停止')
              .fontSize(AppTheme.fontSmall)
              .fontColor(this.tWarning)
              .padding({ left: 12, right: 12, top: 8, bottom: 8 })
              .onClick(() => { this.stop(); })
          } else {
            Text('关闭')
              .fontSize(AppTheme.fontSmall)
              .fontColor(this.tAccent)
              .padding({ left: 12, right: 12, top: 8, bottom: 8 })
              .onClick(() => {
                this.runSeq += 1;
                ApiClient.cancelActiveSse();
                this.onClose();
              })
          }
        }
        .width('100%')
      }
      .width('100%')
      .alignItems(HorizontalAlign.Start)
      .padding({ bottom: 8 })
      .constraintSize({ minHeight: 180 })
    }
  }
}
