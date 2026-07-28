import router from '@ohos.router';
import { Api } from '../data/Api';
import { AppTheme } from '../common/Routes';
import { AlertGovernance, AlertRecord, SilenceRule, InhibitRule, NotifyRoute } from '../data/models/Models';
import { EmptyHint, ErrorBanner, PageHeader } from '../components/CommonUI';

@Entry
@Component
struct AlertExtrasPage {
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


  @State tab: number = 0;
  @State history: AlertRecord[] = [];
  @State gov: AlertGovernance = {};
  @State error: string = '';

  async aboutToAppear(): Promise<void> {
    await this.reload();
  }

  async reload(): Promise<void> {
    try {
      this.history = await Api.alertHistory(100);
      this.gov = await Api.alertGovernance();
    } catch (e) {
      this.error = `${e}`;
    }
  }

  async toggleSilence(r: SilenceRule, enabled: boolean): Promise<void> {
    const rules = (this.gov.silence_rules ?? []).map((x) => {
      if (x.id === r.id) {
        // 保留完整规则字段，避免只写 id/name/enabled 冲掉 match（对齐 Android）
        return {
          id: x.id,
          name: x.name,
          enabled: enabled,
          match: x.match,
          matchers: x.matchers,
          time_start: x.time_start,
          time_end: x.time_end,
          weekdays: x.weekdays,
          starts_at: x.starts_at,
          ends_at: x.ends_at,
          comment: x.comment
        } as SilenceRule;
      }
      return x;
    });
    const next: AlertGovernance = {
      silence_rules: rules,
      inhibit_rules: this.gov.inhibit_rules,
      routes: this.gov.routes
    };
    try {
      await Api.setAlertGovernance(next);
      this.gov = next;
    } catch (e) {
      this.error = `${e}`;
    }
  }

  async toggleInhibit(r: InhibitRule, enabled: boolean): Promise<void> {
    const rules = (this.gov.inhibit_rules ?? []).map((x) => {
      if (x.id === r.id) {
        return {
          id: x.id,
          name: x.name,
          enabled: enabled,
          source: x.source,
          target: x.target,
          same_host: x.same_host,
          source_match: x.source_match,
          target_match: x.target_match,
          equal: x.equal
        } as InhibitRule;
      }
      return x;
    });
    const next: AlertGovernance = {
      silence_rules: this.gov.silence_rules,
      inhibit_rules: rules,
      routes: this.gov.routes
    };
    try {
      await Api.setAlertGovernance(next);
      this.gov = next;
    } catch (e) {
      this.error = `${e}`;
    }
  }

  async toggleRoute(r: NotifyRoute, enabled: boolean): Promise<void> {
    const rules = (this.gov.routes ?? []).map((x) => {
      if (x.id === r.id) {
        return {
          id: x.id,
          name: x.name,
          enabled: enabled,
          match: x.match,
          channels: x.channels,
          matchers: x.matchers,
          continue_route: x.continue_route
        } as NotifyRoute;
      }
      return x;
    });
    const next: AlertGovernance = {
      silence_rules: this.gov.silence_rules,
      inhibit_rules: this.gov.inhibit_rules,
      routes: rules
    };
    try {
      await Api.setAlertGovernance(next);
      this.gov = next;
    } catch (e) {
      this.error = `${e}`;
    }
  }

