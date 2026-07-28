import { Api } from '../data/Api';
import { ApiClient } from '../data/ApiClient';
import { AppTheme } from '../common/Routes';
import { FormatUtil } from '../common/FormatUtil';
import { ActivityLogEntry } from '../data/models/Models';
import {
  EmptyHint,
  ErrorBanner,
  FilterChips,
  PageHeader,
  SearchBar
} from '../components/CommonUI';
import { NavUtil } from '../common/NavUtil';

@Entry
@Component
struct ActivityAuditPage {
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


  @State @Watch('onQueryChange') entries: ActivityLogEntry[] = [];
  @State filtered: ActivityLogEntry[] = [];
  @State @Watch('onQueryChange') query: string = '';
  @State @Watch('onQueryChange') kindIdx: number = 0;
  @State error: string = '';
  private kindLabels: string[] = ['全部', '操作', '系统', '插件', '终端'];
  private kindKeys: string[] = ['', 'operation', 'system', 'plugin', 'terminal'];

  aboutToAppear(): void {
    void this.load();
  }

  onQueryChange(): void {
    this.applyFilter();
  }

  private async load(): Promise<void> {
    this.error = '';
    try {
      this.entries = await Api.activity();
      this.applyFilter();
    } catch (e) {
      this.error = ApiClient.formatError(e as Object);
    }
  }

  private applyFilter(): void {
    const q = this.query.trim().toLowerCase();
    const kind = this.kindKeys[this.kindIdx] || '';
    const out: ActivityLogEntry[] = [];
    for (let i = 0; i < this.entries.length; i++) {
      const e = this.entries[i];
      if (kind && (e.kind || '').toLowerCase() !== kind) {
        continue;
      }
      if (q) {
        const hay =
          `${e.actor ?? ''} ${e.kind ?? ''} ${e.message ?? ''} ${e.host ?? ''} ${e.ip ?? ''}`.toLowerCase();
        if (hay.indexOf(q) < 0) {
          continue;
        }
      }
      out.push(e);
    }
    this.filtered = out;
  }

  build() {
    Column() {
      PageHeader({
        title: '操作审计',
        showBack: true,
        onBack: () => NavUtil.back(),
        trailing: '刷新',
        onTrailing: () => {
          void this.load();
        }
      })
      ErrorBanner({ message: this.error })
      FilterChips({
        labels: this.kindLabels,
        selected: $kindIdx,
        onSelect: (_i: number) => {
          this.applyFilter();
        }
      })
        .padding({ left: AppTheme.pagePadH, right: AppTheme.pagePadH, bottom: 8 })
      SearchBar({
        text: $query,
        placeholder: '搜索用户 / 动作 / 主机…'
      })
        .padding({ left: AppTheme.pagePadH, right: AppTheme.pagePadH, bottom: 8 })

      if (this.filtered.length === 0) {
        EmptyHint({ text: '暂无审计记录' })
      }

      List() {
        ForEach(this.filtered, (e: ActivityLogEntry, idx: number) => {
          ListItem() {
            Column() {
              Row() {
                Text(e.kind || e.level || '事件')
                  .fontSize(AppTheme.fontSmall)
                  .fontColor(this.tAccent)
                Blank()
                Text(FormatUtil.relTime(e.timestamp))
                  .fontSize(AppTheme.fontMicro)
                  .fontColor(this.tMuted)
              }
              .width('100%')
              Text(e.message || '—')
                .fontSize(AppTheme.fontBody)
                .fontColor(this.tText)
                .margin({ top: 6 })
                .maxLines(3)
              Text(`${e.actor || '—'} · ${e.host || e.ip || ''}`)
                .fontSize(AppTheme.fontCaption)
                .fontColor(this.tMuted)
                .margin({ top: 4 })
            }
            .width('100%')
            .padding(AppTheme.spaceMd)
            .backgroundColor(this.tCard)
            .borderRadius(AppTheme.rXs)
          }
          .margin({ left: 16, right: 16, bottom: 8 })
        }, (e: ActivityLogEntry, idx: number) => `${e.timestamp ?? 0}-${idx}`)
      }
      .layoutWeight(1)
    }
    .width('100%')
    .height('100%')
    .backgroundColor(this.tBg)
  }
}
