import { Api } from '../data/Api';
import { ApiClient } from '../data/ApiClient';
import { AppState } from '../common/AppState';
import { AppTheme } from '../common/Routes';
import { ServerEntry, SessionStore } from '../data/SessionStore';
import { ConfirmDialogHost, PrimaryButton } from './CommonUI';
import { AppSymbol } from './AppIcons';

/**
 * Android LoginScreen + EnvSwitcherDialog 1:1 登录面板。
 * Index LoginEmbed / LoginPage 共用。
 */
@Component
export struct LoginPanel {
  @StorageLink('aiops_theme_rev') themeRev: number = 0;

  @StorageLink('aiops_bg') tBg: string = '#080C14';
  @StorageLink('aiops_card') tCard: string = '#111722';
  @StorageLink('aiops_elevated') tElevated: string = '#192130';
  @StorageLink('aiops_text') tText: string = '#E6E8F0';
  @StorageLink('aiops_muted') tMuted: string = '#9AA3B8';
  @StorageLink('aiops_accent') tAccent: string = '#4F7FFF';
  @StorageLink('aiops_accent_soft') tAccentSoft: string = '#1E3166';
  @StorageLink('aiops_success') tSuccess: string = '#00A86B';
  @StorageLink('aiops_success_soft') tSuccessSoft: string = '#0F3A2C';
  @StorageLink('aiops_warning') tWarning: string = '#F59E0B';
  @StorageLink('aiops_danger') tDanger: string = '#EF4444';
  @StorageLink('aiops_border') tBorder: string = '#39445A';
  @StorageLink('aiops_border_soft') tBorderSoft: string = '#222C3D';
  @StorageLink('aiops_on_accent') tOnAccent: string = '#FFFFFF';
  @StorageLink('aiops_scrim') tScrim: string = '#99000000';

  onSuccess: () => void = () => {};

  @State server: string = '';
  @State username: string = '';
  @State password: string = '';
  @State mfa: string = '';
  @State mfaUser: string = '';
  @State mfaPass: string = '';
  @State showMfaDialog: boolean = false;
  @State loading: boolean = false;
  @State error: string = '';
  @State envError: string = '';
  @State servers: ServerEntry[] = [];
  @State showPwd: boolean = false;
  @State showEnv: boolean = false;
  @State showEnvForm: boolean = false;
  @State envEditingUrl: string = '';
  @State envDraft: string = '';
  @State envLabel: string = '';
  @State envDeleteTarget: string = '';
  @State envDeleteLabel: string = '';
  @State showEnvDelete: boolean = false;
  @State tapCount: number = 0;
  @State lastTapAt: number = 0;
  @State userFocused: boolean = false;
  @State passFocused: boolean = false;

  aboutToAppear(): void {
    void this.boot();
  }

  private async boot(): Promise<void> {
    this.server = await SessionStore.getBaseUrl();
    this.username = await SessionStore.getLastUsername();
    this.servers = await SessionStore.getServerEntries();
    if (AppState.statusText && !AppState.needsRetry) {
      this.error = AppState.statusText;
    }
  }

  private async refreshServers(): Promise<void> {
    this.servers = await SessionStore.getServerEntries();
    this.server = await SessionStore.getBaseUrl();
  }

  private onSecretTap(): void {
    const now = Date.now();
    // Android：1.5s 内连点 3 次
    if (now - this.lastTapAt >= 1500) {
      this.tapCount = 0;
    }
    this.lastTapAt = now;
    this.tapCount += 1;
    if (this.tapCount >= 3) {
      this.tapCount = 0;
      this.envError = '';
      this.showEnvForm = false;
      this.envEditingUrl = '';
      this.envDraft = '';
      this.envLabel = '';
      this.showEnv = true;
      void this.refreshServers();
    }
  }

  private serverHost(): string {
    try {
      const h = ApiClient.getServerHost();
      if (h) {
        return h;
      }
    } catch (e) {
      // ignore
    }
    return (this.server || '').replace(/\/api\/v1\/?$/i, '').replace(/\/+$/, '');
  }

