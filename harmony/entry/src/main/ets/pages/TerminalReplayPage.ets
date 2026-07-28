import { Api } from '../data/Api';
import { ApiClient } from '../data/ApiClient';
import { FormatUtil } from '../common/FormatUtil';
import { TerminalReplayEvent } from '../data/models/Models';
import {
  EmptyHint,
  ErrorBanner,
  PageHeader,
  PrimaryButton,
  SecondaryButton
} from '../components/CommonUI';
import { NavUtil } from '../common/NavUtil';
import { AppTheme } from '../common/Routes';


@Entry
@Component
struct TerminalReplayPage {
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

  @State sessionId: string = '';
  @State events: TerminalReplayEvent[] = [];
  @State transcript: string = '';
  @State error: string = '';
  @State needVerify: boolean = false;
  @State viewMode: string = 'transcript'; // transcript | events
  @State pw: string = '';
  @State verifying: boolean = false;

  aboutToAppear(): void {
    this.sessionId =
      NavUtil.paramString('sessionId', '') ||
      NavUtil.paramString('id', '');
    void this.load();
  }

  private eventText(ev: TerminalReplayEvent): string {
    return ev.data || ev.payload || '';
  }

  private buildTranscript(list: TerminalReplayEvent[]): string {
    let out = '';
    for (let i = 0; i < list.length; i++) {
      const ev = list[i];
      const typ = (ev.type || ev.kind || '').toLowerCase();
      const data = this.eventText(ev);
      if (!data) {
        continue;
      }
      // 输出帧优先；未知类型也拼上，便于审计
      if (typ === 'o' || typ === 'out' || typ === 'output' || typ === 'stdout' || typ === '') {
        out += data;
      } else if (typ === 'i' || typ === 'in' || typ === 'input') {
        out += data;
      } else {
        out += data;
      }
    }
    return out;
  }

  private async load(): Promise<void> {
    if (!this.sessionId) {
      this.error = '缺少会话 ID';
      return;
    }
    this.error = '';
    this.needVerify = false;
    try {
      const r = await Api.terminalReplay(this.sessionId);
      const list = r.events ?? r.frames ?? [];
      this.events = list;
      this.transcript = this.buildTranscript(list);
    } catch (e) {
      const msg = ApiClient.formatError(e as Object);
      if (msg.indexOf('403') >= 0 || msg.indexOf('密码') >= 0 || msg.indexOf('password') >= 0 ||
        msg.indexOf('unauthorized') >= 0 || msg.indexOf('验证') >= 0) {
        this.needVerify = true;
        this.error = '';
      } else {
        this.error = msg;
      }
    }
  }

  private async verifyThenLoad(): Promise<void> {
    if (!this.pw) {
      this.error = '请输入终端二次密码';
      return;
    }
    if (this.verifying) {
      return;
    }
    this.verifying = true;
    this.error = '';
    try {
      const resp = await Api.verifyTerminalPassword(this.pw);
      const okRaw = resp['ok'];
      const ok = okRaw === true || okRaw === 'true' || okRaw === 1;
      if (!ok) {
        this.error = '密码错误，请重试';
        return;
      }
      this.pw = '';
      this.needVerify = false;
      await this.load();
    } catch (e) {
      this.error = ApiClient.formatError(e as Object);
    } finally {
      this.verifying = false;
    }
  }

  private truncate(s: string | undefined): string {
    const t = (s || '').replace(/\r/g, '\\r').replace(/\n/g, '\\n');
    if (t.length > 200) {
      return t.substring(0, 200) + '…';
    }
    return t || '—';
  }

