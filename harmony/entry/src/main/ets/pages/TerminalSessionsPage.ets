import { Api } from '../data/Api';
import { ApiClient } from '../data/ApiClient';
import { FormatUtil } from '../common/FormatUtil';
import { TerminalSessionInfo } from '../data/models/Models';
import {
  EmptyHint,
  ErrorBanner,
  PageHeader
} from '../components/CommonUI';
import { NavUtil } from '../common/NavUtil';
import { AppTheme } from '../common/Routes';


@Entry
@Component
struct TerminalSessionsPage {
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

  @State sessions: TerminalSessionInfo[] = [];
  @State error: string = '';

  aboutToAppear(): void {
    void this.load();
  }

  private async load(): Promise<void> {
    this.error = '';
    try {
      this.sessions = await Api.terminalSessions();
    } catch (e) {
      const msg = ApiClient.formatError(e as Object);
      this.error = msg;
      if (msg.indexOf('403') >= 0 || msg.indexOf('密码') >= 0 || msg.indexOf('password') >= 0) {
        this.error = '需要终端二次密码后才能查看会话。请先验证或设置终端密码。';
      }
    }
  }

  private operatorOf(s: TerminalSessionInfo): string {
    return s.operator || s.user || '—';
  }

  private timeOf(s: TerminalSessionInfo): number {
    return s.created_at || s.started_at || 0;
  }

  build() {
    Column() {
      PageHeader({
        title: '终端会话回放',
        showBack: true,
        onBack: () => NavUtil.back(),
        trailingIcon: 'refresh',
        onTrailing: () => {
          void this.load();
        }
      })
      Text('查看已录制的终端会话（需终端二次密码）')
        .fontSize(AppTheme.fontCaption)
        .fontColor(this.tMuted)
        .padding({ left: 16, bottom: 8 })
      ErrorBanner({ message: this.error })
      if (this.error.indexOf('二次密码') >= 0) {
        Text('去设置终端密码')
          .fontSize(AppTheme.fontSmall)
          .fontColor(this.tAccent)
          .padding({ left: 16, bottom: 8 })
          .onClick(() => {
            NavUtil.push('pages/TerminalPasswordPage');
          })
      }
      if (this.sessions.length === 0 && !this.error) {
        EmptyHint({ text: '暂无终端会话记录' })
      }
      List() {
        ForEach(this.sessions, (s: TerminalSessionInfo) => {
          ListItem() {
            Row() {
              Column() {
                Row({ space: 8 }) {
                  Text(s.hostname || s.host_id || s.id)
                    .fontSize(AppTheme.fontBody)
                    .fontWeight(FontWeight.Medium)
                    .fontColor(this.tText)
                    .layoutWeight(1)
                    .maxLines(1)
                    .textOverflow({ overflow: TextOverflow.Ellipsis })
                  if (s.active) {
                    Text('进行中')
                      .fontSize(AppTheme.fontMicro)
                      .fontColor(this.tSuccess)
                      .padding({ left: 8, right: 8, top: 2, bottom: 2 })
                      .backgroundColor(this.tSuccessSoft)
                      .borderRadius(AppTheme.rXs)
                  }
                }
                .width('100%')
                Text(`操作员 ${this.operatorOf(s)}${s.ip ? ` · ${s.ip}` : ''}`)
                  .fontSize(AppTheme.fontCaption)
                  .fontColor(this.tMuted)
                  .margin({ top: 4 })
                Text(`${FormatUtil.relTime(this.timeOf(s))}${s.frames ? ` · ${s.frames} 帧` : ''}${s.observers ? ` · ${s.observers} 观察者` : ''}`)
                  .fontSize(AppTheme.fontMicro)
                  .fontColor(this.tMuted)
                  .margin({ top: 4 })
              }
              .alignItems(HorizontalAlign.Start)
              .layoutWeight(1)
              Text('回放')
                .fontSize(AppTheme.fontCaption)
                .fontColor(this.tAccent)
            }
            .width('100%')
            .padding(AppTheme.cardPad)
            .backgroundColor(this.tCard)
            .borderRadius(AppTheme.rSm)
            .onClick(() => {
              NavUtil.push('pages/TerminalReplayPage', { sessionId: s.id });
            })
          }
          .margin({ left: 16, right: 16, bottom: 10 })
        }, (s: TerminalSessionInfo) => s.id)
      }
      .layoutWeight(1)
      .padding({ top: 8 })
    }
    .width('100%')
    .height('100%')
    .backgroundColor(this.tBg)
  }
}