  private schemeHint(): string {
    if (!this.server || !this.server.trim()) {
      return '尚未配置服务器，请连点右下角设置访问地址';
    }
    // Android：ApiClient.serverHost（含 scheme）
    const host = this.serverHost();
    if (host.toLowerCase().startsWith('https://')) {
      return `HTTPS 安全连接 · ${host}`;
    }
    return `HTTP 明文连接 · ${host}`;
  }

  private schemeHintColor(): string {
    if (!this.server || !this.server.trim()) {
      return this.tWarning;
    }
    if (this.serverHost().toLowerCase().startsWith('https://')) {
      return this.tMuted;
    }
    return this.tWarning;
  }

  private isActive(url: string): boolean {
    const a = (url || '').trim().replace(/\/+$/, '').toLowerCase();
    const b = (this.server || '').trim().replace(/\/+$/, '').toLowerCase();
    return a.length > 0 && a === b;
  }

  private openAddEnv(): void {
    this.envEditingUrl = '';
    this.envDraft = '';
    this.envLabel = '';
    this.showEnvForm = true;
  }

  private openEditEnv(e: ServerEntry): void {
    this.envEditingUrl = e.url;
    this.envDraft = e.url;
    this.envLabel = e.label || '';
    this.showEnvForm = true;
  }

  private async applyEnvSave(): Promise<void> {
    this.envError = '';
    const raw = this.envDraft.trim();
    if (!raw) {
      this.envError = '请填写服务器地址';
      return;
    }
    try {
      const normalized = ApiClient.normalizeBaseUrl(raw);
      const root = normalized.replace(/\/?api\/v1\/?$/i, '').replace(/\/+$/, '');
      if (this.envEditingUrl) {
        await SessionStore.updateServer(this.envEditingUrl, root, this.envLabel);
      } else {
        await SessionStore.saveServer(root, this.envLabel);
      }
      await ApiClient.clearPersistedSession();
      await ApiClient.init(root);
      this.server = root;
      this.showEnvForm = false;
      await this.refreshServers();
    } catch (e) {
      this.envError = ApiClient.formatError(e as Object);
    }
  }

  private async switchEnv(e: ServerEntry): Promise<void> {
    this.envError = '';
    try {
      await ApiClient.init(e.url);
      await ApiClient.clearPersistedSession();
      await SessionStore.switchServer(e.url);
      this.server = e.url;
      this.showEnv = false;
      await this.refreshServers();
    } catch (err) {
      this.envError = ApiClient.formatError(err as Object);
    }
  }

  private async doDeleteEnv(): Promise<void> {
    const target = this.envDeleteTarget;
    const wasActive = this.isActive(target);
    this.showEnvDelete = false;
    this.envDeleteTarget = '';
    this.envDeleteLabel = '';
    try {
      this.envError = '';
      const next = await SessionStore.removeServer(target);
      // Android：仅删除当前地址时清会话并切到备用
      if (wasActive) {
        await ApiClient.clearPersistedSession();
        if (next) {
          try {
            await ApiClient.init(next);
          } catch (e) {
            this.envError = ApiClient.formatError(e as Object);
          }
        }
      }
      await this.refreshServers();
    } catch (e) {
      this.envError = ApiClient.formatError(e as Object);
    }
  }

  private filterMfaDigits(v: string): string {
    let out = '';
    for (let i = 0; i < v.length && out.length < 6; i++) {
      const ch = v.charAt(i);
      if (ch >= '0' && ch <= '9') {
        out += ch;
      }
    }
    return out;
  }