  build() {
    Column() {
      PageHeader({
        title: '会话回放',
        showBack: true,
        onBack: () => NavUtil.back(),
        trailing: this.needVerify ? '' : '刷新',
        onTrailing: () => {
          void this.load();
        }
      })
      Text(this.sessionId)
        .fontSize(AppTheme.fontCaption)
        .fontColor(this.tMuted)
        .padding({ left: 16, bottom: 8 })
      ErrorBanner({ message: this.error })

      if (this.needVerify) {
        Column() {
          Text('需要终端二次验证')
            .fontSize(AppTheme.fontLead)
            .fontWeight(FontWeight.Bold)
            .fontColor(this.tText)
          Text('终端录制含完整 Shell I/O（可能含敏感输入），需先验证终端二次密码。')
            .fontSize(AppTheme.fontCaption)
            .fontColor(this.tMuted)
            .margin({ top: 8, bottom: 16 })
          TextInput({ text: this.pw, placeholder: '终端二次密码' })
            .type(InputType.Password)
            .onChange((v: string) => {
              this.pw = v;
            })
            .height(42)
            .backgroundColor(this.tElevated)
            .fontColor(this.tText)
          PrimaryButton({
            label: this.verifying ? '验证中…' : '验证并加载',
            isEnabled: !this.verifying,
            onTap: () => {
              void this.verifyThenLoad();
            }
          })
            .margin({ top: 12 })
          SecondaryButton({
            label: '去设置终端密码',
            onTap: () => {
              NavUtil.push('pages/TerminalPasswordPage');
            }
          })
            .margin({ top: 10 })
        }
        .width('100%')
        .padding(20)
        .margin(16)
        .backgroundColor(this.tCard)
        .borderRadius(AppTheme.rSm)
        .layoutWeight(1)
        .justifyContent(FlexAlign.Center)
      } else {
        Row({ space: 8 }) {
          Text('转录')
            .fontSize(AppTheme.fontCaption)
            .fontColor(this.viewMode === 'transcript' ? this.tOnAccent : this.tMuted)
            .padding({ left: 12, right: 12, top: 6, bottom: 6 })
            .backgroundColor(this.viewMode === 'transcript' ? this.tAccent : this.tElevated)
            .borderRadius(AppTheme.rSm)
            .onClick(() => {
              this.viewMode = 'transcript';
            })
          Text('事件')
            .fontSize(AppTheme.fontCaption)
            .fontColor(this.viewMode === 'events' ? this.tOnAccent : this.tMuted)
            .padding({ left: 12, right: 12, top: 6, bottom: 6 })
            .backgroundColor(this.viewMode === 'events' ? this.tAccent : this.tElevated)
            .borderRadius(AppTheme.rSm)
            .onClick(() => {
              this.viewMode = 'events';
            })
        }
        .padding({ left: 16, right: 16, bottom: 8 })

        if (this.events.length === 0) {
          EmptyHint({ text: '该会话无录制内容' })
        } else if (this.viewMode === 'transcript') {
          Scroll() {
            Text(this.transcript || '(无输出)')
              .fontSize(AppTheme.fontCaption)
              .fontFamily('monospace')
              .fontColor('#CDD6E4')
              .width('100%')
          }
          .layoutWeight(1)
          .margin(12)
          .padding(AppTheme.spaceMd)
          .backgroundColor('#0B1020')
          .borderRadius(AppTheme.rXs)
        } else {
          List() {
            ForEach(this.events, (ev: TerminalReplayEvent, idx: number) => {
              ListItem() {
                Column() {
                  Row() {
                    Text(ev.type || ev.kind || 'event')
                      .fontSize(AppTheme.fontCaption)
                      .fontColor(this.tAccent)
                    Blank()
                    Text(FormatUtil.relTime(ev.t))
                      .fontSize(AppTheme.fontMicro)
                      .fontColor(this.tMuted)
                  }
                  .width('100%')
                  Text(this.truncate(this.eventText(ev)))
                    .fontSize(AppTheme.fontCaption)
                    .fontFamily('monospace')
                    .fontColor(this.tText)
                    .margin({ top: 6 })
                }
                .width('100%')
                .padding(AppTheme.spaceMd)
                .backgroundColor(this.tCard)
                .borderRadius(AppTheme.rXs)
              }
              .margin({ left: 16, right: 16, bottom: 8 })
            }, (ev: TerminalReplayEvent, idx: number) => `${idx}-${ev.t ?? 0}`)
          }
          .layoutWeight(1)
        }
      }
    }
    .width('100%')
    .height('100%')
    .backgroundColor(this.tBg)
  }
}
