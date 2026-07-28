import { AppTheme } from '../common/Routes';
import { NavUtil } from '../common/NavUtil';
import { FormatUtil } from '../common/FormatUtil';
import { AppSymbol, HeaderIconButton } from './AppIcons';

@Component
export struct PageHeader {
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
  @Prop subtitle: string = '';
  @Prop showBack: boolean = false;
  onBack: () => void = () => {
    NavUtil.back();
  };
  @Prop trailing: string = '';
  onTrailing: () => void = () => {};
  @Prop trailing2: string = '';
  onTrailing2: () => void = () => {};
  @Prop trailingIcon: string = '';
  @Prop trailing2Icon: string = '';
  @Prop trailing3Icon: string = '';
  onTrailing3: () => void = () => {};
  /** Android：标题旁小 spinner */
  @Prop showLoading: boolean = false;

  build() {
    Column() {
      Row() {
        if (this.showBack) {
          AppSymbol({ kind: 'back', tint: this.tText, iconSize: 22 })
            .width(40)
            .height(40)
            .onClick(() => this.onBack())
        }
        Column() {
          Row({ space: AppTheme.spaceSm }) {
            Text(this.title)
              .fontSize(AppTheme.fontDisplay)
              .fontWeight(FontWeight.Bold)
              .fontColor(this.tText)
              .maxLines(1)
              .textOverflow({ overflow: TextOverflow.Ellipsis })
            if (this.showLoading) {
              LoadingProgress().width(14).height(14).color(this.tAccent)
            }
          }
          if (this.subtitle) {
            Text(this.subtitle)
              .fontSize(AppTheme.fontCaption)
              .fontColor(this.tMuted)
              .margin({ top: 4 })
              .maxLines(2)
              .textOverflow({ overflow: TextOverflow.Ellipsis })
          }
        }
        .alignItems(HorizontalAlign.Start)
        .layoutWeight(1)
        if (this.trailing3Icon) {
          HeaderIconButton({
            kind: this.trailing3Icon,
            label: '',
            onTap: () => this.onTrailing3()
          })
        }
        if (this.trailing2 || this.trailing2Icon) {
          HeaderIconButton({
            kind: this.trailing2Icon || 'refresh',
            label: this.trailing2,
            onTap: () => this.onTrailing2()
          })
        }
        if (this.trailing || this.trailingIcon) {
          HeaderIconButton({
            kind: this.trailingIcon || 'settings',
            label: this.trailing,
            onTap: () => this.onTrailing()
          })
        }
      }
      .width('100%')
      .padding({ left: AppTheme.pagePadH, right: 10, top: 14, bottom: 12 })
      .alignItems(VerticalAlign.Center)

      Divider().color(this.tBorderSoft).strokeWidth(AppTheme.hairline).opacity(0.9)
    }
    .width('100%')
    .backgroundColor(this.tBg)
  }
}

@Component
export struct SectionLabel {
  @StorageLink('aiops_theme_rev') themeRev: number = 0;
  @StorageLink('aiops_muted') tMuted: string = '#9AA3B8';
  @StorageLink('aiops_accent') tAccent: string = '#4F7FFF';

  @Prop title: string = '';
  @Prop hint: string = '';

  build() {
    Row() {
      Text(this.title)
        .fontSize(AppTheme.fontCaption)
        .fontWeight(FontWeight.Medium)
        .fontColor(this.tMuted)
        .letterSpacing(0.3)
      Blank()
      if (this.hint) {
        Text(this.hint)
          .fontSize(AppTheme.fontMicro)
          .fontColor(this.tAccent)
      }
    }
    .width('100%')
    .padding({
      left: AppTheme.pagePadH,
      right: AppTheme.pagePadH,
      top: AppTheme.spaceMd,
      bottom: AppTheme.spaceXs
    })
  }
}

@Component
export struct SurfaceCard {
  @StorageLink('aiops_theme_rev') themeRev: number = 0;
  @StorageLink('aiops_card') tCard: string = '#111722';
  @StorageLink('aiops_border_soft') tBorderSoft: string = '#222C3D';
  @StorageLink('aiops_shadow') tShadow: string = '#40000000';

  @Prop elevated: boolean = false;
  @Prop pad: number = AppTheme.cardPad;
  @BuilderParam content: () => void = this.defaultContent;

