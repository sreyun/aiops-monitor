import { common, Want } from '@kit.AbilityKit';
import { ApiClient } from '../data/ApiClient';
import { AppTheme } from '../common/Routes';
import { PageHeader, ErrorBanner } from '../components/CommonUI';
import { AppSymbol } from '../components/AppIcons';
import { NavUtil } from '../common/NavUtil';

/**
 * Android SettingsScreen 1:1 — 配置中心。
 * 主题 / 消息 / 重复主机 / 终端回放 / 切换账号 / 退出 在总览齿轮菜单，不在此页。
 * 服务器地址仅登录页右下角连点三次管理。
 */
@Entry
@Component
struct SettingsPage {
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

  @State err: string = '';

  private loggedIn(): boolean {
    return ApiClient.cookieHeaderValue().length > 0 && ApiClient.isSessionAlive();
  }

  private openWebsite(): void {
    try {
      const host = ApiClient.getServerHost();
      const uri = host && host.length > 8 ? host : 'https://aiops.sreyun.com';
      const ctx = getContext(this) as common.UIAbilityContext;
      const want: Want = {
        action: 'ohos.want.action.viewData',
        entities: ['entity.system.browsable'],
        uri: uri
      };
      void ctx.startAbility(want);
    } catch (e) {
      this.err = '无法打开浏览器';
    }
  }

