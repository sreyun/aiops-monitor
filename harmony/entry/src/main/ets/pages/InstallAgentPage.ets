import { pasteboard } from '@kit.BasicServicesKit';
import { Api } from '../data/Api';
import { ApiClient } from '../data/ApiClient';
import { InstallInfo } from '../data/models/Models';
import { AppSymbol } from '../components/AppIcons';
import { ErrorBanner, PageHeader } from '../components/CommonUI';
import { NavUtil } from '../common/NavUtil';
import { AppTheme } from '../common/Routes';


/** Android InstallAgentScreen 1:1 */
interface CmdBlock {
  title: string;
  hint: string;
  command: string;
}

const PS_TLS12 =
  '[Net.ServicePointManager]::SecurityProtocol=[Net.ServicePointManager]::SecurityProtocol -bor 3072; ';

const OS_LABELS: string[] = ['Linux', 'Windows', 'macOS'];
const MODE_LABELS: string[] = ['标准', '网关中继', '多服务端'];

@Entry
@Component
struct InstallAgentPage {
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

  @State info: InstallInfo | null = null;
  @State loading: boolean = true;
  @State error: string = '';
  @State snack: string = '';
  @State tokenRevealed: boolean = false;
  @State category: string = '';
  @State logPaths: string = '';
  @State showLogPaths: boolean = false;
  @State osIdx: number = 0; // 0 Linux / 1 Windows / 2 macOS
  @State modeIdx: number = 0; // 0 NORMAL / 1 RELAY / 2 MULTI
  @State relayGatewayIp: string = '';
  @State multiServerList: string = '';
  @State cmdBlocks: CmdBlock[] = [];
  @State busyReset: boolean = false;
  @State catFocused: boolean = false;
  @State logFocused: boolean = false;
  @State relayFocused: boolean = false;
  @State multiFocused: boolean = false;

  aboutToAppear(): void {
    void this.reload();
  }

  private serverRoot(): string {
    const fromInfo = (this.info?.server_url || '').replace(/\/+$/, '');
    if (fromInfo) {
      return fromInfo;
    }
    return ApiClient.getServerHost().replace(/\/+$/, '');
  }

  private tokenValue(): string {
    return this.info?.token || '';
  }

  private maskToken(t: string, revealed: boolean): string {
    if (!t || !t.trim()) {
      return '（无 Token）';
    }
    if (revealed) {
      return t;
    }
    if (t.length <= 8) {
      return '••••••••';
    }
    return `${t.substring(0, 4)}••••••••${t.substring(t.length - 4)}`;
  }

  private enc(s: string): string {
    return encodeURIComponent(s);
  }

  private buildQuery(token: string, category: string, logPaths: string, serversJson: string): string {
    let q = `token=${this.enc(token)}`;
    if (category.trim()) {
      q += `&category=${this.enc(category.trim())}`;
    }
    if (logPaths.trim()) {
      q += `&log_paths=${this.enc(logPaths.trim())}`;
    }
    if (serversJson) {
      q += `&servers_json=${this.enc(serversJson)}`;
    }
    return q;
  }

