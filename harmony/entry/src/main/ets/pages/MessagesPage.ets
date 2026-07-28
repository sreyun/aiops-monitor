import { Api } from '../data/Api';
import { ApiClient } from '../data/ApiClient';
import { AppTheme } from '../common/Routes';
import { FormatUtil } from '../common/FormatUtil';
import { MessageItem } from '../data/models/Models';
import {
  EmptyHint,
  ErrorBanner,
  PageHeader
} from '../components/CommonUI';
import { NavUtil } from '../common/NavUtil';

@Entry
@Component
struct MessagesPage {
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


  @State items: MessageItem[] = [];
  @State error: string = '';
  @State loading: boolean = false;

  aboutToAppear(): void {
    void this.load();
  }

  private async load(): Promise<void> {
    this.loading = true;
    this.error = '';
    try {
      const r = await Api.messages();
      this.items = r.items ?? [];
    } catch (e) {
      this.error = ApiClient.formatError(e as Object);
    } finally {
      this.loading = false;
    }
  }

  private bodyOf(m: MessageItem): string {
    return (m.body || m.content || '').trim();
  }

  private async markOne(m: MessageItem): Promise<void> {
    if (m.id === undefined || m.id === null) {
      return;
    }
    try {
      await Api.markMessagesRead([m.id]);
      await this.load();
    } catch (e) {
      this.error = ApiClient.formatError(e as Object);
    }
  }

  private async markAll(): Promise<void> {
    try {
      await Api.markAllMessagesRead();
      await this.load();
    } catch (e) {
      this.error = ApiClient.formatError(e as Object);
    }
  }

  build() {
    Column() {
      PageHeader({
        title: '消息中心',
        showBack: true,
        onBack: () => NavUtil.back(),
        trailing: '全部已读',
        onTrailing: () => {
          void this.markAll();
        }
      })
      ErrorBanner({
        message: this.error,
        showRetry: true,
        onRetry: () => {
          void this.load();
        }
      })

      if (!this.loading && this.items.length === 0) {
        EmptyHint({ text: '暂无消息' })
      }

      List() {
        ForEach(this.items, (m: MessageItem, idx: number) => {
          ListItem() {
            Column() {
              Row() {
                Text(m.title || '通知')
                  .fontSize(AppTheme.fontSubtitle)
                  .fontWeight(m.read ? FontWeight.Normal : FontWeight.Medium)
                  .fontColor(this.tText)
                  .layoutWeight(1)
                  .maxLines(1)
                  .textOverflow({ overflow: TextOverflow.Ellipsis })
                if (!m.read) {
                  Text('未读')
                    .fontSize(AppTheme.fontMicro)
                    .fontColor(this.tOnAccent)
                    .padding({ left: 8, right: 8, top: 2, bottom: 2 })
                    .backgroundColor(this.tWarning)
                    .borderRadius(AppTheme.rXs)
                }
              }
              .width('100%')
              if (this.bodyOf(m)) {
                Text(this.bodyOf(m))
                  .fontSize(AppTheme.fontSmall)
                  .fontColor(this.tMuted)
                  .margin({ top: 6 })
                  .maxLines(4)
                  .textOverflow({ overflow: TextOverflow.Ellipsis })
              }
              Text(FormatUtil.relTime(m.created_at))
                .fontSize(AppTheme.fontMicro)
                .fontColor(this.tMuted)
                .margin({ top: 6 })
            }
            .width('100%')
            .padding(AppTheme.cardPad)
            .backgroundColor(this.tCard)
            .borderRadius(AppTheme.rSm)
            .onClick(() => {
              void this.markOne(m);
            })
          }
          .margin({ left: 16, right: 16, bottom: 10 })
        }, (m: MessageItem, idx: number) => `${m.id ?? idx}`)
      }
      .layoutWeight(1)
      .padding({ top: 8 })
    }
    .width('100%')
    .height('100%')
    .backgroundColor(this.tBg)
  }
}