  @Builder
  defaultContent() {
  }

  build() {
    Column() {
      this.content()
    }
    .width('100%')
    .padding(this.pad)
    .backgroundColor(this.tCard)
    .borderRadius(AppTheme.rMd)
    .border({ width: AppTheme.hairline, color: this.tBorderSoft })
    .shadow(this.elevated
      ? { radius: 10, color: this.tShadow, offsetY: 3 }
      : { radius: 4, color: this.tShadow, offsetY: 1 })
  }
}

/** ~15% / ~40% alpha border of accent — Android StatCard / HostCard border style */
function softAccentBorder(accent: string, strong: boolean = false): string {
  const hex = (accent || '').replace('#', '');
  if (hex.length === 6) {
    return `${strong ? '#66' : '#26'}${hex}`;
  }
  return AppTheme.borderSoft;
}

@Component
export struct StatCard {
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
  value: string = '';
  color: string = AppTheme.brand;
  selected: boolean = false;
  onTap: () => void = () => {};

  build() {
    Column() {
      Text(this.value)
        .fontSize(AppTheme.fontDisplay)
        .fontWeight(FontWeight.Bold)
        .fontColor(this.color)
        .maxLines(1)
        .textOverflow({ overflow: TextOverflow.Ellipsis })
      Text(this.label)
        .fontSize(AppTheme.fontMicro)
        .fontWeight(FontWeight.Medium)
        .fontColor(this.tMuted)
        .margin({ top: AppTheme.spaceXs })
    }
    .layoutWeight(1)
    .padding({ top: AppTheme.spaceMd, bottom: AppTheme.spaceMd, left: 6, right: 6 })
    .justifyContent(FlexAlign.Center)
    .alignItems(HorizontalAlign.Center)
    .backgroundColor(this.tCard)
    .borderRadius(AppTheme.rMd)
    .border({
      width: AppTheme.hairline,
      color: softAccentBorder(this.color, this.selected)
    })
    .shadow({ radius: 6, color: this.tShadow, offsetY: 2 })
    .onClick(() => this.onTap())
  }
}

@Component
export struct EmptyHint {
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


  text: string = '暂无数据';
  /** 填满剩余视口并垂直居中（告警空态等） */
  fill: boolean = false;

  build() {
    Column({ space: AppTheme.spaceSm }) {
      Text(this.text)
        .fontSize(AppTheme.fontBody)
        .fontColor(this.tMuted)
        .width('100%')
        .textAlign(TextAlign.Center)
        .lineHeight(22)
        .constraintSize({ maxWidth: 280 })
    }
    .width('100%')
    .padding({ left: AppTheme.spaceXl, right: AppTheme.spaceXl, top: AppTheme.spaceXl, bottom: AppTheme.spaceXl })
    .justifyContent(FlexAlign.Center)
    .alignItems(HorizontalAlign.Center)
    .layoutWeight(this.fill ? 1 : 0)
  }
}

@Component
export struct ErrorBanner {
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


  message: string = '';
  showRetry: boolean = false;
  onRetry: () => void = () => {};

  build() {
    if (this.message) {
      Column() {
        Text(this.message)
          .fontSize(AppTheme.fontCaption)
          .fontColor(this.tDanger)
          .width('100%')
          .lineHeight(18)
        if (this.showRetry) {
          Text('点击重试')
            .fontSize(AppTheme.fontCaption)
            .fontWeight(FontWeight.Medium)
            .fontColor(this.tAccent)
            .margin({ top: 6 })
            .onClick(() => this.onRetry())
        }
      }
      .width('100%')
      .padding(AppTheme.spaceMd)
      .backgroundColor(this.tDangerSoft)
      .borderRadius(AppTheme.rSm)
      .border({ width: AppTheme.hairline, color: this.tDanger })
      .margin({ left: AppTheme.pagePadH, right: AppTheme.pagePadH, bottom: AppTheme.spaceSm })
      .alignItems(HorizontalAlign.Start)
    }
  }
}

@Component
export struct LoadingBlock {
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


  text: string = '加载中…';
  fill: boolean = false;

