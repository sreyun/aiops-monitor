import { Api } from '../data/Api';
import { ApiClient } from '../data/ApiClient';
import { PageHeader, ErrorBanner, PrimaryButton, SecondaryButton } from '../components/CommonUI';
import { TerminalClient } from '../data/terminal/TerminalClient';
import { VtBuffer } from '../data/terminal/VtBuffer';
import { NavUtil } from '../common/NavUtil';
import { AppTheme } from '../common/Routes';


interface TermKey {
  label: string;
  payload: string;
}

@Entry
@Component
struct TerminalPage {
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

  @State hostId: string = '';
  @State output: string = '';
  @State input: string = '';
  @State error: string = '';
  @State status: string = '等待验证';
  /** Android TerminalScreen: password gate before connect */
  @State showPwGate: boolean = true;
  @State pw: string = '';
  @State verifying: boolean = false;
  @State connected: boolean = false;
  @State reconnecting: boolean = false;
  private client: TerminalClient = new TerminalClient();
  private buffer: VtBuffer = new VtBuffer();
  private termCols: number = 100;
  private termRows: number = 35;
  private termScroller: Scroller = new Scroller();
  private keys: TermKey[] = [
    { label: 'Ctrl+C', payload: '\u0003' },
    { label: 'Tab', payload: '\t' },
    { label: 'Ctrl+D', payload: '\u0004' },
    { label: 'Esc', payload: '\u001b' },
    { label: '↑', payload: '\u001b[A' },
    { label: '↓', payload: '\u001b[B' },
    { label: 'Ctrl+L', payload: '\u000c' },
    { label: 'Ctrl+Z', payload: '\u001a' }
  ];

  aboutToAppear(): void {
    this.hostId = NavUtil.paramString('hostId', '');
    if (!this.hostId) {
      this.status = '缺少主机 ID';
      this.error = '无法打开终端';
      this.showPwGate = false;
      return;
    }
    this.client.onData = (t: string) => {
      const replies = this.buffer.write(t);
      this.refreshOutput(true);
      for (let i = 0; i < replies.length; i++) {
        this.client.sendInput(replies[i]);
      }
    };
    this.client.onSessionStart = (message: string, clearHistory: boolean) => {
      const banner = message.length > 0 ? message : null;
      this.buffer.startNewSession(banner, clearHistory);
      this.refreshOutput(true);
    };
    this.client.onStatus = (raw: string) => {
      this.applyClientStatus(raw);
    };
    this.client.onClose = (r: string) => {
      this.connected = false;
      this.reconnecting = false;
      if (r === 'session_ended') {
        this.status = 'Shell 会话已正常结束';
      } else if (r === '未授权' || r === '会话无效') {
        this.status = '登录会话已失效';
      } else {
        this.status = `已断开: ${r}`;
      }
    };
    this.client.onNeedPassword = () => {
      this.connected = false;
      this.reconnecting = false;
      this.client.close();
      this.error = '终端授权已失效，请重新验证';
      this.showPwGate = true;
      this.status = '需要终端密码';
    };
    // 不自动连接：对齐 Android，先验证二次密码
  }

  aboutToDisappear(): void {
    this.client.close();
  }

  onPageShow(): void {
    if (!this.showPwGate && this.hostId) {
      this.client.ensureConnected();
    }
  }

  private applyClientStatus(raw: string): void {
    this.connected = raw === 'connected';
    this.reconnecting = raw.startsWith('reconnecting:');
    if (raw === 'connected') {
      this.status = '已连接';
      this.error = '';
      return;
    }
    if (raw === 'connecting') {
      this.status = '连接中…';
      return;
    }
    if (raw.startsWith('reconnecting:')) {
      this.status = this.formatReconnectStatus(raw);
      return;
    }
    if (raw === 'closed:session_ended') {
      this.status = 'Shell 会话已正常结束';
      return;
    }
    if (raw === 'unauthorized') {
      this.status = '登录会话已失效';
      return;
    }
    if (raw === 'need_terminal_password') {
      this.status = '需要终端密码';
      return;
    }
    if (raw.startsWith('error:')) {
      this.status = `连接失败：${raw.substring(6)}`;
      return;
    }
    if (raw.startsWith('closed')) {
      this.status = '终端连接已关闭';
    }
  }

  private formatReconnectStatus(raw: string): string {
    const parts = raw.split(':');
    const attemptStr = parts.length > 1 ? parts[1] : '1';
    const secondsStr = parts.length > 2 ? parts[2] : '0';
    const attempt = Number(attemptStr);
    const seconds = Number(secondsStr);
    const n = (!isNaN(attempt) && attempt > 0) ? attempt : 1;
    if (!isNaN(seconds) && seconds > 0) {
      return `网络中断 · 第 ${n} 次自动重连（${seconds}s）`;
    }
    return '正在重新建立终端会话…';
  }

  private refreshOutput(followBottom: boolean): void {
    this.output = this.buffer.displayText();
    if (!followBottom) {
      return;
    }
    setTimeout(() => {
      try {
        this.termScroller.scrollEdge(Edge.Bottom);
      } catch (_e) {
        // ignore
      }
    }, 0);
  }