  build() {
    Column() {
      PageHeader({
        title: '配置中心',
        showBack: true,
        onBack: () => NavUtil.back()
      })
      ErrorBanner({ message: this.err })

      Scroll() {
        Column({ space: 16 }) {
          this.actionCard(
            '安装 Agent',
            '查看各平台一键安装 / 卸载命令，支持复制',
            'hosts',
            '#4F7FFF',
            'pages/InstallAgentPage',
            this.loggedIn()
          )
          this.actionCard(
            '成本管理',
            this.loggedIn() ? '调用次数 · 延迟 · Token · 费用趋势' : '登录后可查看 AI 调用与费用',
            'spark',
            '#F97316',
            'pages/CostManagementPage',
            this.loggedIn()
          )
          this.actionCard(
            '用户管理',
            this.loggedIn() ? '创建 / 改角色 / 删除用户' : '登录后可用（需管理员）',
            'person',
            '#6366F1',
            'pages/UsersAdminPage',
            this.loggedIn()
          )
          this.actionCard(
            '活动审计',
            this.loggedIn() ? '操作 / 系统 / 终端活动日志' : '登录后可查看',
            'logs',
            '#0EA5E9',
            'pages/ActivityAuditPage',
            this.loggedIn()
          )
          this.actionCard(
            '知识库',
            this.loggedIn() ? '规则 / 模板管理（Hermes 知识）' : '登录后可查看',
            'spark',
            '#8B5CF6',
            'pages/KnowledgePage',
            this.loggedIn()
          )

          Column({ space: AppTheme.spaceSm }) {
            Text('AI / 告警配置')
              .fontSize(AppTheme.fontBody)
              .fontWeight(FontWeight.Bold)
              .fontColor(this.tText)
              .width('100%')
            Text('AI Provider 与告警通知通道由网页端管理员配置。本客户端可查看 AI 用量；诊断失败时请先确认服务端 AI 已启用。')
              .fontSize(AppTheme.fontCaption)
              .fontColor(this.tMuted)
              .lineHeight(18)
              .width('100%')
            Row({ space: 10 }) {
              Text('查看 AI 用量')
                .fontSize(AppTheme.fontCaption)
                .fontColor(this.tAccent)
                .padding({ left: AppTheme.spaceMd, right: AppTheme.spaceMd, top: AppTheme.spaceSm, bottom: AppTheme.spaceSm })
                .backgroundColor(this.tAccentSoft)
                .borderRadius(AppTheme.rSm)
                .opacity(this.loggedIn() ? 1 : 0.4)
                .onClick(() => {
                  if (this.loggedIn()) {
                    NavUtil.push('pages/CostManagementPage');
                  }
                })
              Text('打开管理站点')
                .fontSize(AppTheme.fontCaption)
                .fontColor(this.tMuted)
                .padding({ left: AppTheme.spaceMd, right: AppTheme.spaceMd, top: AppTheme.spaceSm, bottom: AppTheme.spaceSm })
                .backgroundColor(this.tElevated)
                .borderRadius(AppTheme.rSm)
                .border({ width: AppTheme.hairline, color: this.tBorderSoft })
                .onClick(() => this.openWebsite())
            }
            .width('100%')
          }
          .width('100%')
          .padding(AppTheme.cardPad)
          .backgroundColor(this.tCard)
          .borderRadius(AppTheme.rMd)
          .border({ width: AppTheme.hairline, color: this.tBorderSoft })
          .shadow({ radius: 4, color: this.tShadow, offsetY: 1 })
          .alignItems(HorizontalAlign.Start)

          Text('系统级设置（备份还原等）仅管理员可在网页端修改；成本管理数据为只读观测。用户管理需 admin。')
            .fontSize(AppTheme.fontCaption)
            .fontColor(this.tMuted)
            .lineHeight(18)
            .width('100%')

          this.actionCard(
            '终端二次密码',
            this.loggedIn() ? '设置或修改终端二次密码' : '登录后可设置',
            'lock',
            AppTheme.brand,
            'pages/TerminalPasswordPage',
            this.loggedIn()
          )

          Column({ space: 10 }) {
            Row({ space: AppTheme.spaceSm }) {
              AppSymbol({ kind: 'settings', tint: AppTheme.brand, iconSize: 20 })
              Text('关于我们')
                .fontSize(AppTheme.fontBody)
                .fontWeight(FontWeight.Bold)
                .fontColor(AppTheme.brand)
            }
            .width('100%')

            Text('智能运维（AIOps）是面向企业基础设施的一体化智能运维平台，覆盖主机监控、网络流量、告警治理、自动化巡检与 AI 辅助诊断，帮助运维团队更快发现、定位并处理故障。')
              .fontSize(AppTheme.fontBody)
              .fontColor(this.tMuted)
              .lineHeight(20)
              .width('100%')

            Row({ space: 6 }) {
              AppSymbol({ kind: 'dashboards', tint: this.tAccent, iconSize: 16 })
              Text('访问官网 https://aiops.sreyun.com')
                .fontSize(AppTheme.fontSmall)
                .fontColor(this.tAccent)
            }
            .width('100%')
            .padding({ top: AppTheme.spaceXs, bottom: AppTheme.spaceXs })
            .onClick(() => this.openWebsite())
          }
          .width('100%')
          .padding(AppTheme.pagePadH)
          .backgroundColor(this.tCard)
          .borderRadius(AppTheme.rMd)
          .border({ width: AppTheme.hairline, color: this.tBorderSoft })
          .shadow({ radius: 4, color: this.tShadow, offsetY: 1 })

          Text('服务器访问地址请在登录页右下角连点三次进行管理与切换。')
            .fontSize(AppTheme.fontMicro)
            .fontColor(this.tMuted)
            .width('100%')
            .padding({ left: AppTheme.spaceXs, right: AppTheme.spaceXs, bottom: 28 })
        }
        .width('100%')
        .padding(AppTheme.pagePadH)
      }
      .layoutWeight(1)
      .scrollBar(BarState.Off)
      .align(Alignment.Top)
    }
    .width('100%')
    .height('100%')
    .backgroundColor(this.themeRev >= 0 ? this.tBg : this.tBg)
  }

  @Builder
  actionCard(title: string, subtitle: string, iconKind: string, tint: string, route: string, enabled: boolean) {
    Row({ space: 12 }) {
      AppSymbol({ kind: iconKind, tint: tint, iconSize: 22 })
      Column() {
        Text(title)
          .fontSize(AppTheme.fontBody)
          .fontWeight(FontWeight.Bold)
          .fontColor(enabled ? this.tText : this.tMuted)
        Text(subtitle)
          .fontSize(AppTheme.fontCaption)
          .fontColor(this.tMuted)
          .margin({ top: 4 })
          .maxLines(2)
      }
      .alignItems(HorizontalAlign.Start)
      .layoutWeight(1)
    }
    .width('100%')
    .padding(AppTheme.pagePadH)
    .backgroundColor(this.tCard)
    .borderRadius(AppTheme.rMd)
    .border({ width: AppTheme.hairline, color: this.tBorderSoft })
    .shadow({ radius: 4, color: this.tShadow, offsetY: 1 })
    .opacity(enabled ? 1 : 0.55)
    .onClick(() => {
      if (enabled) {
        NavUtil.push(route);
      }
    })
  }
}