  build() {
    Column() {
      LoadingProgress().width(36).height(36).color(this.tAccent)
      Text(this.text)
        .fontSize(AppTheme.fontCaption)
        .fontColor(this.tMuted)
        .margin({ top: AppTheme.spaceMd })
    }
    .width('100%')
    .padding(AppTheme.spaceXl)
    .justifyContent(FlexAlign.Center)
    .alignItems(HorizontalAlign.Center)
    .layoutWeight(this.fill ? 1 : 0)
  }
}

@Component
export struct FilterChips {
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


  @Prop labels: string[] = [];
  @Prop counts: number[] = [];
  /** filter=Android FilterChip；tab=主 Tab 14sp；subtab=二级 13sp */
  @Prop variant: string = 'filter';
  @Link selected: number;
  onSelect: (i: number) => void = (_i: number) => {};

  private labelAt(i: number): string {
    const base = this.labels[i] || '';
    if (this.counts.length > i && this.counts[i] >= 0) {
      return `${base} ${this.counts[i]}`;
    }
    return base;
  }

  private fontSizeOf(): number {
    if (this.variant === 'tab') {
      return AppTheme.fontBody;
    }
    if (this.variant === 'subtab') {
      return 13;
    }
    return AppTheme.fontCaption;
  }

  build() {
    Scroll() {
      Row({ space: this.variant === 'tab' || this.variant === 'subtab' ? AppTheme.spaceXs : AppTheme.spaceSm }) {
        ForEach(this.labels, (t: string, i: number) => {
          Text(this.labelAt(i))
            .fontSize(this.fontSizeOf())
            .fontWeight(this.selected === i ? FontWeight.Medium : FontWeight.Regular)
            .fontColor(this.selected === i
              ? (this.variant === 'filter' ? this.tAccentOn : this.tAccent)
              : this.tMuted)
            .padding({
              left: this.variant === 'filter' ? AppTheme.spaceMd : 14,
              right: this.variant === 'filter' ? AppTheme.spaceMd : 14,
              top: 10,
              bottom: 10
            })
            .backgroundColor(this.selected === i && this.variant === 'filter'
              ? this.tAccentSoft
              : Color.Transparent)
            .borderRadius(this.variant === 'filter' ? AppTheme.rMd : 0)
            .border({
              width: {
                left: 0,
                right: 0,
                top: 0,
                bottom: this.selected === i && (this.variant === 'tab' || this.variant === 'subtab') ? 2 : 0
              },
              color: this.tAccent
            })
            .onClick(() => {
              this.selected = i;
              this.onSelect(i);
            })
        }, (t: string, i: number) => `${i}-${t}`)
      }
      .padding({ right: 4 })
    }
    .scrollable(ScrollDirection.Horizontal)
    .scrollBar(BarState.Off)
    .edgeEffect(EdgeEffect.Spring)
    .width('100%')
  }
}

@Component
export struct SearchBar {
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


  @Link text: string;
  placeholder: string = '搜索';
  onSubmit: () => void = () => {};

  build() {
    TextInput({ text: this.text, placeholder: this.placeholder })
      .height(44)
      .fontSize(AppTheme.fontBody)
      .backgroundColor(this.tElevated)
      .fontColor(this.tText)
      .placeholderColor(this.tMuted)
      .borderRadius(AppTheme.rMd)
      .border({ width: AppTheme.hairline, color: this.tBorderSoft })
      .padding({ left: AppTheme.spaceMd, right: AppTheme.spaceMd })
      .onChange((v: string) => {
        this.text = v;
      })
      .onSubmit(() => this.onSubmit())
  }
}

@Component
export struct SectionHeader {
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
  actionLabel: string = '';
  onAction: () => void = () => {};

  build() {
    Row() {
      Text(this.title)
        .fontSize(AppTheme.fontCaption)
        .fontWeight(FontWeight.Bold)
        .fontColor(this.tMuted)
        .letterSpacing(0.6)
        .layoutWeight(1)
      if (this.actionLabel) {
        Text(this.actionLabel)
          .fontSize(AppTheme.fontCaption)
          .fontWeight(FontWeight.Medium)
          .fontColor(this.tAccent)
          .onClick(() => this.onAction())
      }
    }
    .width('100%')
    .padding({ left: AppTheme.pagePadH, right: AppTheme.pagePadH, top: 12, bottom: 8 })
  }
}