  build() {
    Column() {
      PageHeader({
        title: '告警历史 / 治理',
        showBack: true,
        onBack: () => router.back(),
        trailing: '刷新',
        onTrailing: () => this.reload()
      })
      ErrorBanner({ message: this.error })

      Row({ space: 8 }) {
        Text('历史')
          .fontSize(AppTheme.fontSmall)
          .fontColor(this.tab === 0 ? this.tOnAccent : this.tMuted)
          .padding({ left: 14, right: 14, top: 6, bottom: 6 })
          .backgroundColor(this.tab === 0 ? this.tAccent : this.tCard)
          .borderRadius(AppTheme.rSm)
          .onClick(() => this.tab = 0)
        Text('治理')
          .fontSize(AppTheme.fontSmall)
          .fontColor(this.tab === 1 ? this.tOnAccent : this.tMuted)
          .padding({ left: 14, right: 14, top: 6, bottom: 6 })
          .backgroundColor(this.tab === 1 ? this.tAccent : this.tCard)
          .borderRadius(AppTheme.rSm)
          .onClick(() => this.tab = 1)
      }.padding({ left: 16, bottom: 8 })

      if (this.tab === 0) {
        if (this.history.length === 0) {
          EmptyHint({ text: '暂无历史' })
        } else {
          List() {
            ForEach(this.history, (h: AlertRecord, i: number) => {
              ListItem() {
                Column() {
                  Text(`${h.level ?? ''} · ${h.hostname ?? h.host_id ?? ''}`)
                    .fontSize(AppTheme.fontSmall).fontColor(this.tText)
                  Text(h.message ?? '').fontSize(AppTheme.fontCaption).fontColor(this.tMuted).margin({ top: 4 })
                }.alignItems(HorizontalAlign.Start).padding(AppTheme.spaceMd)
                .backgroundColor(this.tCard).borderRadius(AppTheme.rXs).width('100%')
              }.margin({ left: 16, right: 16, bottom: 8 })
            }, (h: AlertRecord, i: number) => `${i}`)
          }.layoutWeight(1)
        }
      } else {
        List() {
          ForEach(this.gov.silence_rules ?? [], (r: SilenceRule) => {
            ListItem() {
              Row() {
                Text(`静默: ${r.name ?? r.id}`)
                  .fontSize(AppTheme.fontSmall).fontColor(this.tText).layoutWeight(1)
                Toggle({ type: ToggleType.Switch, isOn: r.enabled ?? false })
                  .onChange((v: boolean) => this.toggleSilence(r, v))
              }.padding(AppTheme.spaceMd).backgroundColor(this.tCard).borderRadius(AppTheme.rXs)
            }.margin({ left: 16, right: 16, bottom: 8 })
          }, (r: SilenceRule) => r.id)
          ForEach(this.gov.inhibit_rules ?? [], (r: InhibitRule) => {
            ListItem() {
              Row() {
                Text(`抑制: ${r.name ?? r.id}`)
                  .fontSize(AppTheme.fontSmall).fontColor(this.tText).layoutWeight(1)
                Toggle({ type: ToggleType.Switch, isOn: r.enabled ?? false })
                  .onChange((v: boolean) => {
                    void this.toggleInhibit(r, v);
                  })
              }.padding(AppTheme.spaceMd).backgroundColor(this.tCard).borderRadius(AppTheme.rXs)
            }.margin({ left: 16, right: 16, bottom: 8 })
          }, (r: InhibitRule) => r.id ?? '')
          ForEach(this.gov.routes ?? [], (r: NotifyRoute) => {
            ListItem() {
              Row() {
                Column() {
                  Text(`路由: ${r.name ?? r.id}`)
                    .fontSize(AppTheme.fontSmall).fontColor(this.tText)
                  Text((r.channels ?? []).join(',') || '—')
                    .fontSize(AppTheme.fontMicro).fontColor(this.tMuted).margin({ top: 2 })
                }
                .alignItems(HorizontalAlign.Start)
                .layoutWeight(1)
                Toggle({ type: ToggleType.Switch, isOn: r.enabled ?? true })
                  .onChange((v: boolean) => {
                    void this.toggleRoute(r, v);
                  })
              }.padding(AppTheme.spaceMd).backgroundColor(this.tCard).borderRadius(AppTheme.rXs)
            }.margin({ left: 16, right: 16, bottom: 8 })
          }, (r: NotifyRoute) => r.id ?? '')
        }.layoutWeight(1)
      }
    }.width('100%').height('100%').backgroundColor(this.tBg)
  }
}