  private parseMultiServers(text: string): string {
    const lines = text.split(/\r?\n/);
    const servers: string[] = [];
    for (let i = 0; i < lines.length; i++) {
      const line = lines[i].trim();
      if (!line) {
        continue;
      }
      const sp = line.search(/\s+/);
      let server = line;
      let tok = '';
      if (sp >= 0) {
        server = line.substring(0, sp);
        tok = line.substring(sp).trim();
      }
      const safeServer = server.replace(/"/g, '\\"');
      const safeTok = tok.replace(/"/g, '\\"');
      servers.push(`{"server":"${safeServer}","token":"${safeTok}"}`);
    }
    if (servers.length === 0) {
      return '';
    }
    return `[${servers.join(',')}]`;
  }

  private rebuildCommands(): void {
    const server = this.serverRoot();
    const token = this.tokenValue();
    const category = this.category;
    const logPaths = this.logPaths;
    const os = this.osIdx; // 0 L 1 W 2 M
    const mode = this.modeIdx; // 0 N 1 R 2 M
    const out: CmdBlock[] = [];

    if (mode === 1) {
      const q = this.buildQuery(token, category, logPaths, '');
      const gwIP = this.relayGatewayIp.trim() || '<网关IP>';
      const relay = `http://${gwIP}:8529`;
      let gwCmd = '';
      let inCmd = '';
      if (os === 1) {
        gwCmd = `${PS_TLS12}irm "${server}/install-relay.ps1?${q}" | iex`;
        inCmd = `${PS_TLS12}irm "${relay}/install.ps1?${q}" | iex`;
      } else if (os === 2) {
        gwCmd = `curl -fsSL "${server}/install-relay.sh?${q}" | sh`;
        inCmd = `curl -fsSL "${relay}/install.sh?${q}" | sh`;
      } else {
        gwCmd = `curl -fsSL "${server}/install-relay.sh?${q}" | sudo sh`;
        inCmd = `curl -fsSL "${relay}/install.sh?${q}" | sudo sh`;
      }
      out.push({ title: '网关机安装命令', hint: '在可访问外网的网关主机上执行', command: gwCmd });
      out.push({ title: '内网机安装命令', hint: '内网主机通过网关中继安装', command: inCmd });
    } else {
      const sj = mode === 2 ? this.parseMultiServers(this.multiServerList) : '';
      const q = this.buildQuery(token, category, logPaths, sj);
      let cmd = '';
      let title = '';
      let hint = '';
      if (os === 1) {
        cmd = `${PS_TLS12}irm "${server}/install.ps1?${q}" | iex`;
        title = 'PowerShell 安装命令';
        hint = '普通 PowerShell 即可（已内置 TLS 1.2，兼容 Windows Server 2012 R2）';
      } else if (os === 2) {
        cmd = `curl -fsSL "${server}/install.sh?${q}" | sh`;
        title = '终端一键安装';
        hint = '在 macOS 终端执行';
      } else {
        cmd = `curl -fsSL "${server}/install.sh?${q}" | sudo sh`;
        title = 'Linux 一键安装';
        hint = '需要 sudo 权限';
      }
      if (mode === 2) {
        title = '多服务端安装命令';
        hint = '安装后向多个服务端同时上报';
      }
      out.push({ title: title, hint: hint, command: cmd });
    }

    let uninstall = '';
    if (os === 1) {
      uninstall = `${PS_TLS12}irm "${server}/uninstall.ps1" | iex`;
    } else if (os === 2) {
      uninstall = `curl -fsSL "${server}/uninstall.sh" | sh`;
    } else {
      uninstall = `curl -fsSL "${server}/uninstall.sh" | sudo sh`;
    }
    out.push({ title: '卸载命令', hint: '从目标主机移除 Agent', command: uninstall });
    this.cmdBlocks = out;
  }

  private async reload(): Promise<void> {
    this.loading = true;
    this.error = '';
    try {
      this.info = await Api.installInfo();
      this.rebuildCommands();
    } catch (e) {
      this.error = ApiClient.formatError(e as Object);
      this.info = null;
    } finally {
      this.loading = false;
    }
  }

  private async resetToken(): Promise<void> {
    if (this.busyReset) {
      return;
    }
    this.busyReset = true;
    this.snack = '';
    try {
      const r = await Api.resetInstallToken();
      const prev = this.info;
      this.info = {
        server_url: prev?.server_url || this.serverRoot(),
        token: r.token || '',
        require_token: prev?.require_token === true
      };
      this.rebuildCommands();
      this.snack = 'Token 已重置';
    } catch (e) {
      this.snack = ApiClient.formatError(e as Object);
    } finally {
      this.busyReset = false;
    }
  }

  private copyText(text: string): void {
    try {
      const data = pasteboard.createData(pasteboard.MIMETYPE_TEXT_PLAIN, text);
      pasteboard.getSystemPasteboard().setData(data);
      this.snack = '已复制到剪贴板';
    } catch (e) {
      this.snack = '复制失败';
    }
  }

  build() {
    Column() {
      PageHeader({
        title: '安装 Agent',
        showBack: true,
        onBack: () => NavUtil.back()
      })

      if (this.snack) {
        Row() {
          Text(this.snack)
            .fontSize(AppTheme.fontSmall)
            .fontColor(this.tText)
            .layoutWeight(1)
          Text('关闭')
            .fontSize(AppTheme.fontSmall)
            .fontColor(this.tAccent)
            .onClick(() => {
              this.snack = '';
            })
        }
        .width('100%')
        .padding({ left: 16, right: 16, top: 10, bottom: 10 })
        .backgroundColor(this.tElevated)
      }

      ErrorBanner({ message: this.error })

      if (this.loading && !this.info) {
        Column() {
          LoadingProgress().width(36).height(36).color(this.tAccent)
        }
        .width('100%')
        .layoutWeight(1)
        .justifyContent(FlexAlign.Center)
      } else {
        Scroll() {
          Column({ space: 14 }) {
            // Token 卡
            Column({ space: 10 }) {
              Text('安装 Token')
                .fontSize(AppTheme.fontBody)
                .fontWeight(FontWeight.Bold)
                .fontColor(this.tAccent)
                .width('100%')
              Row() {
                Text(this.maskToken(this.tokenValue(), this.tokenRevealed))
                  .fontSize(AppTheme.fontSmall)
                  .fontFamily('monospace')
                  .fontColor(this.tText)
                  .layoutWeight(1)
                  .maxLines(2)
                  .textOverflow({ overflow: TextOverflow.Ellipsis })
                Column() {
                  AppSymbol({
                    kind: this.tokenRevealed ? 'eye_off' : 'eye',
                    tint: this.tMuted,
                    iconSize: 20
                  })
                }
                .width(40)
                .height(40)
                .justifyContent(FlexAlign.Center)
                .onClick(() => {
                  this.tokenRevealed = !this.tokenRevealed;
                })
                Button('重置')
                  .height(32)
                  .fontSize(AppTheme.fontCaption)
                  .backgroundColor(Color.Transparent)
                  .fontColor(this.tAccent)
                  .enabled(!this.busyReset)
                  .onClick(() => {
                    void this.resetToken();
                  })
              }
              .width('100%')
              .alignItems(VerticalAlign.Center)

              if (this.info?.require_token === true) {
                Text('当前服务端要求安装时携带 Token')
                  .fontSize(AppTheme.fontMicro)
                  .fontColor(this.tWarning)
                  .width('100%')
              }
            }
            .width('100%')
            .padding(AppTheme.pagePadH)
            .backgroundColor(this.tCard)
            .borderRadius(AppTheme.rMd)

            this.fieldBlock('分类（可选）', '如：生产 / 机房A', this.category, this.catFocused, (v: string) => {
              this.category = v;
              this.rebuildCommands();
            }, () => {
              this.catFocused = true;
            }, () => {
              this.catFocused = false;
            }, 1)

            Text(this.showLogPaths ? '收起日志路径' : '展开日志路径（可选）')
              .fontSize(AppTheme.fontSmall)
              .fontColor(this.tAccent)
              .padding({ top: 2, bottom: 2 })
              .onClick(() => {
                this.showLogPaths = !this.showLogPaths;
              })

            if (this.showLogPaths) {
              this.fieldBlock('日志路径', '每行一个路径，或逗号分隔', this.logPaths, this.logFocused, (v: string) => {
                this.logPaths = v;
                this.rebuildCommands();
              }, () => {
                this.logFocused = true;
              }, () => {
                this.logFocused = false;
              }, 3)
            }

            Text('操作系统')
              .fontSize(AppTheme.fontBody)
              .fontWeight(FontWeight.Bold)
              .fontColor(this.tAccent)
              .width('100%')
            this.chipRow(OS_LABELS, this.osIdx, (i: number) => {
              this.osIdx = i;
              this.rebuildCommands();
            })

            Text('安装模式')
              .fontSize(AppTheme.fontBody)
              .fontWeight(FontWeight.Bold)
              .fontColor(this.tAccent)
              .width('100%')
            this.chipRow(MODE_LABELS, this.modeIdx, (i: number) => {
              this.modeIdx = i;
              this.rebuildCommands();
            })

            if (this.modeIdx === 1) {
              this.fieldBlock('网关内网 IP', '如 192.168.1.10', this.relayGatewayIp, this.relayFocused, (v: string) => {
                this.relayGatewayIp = v;
                this.rebuildCommands();
              }, () => {
                this.relayFocused = true;
              }, () => {
                this.relayFocused = false;
              }, 1)
            }
            if (this.modeIdx === 2) {
              this.fieldBlock('多服务端列表', '每行：服务器地址 [token]', this.multiServerList, this.multiFocused, (v: string) => {
                this.multiServerList = v;
                this.rebuildCommands();
              }, () => {
                this.multiFocused = true;
              }, () => {
                this.multiFocused = false;
              }, 3)
            }

            ForEach(this.cmdBlocks, (b: CmdBlock, idx: number) => {
              Column({ space: 8 }) {
                Row() {
                  Text(b.title)
                    .fontSize(AppTheme.fontBody)
                    .fontWeight(FontWeight.Bold)
                    .fontColor(this.tText)
                    .layoutWeight(1)
                  Column() {
                    AppSymbol({ kind: 'attach', tint: this.tAccent, iconSize: 18 })
                  }
                  .width(36)
                  .height(36)
                  .justifyContent(FlexAlign.Center)
                  .onClick(() => this.copyText(b.command))
                }
                .width('100%')

                if (b.hint) {
                  Text(b.hint)
                    .fontSize(AppTheme.fontMicro)
                    .fontColor(this.tMuted)
                    .width('100%')
                }

                Text(b.command)
                  .fontSize(AppTheme.fontCaption)
                  .fontFamily('monospace')
                  .fontColor(this.tText)
                  .width('100%')
                  .padding(10)
                  .backgroundColor(this.tBg)
                  .borderRadius(AppTheme.rXs)

                Button() {
                  Row({ space: 6 }) {
                    AppSymbol({ kind: 'attach', tint: this.tAccent, iconSize: 16 })
                    Text('复制命令')
                      .fontSize(AppTheme.fontSmall)
                      .fontColor(this.tAccent)
                  }
                }
                .width('100%')
                .height(40)
                .backgroundColor(Color.Transparent)
                .border({ width: 1, color: this.tBorderSoft })
                .borderRadius(AppTheme.rSm)
                .onClick(() => this.copyText(b.command))
              }
              .width('100%')
              .padding(AppTheme.pagePadH)
              .backgroundColor(this.tCard)
              .borderRadius(AppTheme.rMd)
            }, (b: CmdBlock, idx: number) => `${idx}-${b.title}`)

            Blank().height(20)
          }
          .width('100%')
          .padding(AppTheme.pagePadH)
        }
        .layoutWeight(1)
        .scrollBar(BarState.Off)
        .align(Alignment.Top)
      }
    }
    .width('100%')
    .height('100%')
    .backgroundColor(this.themeRev >= 0 ? this.tBg : this.tBg)
  }

  @Builder
  chipRow(labels: string[], selected: number, onPick: (i: number) => void) {
    Row({ space: 8 }) {
      ForEach(labels, (lab: string, i: number) => {
        Text(lab)
          .fontSize(AppTheme.fontSmall)
          .fontColor(selected === i ? this.tOnAccent : this.tText)
          .padding({ left: 14, right: 14, top: 8, bottom: 8 })
          .backgroundColor(selected === i ? this.tAccent : this.tElevated)
          .borderRadius(AppTheme.rLg)
          .border({
            width: 1,
            color: selected === i ? this.tAccent : this.tBorderSoft
          })
          .onClick(() => onPick(i))
      }, (lab: string, i: number) => `${i}-${lab}`)
    }
    .width('100%')
  }

  @Builder
  fieldBlock(
    label: string,
    placeholder: string,
    value: string,
    focused: boolean,
    onChange: (v: string) => void,
    onFocus: () => void,
    onBlur: () => void,
    lines: number
  ) {
    Column({ space: 6 }) {
      Text(label)
        .fontSize(AppTheme.fontCaption)
        .fontColor(focused ? this.tAccent : this.tMuted)
        .width('100%')
      TextInput({ text: value, placeholder: placeholder })
        .onChange((v: string) => onChange(v))
        .onFocus(() => onFocus())
        .onBlur(() => onBlur())
        .height(lines > 1 ? 44 + (lines - 1) * 22 : 44)
        .backgroundColor(this.tBg)
        .fontColor(this.tText)
        .placeholderColor(this.tMuted)
        .borderRadius(AppTheme.rSm)
        .border({ width: 1, color: focused ? this.tAccent : this.tBorderSoft })
    }
    .width('100%')
  }
}