@Component
export struct ActionRow {
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
  trailing: string = '›';
  danger: boolean = false;
  onTap: () => void = () => {};

  build() {
    Row() {
      Column() {
        Text(this.title)
          .fontSize(AppTheme.fontSubtitle)
          .fontColor(this.danger ? this.tDanger : this.tText)
        if (this.subtitle) {
          Text(this.subtitle)
            .fontSize(AppTheme.fontCaption)
            .fontColor(this.tMuted)
            .margin({ top: 4 })
        }
      }
      .alignItems(HorizontalAlign.Start)
      .layoutWeight(1)
      Text(this.trailing).fontSize(AppTheme.fontLead).fontColor(this.tMuted)
    }
    .width('100%')
    .padding(AppTheme.cardPad)
    .backgroundColor(this.tCard)
    .borderRadius(AppTheme.rSm)
    .border({ width: 1, color: this.tBorderSoft })
    .onClick(() => this.onTap())
  }
}

@Component
export struct SeverityBadge {
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


  level: string = '';

  build() {
    Text(FormatUtil.levelLabel(this.level))
      .fontSize(AppTheme.fontMicro)
      .fontWeight(FontWeight.Bold)
      .fontColor(FormatUtil.sevColor(this.level))
      .padding({ left: 8, right: 8, top: 3, bottom: 3 })
      .backgroundColor(FormatUtil.sevSoft(this.level))
      .borderRadius(AppTheme.rChip)
  }
}

@Component
export struct StatusDot {
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


  @Prop online: boolean = false;
  @Prop dotSize: number = 10;

  build() {
    Circle({ width: this.dotSize, height: this.dotSize })
      .fill(this.online ? this.tSuccess : this.tDanger)
  }
}

@Component
export struct HostCard {
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


  @Prop hostname: string = '';
  @Prop ip: string = '';
  @Prop online: boolean = false;
  @Prop cpu: string = '';
  @Prop mem: string = '';
  @Prop disk: string = '';
  @Prop cpuPct: number = -1;
  @Prop memPct: number = -1;
  @Prop diskPct: number = -1;
  @Prop category: string = '';
  @Prop gpuCount: number = 0;
  @Prop gpuUtil: number = -1;
  @Prop gpuMem: number = -1;
  @Prop gpuTemp: number = -1;
  @Prop gpuDetail: string = '';
  @Prop hasLatest: boolean = false;
  onTap: () => void = () => {};

  private statusColor(): string {
    return this.online ? AppTheme.brandGreen : AppTheme.brandRed;
  }

  private parsePct(raw: string, fallback: number): number {
    if (fallback >= 0) {
      return fallback;
    }
    if (!raw) {
      return -1;
    }
    const n = Number(raw.replace('%', '').trim());
    return isNaN(n) ? -1 : n;
  }

  private cpuValue(): number {
    return this.parsePct(this.cpu, this.cpuPct);
  }

  private memValue(): number {
    return this.parsePct(this.mem, this.memPct);
  }

  private diskValue(): number {
    return this.parsePct(this.disk, this.diskPct);
  }

  private hasMetrics(): boolean {
    return this.cpuValue() >= 0 || this.memValue() >= 0 || this.diskValue() >= 0;
  }

