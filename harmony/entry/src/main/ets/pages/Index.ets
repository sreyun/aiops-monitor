import { LoginPanel } from '../components/LoginUI';
import { AppState } from '../common/AppState';
import { AppTheme } from '../common/Routes';
import { ApiClient } from '../data/ApiClient';
import { NavUtil } from '../common/NavUtil';
import { DashboardTab } from './DashboardPage';
import { AlertsTab } from './AlertsPage';
import { InfraHubTab } from './InfraHubPage';
import { OperationsTab } from './OperationsPage';
import { AiCopilotTab } from './AiCopilotPage';
import { LoadingBlock } from '../components/CommonUI';
import { TabBarItem } from '../components/AppIcons';

@Entry
@Component
struct Index {
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


  @State ready: boolean = false;
  @State loggedIn: boolean = false;
  @State needsRetry: boolean = false;
  @State statusText: string = '';
  @State selectedTab: number = 0;
  @State infraTab: number = 0;
  @State infraRes: number = 0;
  @State alertLevelFilter: string = '';
  @State opsTabHint: number = 0;
  @State loginToken: number = 0;
  private authListener: () => void = () => {
    this.syncAuthFromState();
  };

  aboutToAppear(): void {
    void this.onAppearAsync();
  }

  aboutToDisappear(): void {
    AppState.removeAuthListener(this.authListener);
  }

  private syncAuthFromState(): void {
    this.loginToken = AppState.forceLoginToken;
    this.loggedIn = AppState.loggedIn;
    this.needsRetry = AppState.needsRetry;
    this.statusText = AppState.statusText;
    if (!this.loggedIn) {
      this.selectedTab = 0;
    } else {
      this.consumeDeepLink();
    }
  }

  private async onAppearAsync(): Promise<void> {
    AppState.addAuthListener(this.authListener);
    const ok = await AppState.bootstrap();
    this.loggedIn = ok;
    this.needsRetry = AppState.needsRetry;
    this.statusText = AppState.statusText;
    this.ready = true;
    this.loginToken = AppState.forceLoginToken;
    if (ok) {
      this.consumeDeepLink();
    }
  }

  private async retrySession(): Promise<void> {
    this.ready = false;
    const ok = await AppState.retryBootstrap();
    this.loggedIn = ok;
    this.needsRetry = AppState.needsRetry;
    this.statusText = AppState.statusText;
    this.ready = true;
    if (ok) {
      this.consumeDeepLink();
    }
  }

