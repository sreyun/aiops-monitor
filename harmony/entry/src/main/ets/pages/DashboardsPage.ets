import { Api } from '../data/Api';
import { ApiClient } from '../data/ApiClient';
import { DashboardMeta } from '../data/models/Models';
import {
  EmptyHint,
  ErrorBanner,
  PageHeader
} from '../components/CommonUI';
import { NavUtil } from '../common/NavUtil';
import { AppTheme } from '../common/Routes';


@Entry
@Component
struct DashboardsPage {
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

  @State items: DashboardMeta[] = [];
  @State error: string = '';
  @State loading: boolean = false;

  aboutToAppear(): void {
    void this.load();
  }

  private async load(): Promise<void> {
    this.error = '';
    this.loading = true;
    try {
      const r = await Api.dashboards();
      const list = r.dashboards ?? [];
      list.sort((a: DashboardMeta, b: DashboardMeta) => (b.updated_at ?? 0) - (a.updated_at ?? 0));
      this.items = list;
    } catch (e) {
      this.error = ApiClient.formatError(e as Object);
    } finally {
      this.loading = false;
    }
  }

  private panelCount(d: DashboardMeta): number {
    const n = d.panels;
    if (typeof n === 'number') {
      return n;
    }
    return 0;
  }

  private sourceHint(d: DashboardMeta): string {
    const s = (d.source || '').trim();
    if (!s) {
      return '自定义';
    }
    if (s.indexOf('grafana') >= 0) {
      return 'Grafana';
    }
    if (s === 'import') {
      return '导入';
    }
    if (s === 'ai') {
      return 'AI';
    }
    return s;
  }

  build() {
    Column() {
      PageHeader({
        title: '仪表盘',
        showBack: true,
        onBack: () => NavUtil.back(),
        trailing: '刷新',
        onTrailing: () => {
          void this.load();
        }
      })
      Text('自定义面板 · 只读查看')
        .fontSize(AppTheme.fontCaption)
        .fontColor(this.tMuted)
        .padding({ left: 16, bottom: 8 })
      ErrorBanner({ message: this.error })
      if (!this.loading && this.items.length === 0 && !this.error) {
        EmptyHint({ text: '暂无仪表盘\n可在 Web 端创建或导入 Grafana 看板' })
      }
      List() {
        ForEach(this.items, (d: DashboardMeta) => {
          ListItem() {
            Row() {
              Column() {
                Text(d.name || d.id)
                  .fontSize(AppTheme.fontSubtitle)
                  .fontWeight(FontWeight.Medium)
                  .fontColor(this.tText)
                  .maxLines(1)
                  .textOverflow({ overflow: TextOverflow.Ellipsis })
                if (d.description) {
                  Text(d.description)
                    .fontSize(AppTheme.fontCaption)
                    .fontColor(this.tMuted)
                    .margin({ top: 4 })
                    .maxLines(2)
                    .textOverflow({ overflow: TextOverflow.Ellipsis })
                }
                Text(`${this.panelCount(d)} 个面板 · ${this.sourceHint(d)}`)
                  .fontSize(AppTheme.fontMicro)
                  .fontColor(this.tMuted)
                  .margin({ top: 4 })
              }
              .alignItems(HorizontalAlign.Start)
              .layoutWeight(1)
              Text('›')
                .fontSize(AppTheme.fontDisplay)
                .fontColor(this.tMuted)
            }
            .width('100%')
            .padding(AppTheme.cardPad)
            .backgroundColor(this.tCard)
            .borderRadius(AppTheme.rSm)
            .onClick(() => {
              NavUtil.push('pages/DashboardViewPage', { dashboardId: d.id });
            })
          }
          .margin({ left: 16, right: 16, bottom: 10 })
        }, (d: DashboardMeta) => d.id)
      }
      .layoutWeight(1)
      .padding({ top: 8 })
    }
    .width('100%')
    .height('100%')
    .backgroundColor(this.tBg)
  }
}