  private onTermAreaChange(_old: Area, area: Area): void {
    const w = Number(area.width);
    const h = Number(area.height);
    if (isNaN(w) || isNaN(h) || w < 40 || h < 40) {
      return;
    }
    const cols = Math.max(40, Math.min(240, Math.floor(w / 7.2)));
    const rows = Math.max(8, Math.min(120, Math.floor(h / 16)));
    if (cols === this.termCols && rows === this.termRows) {
      return;
    }
    this.termCols = cols;
    this.termRows = rows;
    this.buffer.resize(cols, rows);
    this.refreshOutput(false);
    this.client.resize(cols, rows);
  }

  private startConnect(): void {
    this.buffer = new VtBuffer(this.termCols, this.termRows);
    this.output = '';
    this.status = '连接中…';
    this.connected = false;
    this.reconnecting = false;
    this.showPwGate = false;
    this.client.connect(this.hostId, this.termCols, this.termRows);
  }

  private async verifyAndConnect(): Promise<void> {
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
      this.startConnect();
    } catch (e) {
      this.error = ApiClient.formatError(e as Object);
    } finally {
      this.verifying = false;
    }
  }

  private sendKey(payload: string): void {
    if (!ApiClient.isSessionAlive()) {
      this.error = '会话已失效';
      return;
    }
    if (!this.client.sendInput(payload)) {
      this.error = this.reconnecting ? '正在重连，请稍候' : '终端未连接';
    }
  }

  sendLine(): void {
    if (!ApiClient.isSessionAlive()) {
      this.error = '会话已失效';
      return;
    }
    const line = this.input;
    if (!this.client.sendInput(line + '\n')) {
      this.error = this.reconnecting ? '正在重连，请稍候' : '终端未连接';
      return;
    }
    this.input = '';
  }

  build() {
    Column() {
      PageHeader({
        title: `终端 · ${this.hostId || '—'}`,
        showBack: true,
        onBack: () => {
          this.client.close();
          NavUtil.back();
        },
        trailing: (!this.showPwGate && this.hostId.length > 0) ? '重连' : '',
        onTrailing: () => {
          if (this.showPwGate || !this.hostId) {
            return;
          }
          this.client.reconnectNow();
        }
      })
      Text(this.status)
        .fontSize(AppTheme.fontCaption)
        .fontColor(this.reconnecting ? this.tWarning : this.tMuted)
        .padding({ left: 16 })
      ErrorBanner({ message: this.error })

      if (this.showPwGate) {
        Column() {
          Text('终端验证')
            .fontSize(AppTheme.fontLead)
            .fontWeight(FontWeight.Bold)
            .fontColor(this.tText)
          Text('该操作具有最高权限，请输入终端二次密码后再连接。')
            .fontSize(AppTheme.fontSmall)
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
            .onSubmit(() => {
              void this.verifyAndConnect();
            })
          PrimaryButton({
            label: this.verifying ? '验证中…' : '验证并连接',
            isEnabled: !this.verifying,
            onTap: () => {
              void this.verifyAndConnect();
            }
          })
            .margin({ top: 14 })
          SecondaryButton({
            label: '设置 / 修改终端密码',
            onTap: () => {
              NavUtil.push('pages/TerminalPasswordPage');
            }
          })
            .margin({ top: 10 })
          SecondaryButton({
            label: '查看会话回放',
            onTap: () => {
              NavUtil.push('pages/TerminalSessionsPage');
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
        Scroll(this.termScroller) {
          Text(this.output || ' ')
            .fontSize(AppTheme.fontCaption)
            .fontFamily('monospace')
            .fontColor('#D1E7DD')
            .width('100%')
        }
        .layoutWeight(1)
        .margin(12)
        .padding(AppTheme.spaceSm)
        .backgroundColor('#0A0F0A')
        .borderRadius(AppTheme.rXs)
        .scrollBar(BarState.Auto)
        .onAreaChange((oldArea: Area, newArea: Area) => {
          this.onTermAreaChange(oldArea, newArea);
        })

        Scroll() {
          Row({ space: 6 }) {
            ForEach(this.keys, (k: TermKey) => {
              Text(k.label)
                .fontSize(AppTheme.fontMicro)
                .fontColor(this.tText)
                .padding({ left: 10, right: 10, top: 6, bottom: 6 })
                .backgroundColor(this.tElevated)
                .borderRadius(AppTheme.rXs)
                .onClick(() => this.sendKey(k.payload))
            }, (k: TermKey) => k.label)
          }
        }
        .scrollable(ScrollDirection.Horizontal)
        .scrollBar(BarState.Off)
        .height(40)
        .padding({ left: 12, right: 12 })

        Row({ space: 8 }) {
          TextInput({ text: this.input, placeholder: '输入命令，回车发送' })
            .onChange((v: string) => {
              this.input = v;
            })
            .layoutWeight(1)
            .height(40)
            .backgroundColor(this.tElevated)
            .fontColor(this.tText)
            .onSubmit(() => this.sendLine())
          Button('发送')
            .backgroundColor(this.tAccent)
            .enabled(ApiClient.isSessionAlive() && this.connected)
            .onClick(() => this.sendLine())
        }
        .padding(AppTheme.spaceMd)
      }
    }
    .width('100%')
    .height('100%')
    .backgroundColor(this.tBg)
  }
}