  build() {
    Column() {
      Row() {
        StatusDot({ online: this.online }).margin({ right: 8 })
        Text(this.hostname || '—')
          .fontSize(AppTheme.fontLead)
          .fontColor(this.tText)
          .fontWeight(FontWeight.Bold)
          .layoutWeight(1)
          .maxLines(1)
          .textOverflow({ overflow: TextOverflow.Ellipsis })
      }
      .width('100%')

      Row() {
        Text(this.ip || '未知 IP')
          .fontSize(AppTheme.fontCaption)
          .fontColor(this.tMuted)
          .layoutWeight(1)
          .maxLines(1)
        Text(this.online ? '在线' : '离线')
          .fontSize(AppTheme.fontCaption)
          .fontWeight(FontWeight.Medium)
          .fontColor(this.statusColor())
        if (this.category) {
          Text(this.category.toUpperCase())
            .fontSize(AppTheme.fontTiny)
            .fontWeight(FontWeight.Bold)
            .fontColor(AppTheme.brand)
            .padding({ left: 8, right: 8, top: 3, bottom: 3 })
            .backgroundColor(this.tAccentSoft)
            .borderRadius(AppTheme.rChip)
            .margin({ left: 8 })
        }
      }
      .width('100%')
      .margin({ top: 4 })

      if (this.hasMetrics()) {
        Row({ space: 12 }) {
          MetricBar({
            label: 'CPU',
            value: this.cpuValue() >= 0 ? this.cpuValue() : 0,
            valueText: this.cpu || `${Math.round(this.cpuValue())}%`,
            compact: true
          })
            .layoutWeight(1)
          MetricBar({
            label: '内存',
            value: this.memValue() >= 0 ? this.memValue() : 0,
            valueText: this.mem || `${Math.round(this.memValue())}%`,
            compact: true
          })
            .layoutWeight(1)
          MetricBar({
            label: '磁盘',
            value: this.diskValue() >= 0 ? this.diskValue() : 0,
            valueText: this.disk || `${Math.round(this.diskValue())}%`,
            compact: true
          })
            .layoutWeight(1)
        }
        .width('100%')
        .margin({ top: 14 })

        if (this.gpuCount > 0) {
          Divider().color(this.tBorderSoft).strokeWidth(0.5).margin({ top: 12, bottom: 12 })
          Row({ space: 12 }) {
            MetricBar({
              label: `GPU ×${this.gpuCount}`,
              value: this.gpuUtil >= 0 ? this.gpuUtil : 0,
              valueText: `${Math.round(this.gpuUtil >= 0 ? this.gpuUtil : 0)}%`,
              compact: true
            }).layoutWeight(1)
            MetricBar({
              label: 'GPU 显存',
              value: this.gpuMem >= 0 ? this.gpuMem : 0,
              valueText: `${Math.round(this.gpuMem >= 0 ? this.gpuMem : 0)}%`,
              compact: true
            }).layoutWeight(1)
            MetricBar({
              label: 'GPU 温度',
              value: this.gpuTemp > 0 ? this.gpuTemp : 0,
              valueText: this.gpuTemp > 0 ? `${Math.round(this.gpuTemp)}°C` : '—',
              compact: true
            }).layoutWeight(1)
          }
          .width('100%')
          if (this.gpuDetail) {
            Text(this.gpuDetail)
              .fontSize(AppTheme.fontTiny)
              .fontColor(this.tMuted)
              .margin({ top: 5 })
              .width('100%')
              .maxLines(1)
              .textOverflow({ overflow: TextOverflow.Ellipsis })
          }
        }
      } else if (this.cpu || this.mem || this.disk) {
        Row({ space: 10 }) {
          if (this.cpu) {
            Text(`CPU ${this.cpu}`)
              .fontSize(AppTheme.fontMicro)
              .fontColor(this.tAccent)
              .padding({ left: 8, right: 8, top: 3, bottom: 3 })
              .backgroundColor(this.tAccentSoft)
              .borderRadius(AppTheme.rChip)
          }
          if (this.mem) {
            Text(`MEM ${this.mem}`)
              .fontSize(AppTheme.fontMicro)
              .fontColor(this.tMuted)
              .padding({ left: 8, right: 8, top: 3, bottom: 3 })
              .backgroundColor(this.tElevated)
              .borderRadius(AppTheme.rChip)
          }
          if (this.disk) {
            Text(`DISK ${this.disk}`)
              .fontSize(AppTheme.fontMicro)
              .fontColor(this.tMuted)
              .padding({ left: 8, right: 8, top: 3, bottom: 3 })
              .backgroundColor(this.tElevated)
              .borderRadius(AppTheme.rChip)
          }
        }
        .margin({ top: 12 })
      } else {
        Text(this.hasLatest ? '等待 Agent 数据...' : '等待 Agent 数据...')
          .fontSize(AppTheme.fontTiny)
          .fontColor(this.tMuted)
          .margin({ top: 12 })
      }
    }
    .width('100%')
    .padding(AppTheme.cardPad)
    .backgroundColor(this.tCard)
    .borderRadius(AppTheme.rSm)
    .borderWidth({ left: 4, right: 1, top: 1, bottom: 1 })
    .borderColor({
      left: this.statusColor(),
      right: softAccentBorder(this.statusColor()),
      top: softAccentBorder(this.statusColor()),
      bottom: softAccentBorder(this.statusColor())
    })
    .alignItems(HorizontalAlign.Start)
    .shadow({ radius: 6, color: this.tShadow, offsetY: 1 })
    .onClick(() => this.onTap())
  }
}

