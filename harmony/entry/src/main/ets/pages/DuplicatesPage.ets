import { Api } from '../data/Api';
import { ApiClient } from '../data/ApiClient';
import { AppTheme } from '../common/Routes';
import { DupGroup } from '../data/models/Models';
import {
  ConfirmDialogHost,
  EmptyHint,
  ErrorBanner,
  PageHeader,
  PrimaryButton
} from '../components/CommonUI';
import { NavUtil } from '../common/NavUtil';

@Entry
@Component
struct DuplicatesPage {
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


  @State groups: DupGroup[] = [];
  @State error: string = '';
  @State showClean: boolean = false;
  @State busy: boolean = false;

  aboutToAppear(): void {
    void this.load();
  }

  private async load(): Promise<void> {
    this.error = '';
    try {
      const r = await Api.hostDuplicates();
      this.groups = r.groups ?? [];
    } catch (e) {
      this.error = ApiClient.formatError(e as Object);
    }
  }

  private idList(g: DupGroup): string {
    if (g.ids && g.ids.length > 0) {
      return g.ids.join(', ');
    }
    if (g.hosts && g.hosts.length > 0) {
      const parts: string[] = [];
      for (let i = 0; i < g.hosts.length; i++) {
        parts.push(g.hosts[i].id);
      }
      return parts.join(', ');
    }
    return '—';
  }

  private countOf(g: DupGroup): number {
    if (g.count !== undefined && g.count > 0) {
      return g.count;
    }
    if (g.ids) {
      return g.ids.length;
    }
    if (g.hosts) {
      return g.hosts.length;
    }
    return 0;
  }

  private async doCleanup(): Promise<void> {
    this.busy = true;
    this.error = '';
    try {
      await Api.cleanupDuplicates();
      this.showClean = false;
      await this.load();
    } catch (e) {
      this.error = ApiClient.formatError(e as Object);
    } finally {
      this.busy = false;
    }
  }

  build() {
    Column() {
      PageHeader({
        title: '重复主机',
        subtitle: '按机器指纹识别 · 重装 Agent 产生的同机旧身份',
        showBack: true,
        onBack: () => NavUtil.back(),
        trailingIcon: 'refresh',
        onTrailing: () => {
          void this.load();
        }
      })
      ErrorBanner({ message: this.error })

      if (this.groups.length === 0) {
        EmptyHint({ text: '无重复主机\n所有主机身份唯一，无需清理' })
      }

      List() {
        ForEach(this.groups, (g: DupGroup, idx: number) => {
          ListItem() {
            Column() {
              Text(g.hostname || '未知主机名')
                .fontSize(AppTheme.fontSubtitle)
                .fontWeight(FontWeight.Medium)
                .fontColor(this.tText)
              Text(`${this.countOf(g)} 条记录`)
                .fontSize(AppTheme.fontCaption)
                .fontColor(this.tWarning)
                .margin({ top: 4 })
              Text(this.idList(g))
                .fontSize(AppTheme.fontCaption)
                .fontColor(this.tMuted)
                .margin({ top: 6 })
                .maxLines(3)
                .textOverflow({ overflow: TextOverflow.Ellipsis })
            }
            .width('100%')
            .padding(AppTheme.cardPad)
            .backgroundColor(this.tCard)
            .borderRadius(AppTheme.rSm)
          }
          .margin({ left: 16, right: 16, bottom: 10 })
        }, (g: DupGroup, idx: number) => `${g.hostname ?? ''}-${idx}`)
      }
      .layoutWeight(1)

      if (this.groups.length > 0) {
        PrimaryButton({
          label: this.busy ? '清理中…' : '清理重复主机',
          isEnabled: !this.busy,
          onTap: () => {
            this.showClean = true;
          }
        })
          .padding({ left: 16, right: 16, bottom: 20 })
      }

      ConfirmDialogHost({
        visible: this.showClean,
        title: '清理重复主机',
        message: '将合并/清理重复主机记录，此操作不可撤销。确认继续？',
        confirmText: '清理',
        onConfirm: () => {
          void this.doCleanup();
        },
        onCancel: () => {
          this.showClean = false;
        }
      })
    }
    .width('100%')
    .height('100%')
    .backgroundColor(this.tBg)
  }
}