  async doLogin(fromMfa: boolean): Promise<void> {
    this.error = '';
    if (!this.server.trim()) {
      this.error = '尚未配置服务器，请连点右下角设置访问地址';
      this.showEnv = true;
      return;
    }
    const user = fromMfa ? this.mfaUser : this.username.trim();
    const pass = fromMfa ? this.mfaPass : this.password;
    if (!user || !pass) {
      this.error = '请填写用户名和密码';
      return;
    }
    if (fromMfa && this.mfa.length < 6) {
      this.error = '请输入 6 位动态口令';
      return;
    }
    this.loading = true;
    try {
      await ApiClient.init(this.server);
      await SessionStore.saveServer(this.server.trim(), '');
      const resp = await Api.login({
        username: user,
        password: pass,
        code: fromMfa ? this.mfa : '',
        login_type: 'username'
      });
      if (resp.mfa_required) {
        this.mfaUser = user;
        this.mfaPass = pass;
        this.showMfaDialog = true;
        this.error = '';
        return;
      }
      if (resp.require_mfa_setup) {
        this.error = '请先在 Web 端完成 MFA 绑定';
        this.showMfaDialog = false;
        return;
      }
      if (resp.must_change_password) {
        this.error = '请先在 Web 端修改初始密码';
        this.showMfaDialog = false;
        return;
      }
      if (resp.ok === false || (resp.error && resp.error.length > 0 && resp.ok !== true)) {
        if (this.showMfaDialog) {
          this.error = resp.error || '动态口令错误';
        } else {
          this.error = resp.error || '登录失败';
        }
        return;
      }
      if (!ApiClient.cookieHeaderValue()) {
        this.error = '登录成功但未收到会话 Cookie，请检查服务器地址';
        return;
      }
      this.showMfaDialog = false;
      await AppState.onLoginSuccess(user);
      this.onSuccess();
    } catch (e) {
      this.error = ApiClient.formatError(e as Object);
    } finally {
      this.loading = false;
    }
  }

