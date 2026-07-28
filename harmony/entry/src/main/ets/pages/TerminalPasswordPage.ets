import { Api } from '../data/Api';
import { ApiClient } from '../data/ApiClient';
import {
  ErrorBanner,
  KeyValueRow,
  PageHeader,
  PrimaryButton
} from '../components/CommonUI';
import { NavUtil } from '../common/NavUtil';
import { AppTheme } from '../common/Routes';


@Entry
@Component
struct TerminalPasswordPage {
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


  @State hasPassword: string = '未知';
  @State required: string = '未知';
  @State password: string = '';
  @State confirmPwd: string = '';
  @State verifyPwd: string = '';
  @State statusMsg: string = '';
  @State error: string = '';
  @State strengthHints: string[] = [];

  aboutToAppear(): void {
    void this.loadStatus();
  }

  private unmet(p: string): string[] {
    const reqs: string[] = [];
    if (p.length < 8) {
      reqs.push('至少 8 位');
    }
    let hasUpper = false;
    let hasLower = false;
    let hasDigit = false;
    let hasSpecial = false;
    for (let i = 0; i < p.length; i++) {
      const c = p.charAt(i);
      if (c >= 'A' && c <= 'Z') {
        hasUpper = true;
      } else if (c >= 'a' && c <= 'z') {
        hasLower = true;
      } else if (c >= '0' && c <= '9') {
        hasDigit = true;
      } else {
        hasSpecial = true;
      }
    }
    if (!hasUpper) {
      reqs.push('需包含大写字母');
    }
    if (!hasLower) {
      reqs.push('需包含小写字母');
    }
    if (!hasDigit) {
      reqs.push('需包含数字');
    }
    if (!hasSpecial) {
      reqs.push('需包含特殊符号');
    }
    return reqs;
  }

  private async loadStatus(): Promise<void> {
    this.error = '';
    try {
      const s = await Api.terminalPasswordStatus();
      const hasVal: ESObject = s['has_password'] ?? s['set'] ?? s['configured'];
      const reqVal: ESObject = s['required'] ?? s['enabled'];
      const hasRaw: string = String(hasVal ?? '');
      const reqRaw: string = String(reqVal ?? '');
      this.hasPassword =
        hasVal === true || hasRaw === 'true' || hasRaw === '1' || hasRaw === 'True' ? '已设置' : '未设置';
      this.required =
        reqVal === true || reqRaw === 'true' || reqRaw === '1' || reqRaw === 'True' ? '需要' : '可选';
      this.statusMsg = '';
    } catch (e) {
      this.error = ApiClient.formatError(e as Object);
    }
  }

  private async setPassword(): Promise<void> {
    if (!this.password) {
      this.error = '请输入新终端密码';
      return;
    }
    const unmet = this.unmet(this.password);
    if (unmet.length > 0) {
      this.error = unmet.join('；');
      return;
    }
    if (this.confirmPwd !== this.password) {
      this.error = '两次输入的密码不一致';
      return;
    }
    try {
      await Api.setTerminalPassword(this.password);
      this.password = '';
      this.confirmPwd = '';
      this.strengthHints = [];
      this.statusMsg = '终端密码已更新';
      await this.loadStatus();
    } catch (e) {
      this.error = ApiClient.formatError(e as Object);
    }
  }

  private async verify(): Promise<void> {
    if (!this.verifyPwd) {
      this.error = '请输入密码以验证';
      return;
    }
    try {
      await Api.verifyTerminalPassword(this.verifyPwd);
      this.verifyPwd = '';
      this.statusMsg = '验证成功';
      NavUtil.back();
    } catch (e) {
      this.error = ApiClient.formatError(e as Object);
    }
  }

  build() {
    Column() {
      PageHeader({
        title: this.hasPassword === '已设置' ? '修改终端密码' : '设置终端密码',
        showBack: true,
        onBack: () => NavUtil.back()
      })
      ErrorBanner({ message: this.error })
      if (this.statusMsg) {
        Text(this.statusMsg)
          .fontSize(AppTheme.fontSmall)
          .fontColor(this.tSuccess)
          .padding({ left: 16, bottom: 8 })
      }

      KeyValueRow({ label: '状态', value: this.hasPassword })
        .padding({ left: 16, right: 16, top: 8 })
      KeyValueRow({ label: '策略', value: this.required })
        .padding({ left: 16, right: 16, bottom: 12 })

      Text('设置 / 修改密码')
        .fontSize(AppTheme.fontSmall)
        .fontColor(this.tMuted)
        .padding({ left: 16 })
      Text('终端二次密码与登录密码相互独立，用于打开远程终端。')
        .fontSize(AppTheme.fontCaption)
        .fontColor(this.tMuted)
        .padding({ left: 16, right: 16, top: 4, bottom: 4 })
      TextInput({ text: this.password, placeholder: '新终端密码' })
        .type(InputType.Password)
        .onChange((v: string) => {
          this.password = v;
          this.strengthHints = v ? this.unmet(v) : [];
        })
        .margin({ left: 16, right: 16, top: 8, bottom: 8 })
        .height(40)
        .backgroundColor(this.tElevated)
        .fontColor(this.tText)
      TextInput({ text: this.confirmPwd, placeholder: '确认新密码' })
        .type(InputType.Password)
        .onChange((v: string) => {
          this.confirmPwd = v;
        })
        .margin({ left: 16, right: 16, bottom: 8 })
        .height(40)
        .backgroundColor(this.tElevated)
        .fontColor(this.tText)
      if (this.strengthHints.length > 0) {
        ForEach(this.strengthHints, (h: string, i: number) => {
          Text(`· ${h}`)
            .fontSize(AppTheme.fontCaption)
            .fontColor(this.tDanger)
            .padding({ left: 16 })
        }, (h: string, i: number) => `st-${i}`)
      }
      if (this.confirmPwd && this.confirmPwd !== this.password) {
        Text('· 两次输入的密码不一致')
          .fontSize(AppTheme.fontCaption)
          .fontColor(this.tDanger)
          .padding({ left: 16, bottom: 4 })
      }
      PrimaryButton({
        label: '保存密码',
        onTap: () => {
          void this.setPassword();
        }
      })
        .padding({ left: 16, right: 16, bottom: 16 })

      Text('验证密码')
        .fontSize(AppTheme.fontSmall)
        .fontColor(this.tMuted)
        .padding({ left: 16 })
      TextInput({ text: this.verifyPwd, placeholder: '输入当前终端密码' })
        .type(InputType.Password)
        .onChange((v: string) => {
          this.verifyPwd = v;
        })
        .margin(16)
        .height(40)
        .backgroundColor(this.tElevated)
        .fontColor(this.tText)
      PrimaryButton({
        label: '验证并返回',
        onTap: () => {
          void this.verify();
        }
      })
        .padding({ left: 16, right: 16, bottom: 24 })
    }
    .width('100%')
    .height('100%')
    .backgroundColor(this.tBg)
  }
}