  private consumeDeepLink(): void {
    const route = ApiClient.pendingDeepLink;
    if (!route || !this.loggedIn) {
      return;
    }
    ApiClient.pendingDeepLink = '';
    const r = route.toLowerCase();

    // alerts/{level}
    const alertMatch = route.match(/alerts?\/([a-zA-Z_]+)/);
    if (alertMatch && alertMatch[1]) {
      this.alertLevelFilter = alertMatch[1];
      this.selectedTab = 2;
      return;
    }

    // infra/tab/{tab}?res={res} — 对齐 Android Routes.infraTab
    const infraMatch = route.match(/(?:infra|monitor)\/tab\/(\d+)/);
    if (infraMatch) {
      this.infraTab = Number(infraMatch[1]) || 0;
      const resMatch = route.match(/[?&]res=(\d+)/);
      this.infraRes = resMatch ? (Number(resMatch[1]) || 0) : 0;
      this.selectedTab = 1;
      return;
    }

    // operations/tab/{tab}
    const opsMatch = route.match(/operations?\/tab\/(\d+)/);
    if (opsMatch) {
      this.opsTabHint = Number(opsMatch[1]) || 0;
      this.selectedTab = 3;
      return;
    }

    // ai_copilot/diagnose/{hostId}
    const aiMatch = route.match(/ai_copilot\/diagnose\/([^/?#]+)/i) ||
      route.match(/ai\/diagnose\/([^/?#]+)/i);
    if (aiMatch && aiMatch[1]) {
      this.selectedTab = 4;
      NavUtil.push('pages/AiCopilotPage', { hostId: aiMatch[1] });
      return;
    }

    // host_detail/{id}
    const hostMatch = route.match(/host_detail\/([^/?#]+)/i) ||
      route.match(/hosts?\/([^/?#]+)/i);
    if (hostMatch && hostMatch[1] && r.indexOf('duplicate') < 0) {
      NavUtil.push('pages/HostDetailPage', { hostId: hostMatch[1] });
      return;
    }

    if (r.indexOf('alert') >= 0) {
      this.selectedTab = 2;
    } else if (r.indexOf('operation') >= 0) {
      this.selectedTab = 3;
    } else if (r.indexOf('ai') >= 0) {
      this.selectedTab = 4;
    } else if (r.indexOf('monitor') >= 0 || r.indexOf('infra') >= 0) {
      this.selectedTab = 1;
    } else if (r.indexOf('message') >= 0) {
      NavUtil.push('pages/MessagesPage');
    } else if (r.indexOf('setting') >= 0) {
      NavUtil.push('pages/SettingsPage');
    } else if (r.indexOf('dashboard') >= 0 && r.indexOf('view') >= 0) {
      NavUtil.push('pages/DashboardsPage');
    }
  }

  build() {
    Column() {
      if (!this.ready) {
        LoadingBlock({ text: '正在校验会话…' })
      } else if (this.needsRetry && !this.loggedIn) {
        Column() {
          Text('无法连接服务器')
            .fontSize(AppTheme.fontTitle)
            .fontWeight(FontWeight.Medium)
            .fontColor(this.tText)
          Text(this.statusText || '请检查网络后重试，会话凭证已保留')
            .fontSize(AppTheme.fontSmall)
            .fontColor(this.tMuted)
            .margin({ top: 10, bottom: 20 })
            .textAlign(TextAlign.Center)
            .padding({ left: 24, right: 24 })
          Button('重试连接')
            .width('80%')
            .backgroundColor(this.tAccent)
            .onClick(() => {
              void this.retrySession();
            })
          Button('改用其他账号登录')
            .width('80%')
            .margin({ top: 12 })
            .backgroundColor(this.tElevated)
            .fontColor(this.tText)
            .onClick(() => {
              void AppState.logout().then(() => {
                this.needsRetry = false;
                this.loggedIn = false;
              });
            })
        }
        .width('100%')
        .height('100%')
        .justifyContent(FlexAlign.Center)
        .backgroundColor(this.tBg)
      } else if (!this.loggedIn) {
        LoginEmbed({
          onSuccess: () => {
            this.loggedIn = true;
            this.needsRetry = false;
            this.loginToken = AppState.forceLoginToken;
            this.consumeDeepLink();
          }
        })
      } else {
        Tabs({ barPosition: BarPosition.End, index: this.selectedTab }) {
          TabContent() {
            DashboardTab({
              onOpenSettings: () => {
                NavUtil.push('pages/SettingsPage');
              },
              onOpenHost: (id: string) => {
                NavUtil.push('pages/HostDetailPage', { hostId: id });
              },
              onOpenAlerts: (level: string) => {
                this.alertLevelFilter = level;
                this.selectedTab = 2;
              },
              onSwitchTab: (bottom: number, sub: number, res: number) => {
                if (bottom === 1) {
                  this.infraTab = sub;
                  this.infraRes = res;
                }
                if (bottom === 3) {
                  this.opsTabHint = sub;
                }
                this.selectedTab = bottom;
              }
            })
          }.tabBar(this.tabBuilder('总览', 'house', 0))

          TabContent() {
            InfraHubTab({
              initialTab: this.infraTab,
              initialRes: this.infraRes,
              onOpenHost: (id: string) => {
                NavUtil.push('pages/HostDetailPage', { hostId: id });
              }
            })
          }.tabBar(this.tabBuilder('监控', 'speed', 1))

          TabContent() {
            AlertsTab({
              initialLevel: this.alertLevelFilter,
              onDiagnose: (hostId: string, context?: string, display?: string) => {
                this.selectedTab = 4;
                NavUtil.push('pages/AiCopilotPage', {
                  hostId: hostId,
                  prompt: context || '',
                  display: display || ''
                });
              }
            })
          }.tabBar(this.tabBuilder('告警', 'warning', 2))

          TabContent() {
            OperationsTab({
              initialTab: this.opsTabHint,
              onExecution: (id: number) => {
                NavUtil.push('pages/ExecutionDetailPage', { executionId: id });
              }
            })
          }.tabBar(this.tabBuilder('运维', 'build', 3))

          TabContent() {
            AiCopilotTab()
          }.tabBar(this.tabBuilder('AI', 'spark', 4))
        }
        .onChange((i: number) => {
          this.selectedTab = i;
        })
        .width('100%')
        .height('100%')
        .backgroundColor(this.tBg)
        .barHeight(AppTheme.navBarH)
        .barBackgroundColor(this.tNavBar)
        .animationDuration(220)
      }
    }
    .width('100%')
    .height('100%')
    .backgroundColor(this.tBg)
  }

  @Builder
  tabBuilder(title: string, kind: string, index: number) {
    TabBarItem({
      title: title,
      kind: kind,
      selected: this.selectedTab === index
    })
  }
}

@Component
struct LoginEmbed {
  onSuccess: () => void = () => {};

  build() {
    LoginPanel({
      onSuccess: () => this.onSuccess()
    })
  }
}