  build() {
    Stack() {
      Column() {
        Blank().layoutWeight(1)
        Column({ space: 14 }) {
          Column() {
            AppSymbol({ kind: 'lock', tint: AppTheme.brand, iconSize: 28 })
          }
          .width(56)
          .height(56)
          .justifyContent(FlexAlign.Center)
          .borderRadius(AppTheme.rMd)
          .backgroundColor('#1F4F7FFF')
          .margin({ bottom: 0 })

          Column() {
            Text('智能运维')
              .fontSize(AppTheme.fontDisplay)
              .fontWeight(FontWeight.Bold)
              .fontColor(this.tText)
            Text('AIOps 智能运维监控平台')
              .fontSize(AppTheme.fontCaption)
              .fontColor(this.tMuted)
              .margin({ top: 4 })
          }
          .alignItems(HorizontalAlign.Center)

          Column({ space: 14 }) {
            Text('账号登录')
              .fontSize(AppTheme.fontLead)
              .fontWeight(FontWeight.Bold)
              .fontColor(this.tText)
              .width('100%')

            Column({ space: 6 }) {
              Text('用户名')
                .fontSize(AppTheme.fontCaption)
                .fontColor(this.userFocused ? this.tAccent : this.tMuted)
                .width('100%')
              TextInput({ text: this.username, placeholder: '用户名' })
                .onChange((v: string) => {
                  this.username = v;
                })
                .onFocus(() => {
                  this.userFocused = true;
                })
                .onBlur(() => {
                  this.userFocused = false;
                })
                .height(48)
                .backgroundColor(this.tBg)
                .fontColor(this.tText)
                .placeholderColor(this.tMuted)
                .borderRadius(AppTheme.rSm)
                .border({
                  width: 1,
                  color: this.userFocused ? this.tAccent : this.tBorderSoft
                })
            }
            .width('100%')

            Column({ space: 6 }) {
              Text('密码')
                .fontSize(AppTheme.fontCaption)
                .fontColor(this.passFocused ? this.tAccent : this.tMuted)
                .width('100%')
              Row() {
                TextInput({ text: this.password, placeholder: '密码' })
                  .onChange((v: string) => {
                    this.password = v;
                  })
                  .onFocus(() => {
                    this.passFocused = true;
                  })
                  .onBlur(() => {
                    this.passFocused = false;
                  })
                  .type(this.showPwd ? InputType.Normal : InputType.Password)
                  .layoutWeight(1)
                  .height(48)
                  .backgroundColor(this.tBg)
                  .fontColor(this.tText)
                  .placeholderColor(this.tMuted)
                  .borderRadius({ topLeft: AppTheme.rSm, bottomLeft: AppTheme.rSm, topRight: 0, bottomRight: 0 })
                  .border({
                    width: {
                      left: 1,
                      top: 1,
                      bottom: 1,
                      right: 0
                    },
                    color: this.passFocused ? this.tAccent : this.tBorderSoft
                  })
                Column() {
                  AppSymbol({
                    kind: this.showPwd ? 'eye_off' : 'eye',
                    tint: this.tMuted,
                    iconSize: 20
                  })
                }
                .width(48)
                .height(48)
                .justifyContent(FlexAlign.Center)
                .backgroundColor(this.tBg)
                .borderRadius({ topRight: AppTheme.rSm, bottomRight: AppTheme.rSm })
                .border({
                  width: {
                    left: 0,
                    top: 1,
                    bottom: 1,
                    right: 1
                  },
                  color: this.passFocused ? this.tAccent : this.tBorderSoft
                })
                .onClick(() => {
                  this.showPwd = !this.showPwd;
                })
              }
              .width('100%')
              .alignItems(VerticalAlign.Center)
            }
            .width('100%')

            if (this.error && !this.showMfaDialog) {
              Text(this.error)
                .fontSize(AppTheme.fontCaption)
                .fontColor(this.tDanger)
                .width('100%')
            }

            PrimaryButton({
              label: this.loading ? '正在验证...' : '登 录',
              loading: this.loading,
              isEnabled: !!this.username.trim() && !!this.password && !!this.server.trim(),
              onTap: () => {
                void this.doLogin(false);
              }
            })
          }
          .width('100%')
          .padding(20)
          .backgroundColor(this.tCard)
          .borderRadius(AppTheme.rMd)

          Text(this.schemeHint())
            .fontSize(AppTheme.fontMicro)
            .fontColor(this.schemeHintColor())
            .width('100%')
            .textAlign(TextAlign.Center)
        }
        .width('100%')
        .constraintSize({ maxWidth: 480 })
        .padding({ left: 24, right: 24, top: 12, bottom: 12 })
        Blank().layoutWeight(1)

        // 右下角隐藏热区：透明、无视觉反馈（对齐 Android）
        Row()
          .width(64)
          .height(64)
          .alignSelf(ItemAlign.End)
          .onClick(() => this.onSecretTap())
      }
      .width('100%')
      .height('100%')
      .backgroundColor(this.tBg)

      if (this.showMfaDialog) {
        Column() {
          Column({ space: 12 }) {
            Text('双因素认证')
              .fontSize(AppTheme.fontLead)
              .fontWeight(FontWeight.Bold)
              .fontColor(this.tText)
            Text('请输入 6 位动态口令以完成登录')
              .fontSize(AppTheme.fontBody)
              .fontColor(this.tMuted)
            TextInput({ text: this.mfa, placeholder: '动态口令' })
              .type(InputType.Number)
              .maxLength(6)
              .onChange((v: string) => {
                this.mfa = this.filterMfaDigits(v);
              })
              .height(48)
              .backgroundColor(this.tBg)
              .fontColor(this.tText)
              .borderRadius(AppTheme.rSm)
              .border({ width: 1, color: this.tBorderSoft })
            if (this.error) {
              Text(this.error).fontSize(AppTheme.fontCaption).fontColor(this.tDanger)
            }
            Row({ space: 12 }) {
              Button('取消')
                .layoutWeight(1)
                .height(40)
                .backgroundColor(Color.Transparent)
                .fontColor(this.tAccent)
                .onClick(() => {
                  this.showMfaDialog = false;
                  this.mfa = '';
                  this.error = '';
                })
              Button() {
                if (this.loading) {
                  LoadingProgress().width(18).height(18).color(this.tOnAccent)
                } else {
                  Text('验证').fontSize(AppTheme.fontSmall).fontColor(this.tOnAccent)
                }
              }
              .layoutWeight(1)
              .height(40)
              .backgroundColor(this.tAccent)
              .borderRadius(AppTheme.rSm)
              .enabled(this.mfa.length >= 6 && !this.loading)
              .onClick(() => {
                void this.doLogin(true);
              })
            }
            .margin({ top: 4 })
          }
          .width('86%')
          .padding(18)
          .backgroundColor(this.tCard)
          .borderRadius(AppTheme.rMd)
        }
        .width('100%')
        .height('100%')
        .justifyContent(FlexAlign.Center)
        .backgroundColor(this.tScrim)
        .zIndex(60)
      }

      if (this.showEnv) {
        Column() {
          Column({ space: 12 }) {
            Row() {
              Text('环境与服务器')
                .fontSize(AppTheme.fontLead)
                .fontWeight(FontWeight.Bold)
                .fontColor(this.tText)
                .layoutWeight(1)
              Text('+ 添加')
                .fontSize(AppTheme.fontSmall)
                .fontColor(this.tAccent)
                .onClick(() => this.openAddEnv())
            }
            .width('100%')

            Text('连点右下角唤出。切换后需重新登录。')
              .fontSize(AppTheme.fontMicro)
              .fontColor(this.tMuted)
              .width('100%')

            if (this.envError) {
              Text(this.envError).fontSize(AppTheme.fontCaption).fontColor(this.tDanger).width('100%')
            }

            if (this.showEnvForm) {
              Column({ space: 10 }) {
                Text(this.envEditingUrl ? '编辑服务器' : '添加服务器')
                  .fontSize(AppTheme.fontBody)
                  .fontWeight(FontWeight.Bold)
                  .fontColor(this.tAccent)
                TextInput({ text: this.envLabel, placeholder: '名称（如：正式库 / 测试库）' })
                  .onChange((v: string) => {
                    this.envLabel = v;
                  })
                  .height(44)
                  .backgroundColor(this.tBg)
                  .fontColor(this.tText)
                  .borderRadius(AppTheme.rSm)
                  .border({ width: 1, color: this.tBorderSoft })
                TextInput({ text: this.envDraft, placeholder: 'http:// 或 https://' })
                  .onChange((v: string) => {
                    this.envDraft = v;
                  })
                  .height(44)
                  .backgroundColor(this.tBg)
                  .fontColor(this.tText)
                  .borderRadius(AppTheme.rSm)
                  .border({ width: 1, color: this.tBorderSoft })
                Row({ space: 8 }) {
                  Button('取消')
                    .layoutWeight(1)
                    .height(40)
                    .backgroundColor(this.tElevated)
                    .fontColor(this.tText)
                    .borderRadius(AppTheme.rSm)
                    .onClick(() => {
                      this.showEnvForm = false;
                    })
                  Button('保存')
                    .layoutWeight(1)
                    .height(40)
                    .backgroundColor(this.tAccent)
                    .fontColor(this.tOnAccent)
                    .borderRadius(AppTheme.rSm)
                    .enabled(!!this.envDraft.trim())
                    .onClick(() => {
                      void this.applyEnvSave();
                    })
                }
              }
              .width('100%')
              .padding(AppTheme.spaceMd)
              .backgroundColor(this.tBg)
              .borderRadius(AppTheme.rSm)
            }

            Scroll() {
              Column() {
                if (this.servers.length === 0 && !this.showEnvForm) {
                  Text('暂无服务器，请先添加访问地址')
                    .fontSize(AppTheme.fontBody)
                    .fontColor(this.tMuted)
                    .padding({ top: 28, bottom: 28 })
                    .width('100%')
                    .textAlign(TextAlign.Center)
                }
                ForEach(this.servers, (e: ServerEntry, idx: number) => {
                  Column() {
                    Row() {
                      Text(this.isActive(e.url) ? '●' : '○')
                        .fontSize(AppTheme.fontBody)
                        .fontColor(this.isActive(e.url) ? this.tSuccess : this.tMuted)
                        .width(22)
                      Column() {
                        Row() {
                          Text(e.label || '未命名')
                            .fontSize(AppTheme.fontBody)
                            .fontWeight(FontWeight.Bold)
                            .fontColor(this.isActive(e.url) ? this.tSuccess : this.tText)
                            .maxLines(1)
                            .layoutWeight(1)
                          if (this.isActive(e.url)) {
                            Text('当前')
                              .fontSize(AppTheme.fontTiny)
                              .fontWeight(FontWeight.Bold)
                              .fontColor(this.tSuccess)
                              .padding({ left: 6, right: 6, top: 2, bottom: 2 })
                              .backgroundColor(this.tSuccessSoft)
                              .borderRadius(AppTheme.rChip)
                              .margin({ left: 6 })
                          }
                        }
                        .width('100%')
                        Text(e.url)
                          .fontSize(AppTheme.fontCaption)
                          .fontColor(this.isActive(e.url) ? this.tAccent : this.tMuted)
                          .maxLines(1)
                          .textOverflow({ overflow: TextOverflow.Ellipsis })
                          .margin({ top: 2 })
                      }
                      .alignItems(HorizontalAlign.Start)
                      .layoutWeight(1)
                      if (!this.isActive(e.url)) {
                        Text('切换')
                          .fontSize(AppTheme.fontCaption)
                          .fontWeight(FontWeight.Bold)
                          .fontColor(this.tAccent)
                          .padding(AppTheme.spaceSm)
                          .onClick(() => {
                            void this.switchEnv(e);
                          })
                      }
                      Text('编辑')
                        .fontSize(AppTheme.fontCaption)
                        .fontColor(this.tMuted)
                        .padding(6)
                        .onClick(() => this.openEditEnv(e))
                      Text('删除')
                        .fontSize(AppTheme.fontCaption)
                        .fontColor(this.tMuted)
                        .padding(6)
                        .onClick(() => {
                          this.envDeleteTarget = e.url;
                          this.envDeleteLabel = e.label || e.url;
                          this.showEnvDelete = true;
                        })
                    }
                    .width('100%')
                    .padding({ top: 10, bottom: 10, left: 4, right: 4 })
                    .backgroundColor(this.isActive(e.url) ? '#144F7FFF' : Color.Transparent)
                    if (idx < this.servers.length - 1) {
                      Divider().color(this.tBorderSoft).strokeWidth(0.5)
                    }
                  }
                  .width('100%')
                }, (e: ServerEntry) => e.url)
              }
              .width('100%')
            }
            .constraintSize({ maxHeight: 280 })
            .scrollBar(BarState.Off)
            .width('100%')

            Button('关闭')
              .width('100%')
              .height(40)
              .backgroundColor(this.tElevated)
              .fontColor(this.tText)
              .borderRadius(AppTheme.rSm)
              .onClick(() => {
                this.showEnv = false;
              })
          }
          .width('90%')
          .constraintSize({ maxWidth: 420, maxHeight: '85%' })
          .padding(18)
          .backgroundColor(this.tCard)
          .borderRadius(AppTheme.rMd)
        }
        .width('100%')
        .height('100%')
        .justifyContent(FlexAlign.Center)
        .backgroundColor(this.tScrim)
        .zIndex(50)
      }

      ConfirmDialogHost({
        visible: this.showEnvDelete,
        title: '确认删除',
        message: `确定删除「${this.envDeleteLabel}」吗？`,
        confirmText: '删除',
        danger: true,
        onConfirm: () => {
          void this.doDeleteEnv();
        },
        onCancel: () => {
          this.showEnvDelete = false;
        }
      })
    }
    .width('100%')
    .height('100%')
  }
}