@Component
export struct KeyValueRow {
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
  value: string = '';

  build() {
    Row() {
      Text(this.label).fontSize(AppTheme.fontSmall).fontColor(this.tMuted).width(100)
      Text(this.value).fontSize(AppTheme.fontSmall).fontColor(this.tText).layoutWeight(1)
    }
    .width('100%')
    .padding({ top: 4, bottom: 4 })
  }
}

@Component
export struct PrimaryButton {
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


  /** 必须 @Prop：父组件用户名/密码/服务器变化后才会刷新启用态（否则登录钮会一直灰） */
  @Prop label: string = '';
  @Prop isEnabled: boolean = true;
  @Prop loading: boolean = false;
  onTap: () => void = () => {};

  build() {
    Button() {
      Row({ space: 8 }) {
        if (this.loading) {
          LoadingProgress().width(18).height(18).color(this.tOnAccent)
        } else {
          Text(this.label)
            .fontSize(AppTheme.fontSubtitle)
            .fontWeight(FontWeight.Bold)
            .fontColor(this.tOnAccent)
        }
      }
    }
    .width('100%')
    .height(48)
    .enabled(this.isEnabled && !this.loading)
    .backgroundColor(this.isEnabled && !this.loading ? this.tAccent : this.tBorder)
    .borderRadius(AppTheme.rSm)
    .opacity(this.isEnabled && !this.loading ? 1 : 0.55)
    .onClick(() => {
      if (this.isEnabled && !this.loading) {
        this.onTap();
      }
    })
  }
}

@Component
export struct SecondaryButton {
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
    Button(this.label)
      .width('100%')
      .height(44)
      .backgroundColor(this.tElevated)
      .fontColor(this.tText)
      .borderRadius(AppTheme.rSm)
      .border({ width: 1, color: this.tBorder })
      .onClick(() => this.onTap())
  }
}

@Component
export struct MetricBar {
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


  @Prop label: string = '';
  @Prop value: number = 0;
  @Prop valueText: string = '';
  @Prop detailText: string = '';
  @Prop color: string = '';
  @Prop compact: boolean = false;

  private barColor(): string {
    if (this.color) {
      return this.color;
    }
    return FormatUtil.thresholdColor(this.value) || AppTheme.brand;
  }

  private displayValue(): string {
    if (this.valueText) {
      return this.valueText;
    }
    return `${this.value.toFixed(1)}%`;
  }

  build() {
    Column() {
      Row() {
        Text(this.label)
          .fontSize(AppTheme.fontMicro)
          .fontColor(this.tMuted)
          .layoutWeight(1)
          .maxLines(1)
        Text(this.displayValue())
          .fontSize(AppTheme.fontMicro)
          .fontWeight(FontWeight.Bold)
          .fontColor(this.barColor())
      }
      .width('100%')

      if (this.detailText) {
        Text(this.detailText)
          .fontSize(AppTheme.fontTiny)
          .fontColor(this.tMuted)
          .width('100%')
          .margin({ top: 2 })
          .maxLines(1)
          .textOverflow({ overflow: TextOverflow.Ellipsis })
      }

      Stack({ alignContent: Alignment.Start }) {
        Row()
          .width('100%')
          .height(8)
          .backgroundColor(this.tBorderSoft)
          .borderRadius(AppTheme.rChip)
        Row()
          .width(`${Math.min(100, Math.max(0, this.value))}%`)
          .height(8)
          .backgroundColor(this.barColor())
          .borderRadius(AppTheme.rChip)
          .opacity(0.92)
      }
      .width('100%')
      .height(8)
      .margin({ top: 6 })
      .clip(true)
      .borderRadius(AppTheme.rChip)
    }
    .width('100%')
    .margin({ bottom: this.compact ? 0 : 8 })
  }
}

/** Android SectionCard：uppercase primary 标题 + surface 卡片 */
@Component
export struct SectionCard {
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
  @BuilderParam content: () => void = this.emptyBody;

  @Builder
  emptyBody() {
    Text('').height(1)
  }

  build() {
    Column() {
      if (this.title) {
        Text(this.title.toUpperCase())
          .fontSize(AppTheme.fontCaption)
          .fontWeight(FontWeight.Bold)
          .fontColor(this.tAccent)
          .letterSpacing(1)
          .width('100%')
          .margin({ bottom: 12 })
      }
      this.content()
    }
    .width('100%')
    .padding(AppTheme.spaceMd)
    .backgroundColor(this.tCard)
    .borderRadius(AppTheme.rMd)
    .border({ width: 1, color: this.tBorderSoft })
    .alignItems(HorizontalAlign.Start)
  }
}

@Component
export struct ConfirmDialogHost {
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
  title: string = '确认';
  message: string = '';
  confirmText: string = '确定';
  cancelText: string = '取消';
  danger: boolean = false;
  onConfirm: () => void = () => {};
  onCancel: () => void = () => {};

  build() {
    if (this.visible) {
      Column() {
        Column() {
          Text(this.title)
            .fontSize(AppTheme.fontLead)
            .fontWeight(FontWeight.Bold)
            .fontColor(this.tText)
          Text(this.message)
            .fontSize(AppTheme.fontSmall)
            .fontColor(this.tMuted)
            .margin({ top: 10, bottom: 16 })
          Row({ space: 12 }) {
            Button(this.cancelText)
              .layoutWeight(1)
              .height(44)
              .backgroundColor(this.tElevated)
              .fontColor(this.tText)
              .borderRadius(AppTheme.rSm)
              .border({ width: 1, color: this.tBorderSoft })
              .onClick(() => this.onCancel())
            Button(this.confirmText)
              .layoutWeight(1)
              .height(44)
              .backgroundColor(this.danger ? this.tDanger : this.tAccent)
              .fontColor(this.tOnAccent)
              .borderRadius(AppTheme.rSm)
              .onClick(() => this.onConfirm())
          }
          .width('100%')
        }
        .width('84%')
        .padding(20)
        .backgroundColor(this.tCard)
        .borderRadius(AppTheme.rMd)
        .border({ width: 1, color: this.tBorderSoft })
      }
      .width('100%')
      .height('100%')
      .justifyContent(FlexAlign.Center)
      .backgroundColor(this.tScrim)
      .position({ x: 0, y: 0 })
      .zIndex(100)
      .onClick(() => this.onCancel())
    }
  }
}

@Component
export struct MenuSheet {
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
  @Prop items: string[] = [];
  onSelect: (i: number) => void = (_i: number) => {};
  onClose: () => void = () => {};

  build() {
    if (this.visible) {
      Column() {
        Blank()
          .layoutWeight(1)
          .onClick(() => this.onClose())
        Column() {
          if (this.title) {
            Text(this.title)
              .fontSize(AppTheme.fontBody)
              .fontColor(this.tMuted)
              .width('100%')
              .margin({ bottom: 8 })
          }
          ForEach(this.items, (it: string, i: number) => {
            Text(it)
              .fontSize(AppTheme.fontSubtitle)
              .fontColor(this.tText)
              .width('100%')
              .padding({ top: 14, bottom: 14 })
              .onClick(() => {
                this.onSelect(i);
                this.onClose();
              })
          }, (it: string, i: number) => `${i}-${it}`)
          Text('取消')
            .fontSize(AppTheme.fontSubtitle)
            .fontColor(this.tMuted)
            .width('100%')
            .textAlign(TextAlign.Center)
            .padding({ top: 16, bottom: 8 })
            .onClick(() => this.onClose())
        }
        .width('100%')
        .padding({ left: 16, right: 16, top: 16, bottom: 24 })
        .backgroundColor(this.tCard)
        .borderRadius({ topLeft: AppTheme.rMd, topRight: AppTheme.rMd })
        .border({ width: { top: 1 }, color: this.tBorderSoft })
      }
      .width('100%')
      .height('100%')
      .backgroundColor(this.tScrim)
      .position({ x: 0, y: 0 })
      .zIndex(99)
    }
  }
}
