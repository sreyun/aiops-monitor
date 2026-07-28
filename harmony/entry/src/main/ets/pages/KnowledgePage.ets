import { Api } from '../data/Api';
import { ApiClient } from '../data/ApiClient';
import { AppTheme } from '../common/Routes';
import { KnowledgeRule, KnowledgeTemplate } from '../data/models/Models';
import {
  ConfirmDialogHost,
  EmptyHint,
  ErrorBanner,
  FilterChips,
  PageHeader,
  PrimaryButton
} from '../components/CommonUI';
import { NavUtil } from '../common/NavUtil';

@Entry
@Component
struct KnowledgePage {
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


  @State tabIdx: number = 0;
  private tabLabels: string[] = ['规则', '模板'];
  @State rules: KnowledgeRule[] = [];
  @State templates: KnowledgeTemplate[] = [];
  @State error: string = '';
  @State showForm: boolean = false;
  @State editId: number = 0;
  @State formName: string = '';
  @State formDesc: string = '';
  @State formConfig: string = '';
  @State formEnabled: boolean = true;
  @State formPriority: number = 0;
  @State deleteId: number = 0;
  @State showDelete: boolean = false;

  aboutToAppear(): void {
    void this.load();
  }

  private async load(): Promise<void> {
    this.error = '';
    try {
      this.rules = await Api.knowledgeRules();
      this.templates = await Api.knowledgeTemplates();
    } catch (e) {
      this.error = ApiClient.formatError(e as Object);
    }
  }

  private openCreate(): void {
    this.editId = 0;
    this.formName = '';
    this.formDesc = '';
    this.formConfig = '';
    this.formEnabled = true;
    this.formPriority = 0;
    this.showForm = true;
  }

  private openEdit(r: KnowledgeRule): void {
    this.editId = r.id ?? 0;
    this.formName = r.name ?? '';
    this.formDesc = r.description ?? '';
    this.formConfig = r.config ?? '';
    this.formEnabled = r.enabled !== false;
    this.formPriority = r.priority ?? 0;
    this.showForm = true;
  }

  private async toggleEnabled(r: KnowledgeRule, enabled: boolean): Promise<void> {
    const body: KnowledgeRule = {
      id: r.id,
      name: r.name,
      description: r.description,
      config: r.config,
      priority: r.priority,
      enabled: enabled
    };
    try {
      await Api.upsertKnowledgeRule(body);
      await this.load();
    } catch (e) {
      this.error = ApiClient.formatError(e as Object);
    }
  }

  private async saveRule(): Promise<void> {
    if (!this.formName.trim()) {
      this.error = '请填写规则名称';
      return;
    }
    const body: KnowledgeRule = {
      id: this.editId > 0 ? this.editId : undefined,
      name: this.formName.trim(),
      description: this.formDesc.trim(),
      config: this.formConfig,
      priority: this.formPriority,
      enabled: this.formEnabled
    };
    try {
      await Api.upsertKnowledgeRule(body);
      this.showForm = false;
      await this.load();
    } catch (e) {
      this.error = ApiClient.formatError(e as Object);
    }
  }

  private async doDelete(): Promise<void> {
    try {
      await Api.deleteKnowledgeRule(this.deleteId);
      this.showDelete = false;
      await this.load();
    } catch (e) {
      this.error = ApiClient.formatError(e as Object);
    }
  }

  build() {
    Column() {
      PageHeader({
        title: '知识库',
        showBack: true,
        onBack: () => NavUtil.back(),
        trailing: '刷新',
        onTrailing: () => {
          void this.load();
        }
      })
      ErrorBanner({ message: this.error })
      FilterChips({
        labels: this.tabLabels,
        selected: $tabIdx
      })
        .padding({ left: 16, right: 16, bottom: 8 })

      if (this.tabIdx === 0) {
        PrimaryButton({
          label: '新建规则',
          onTap: () => this.openCreate()
        })
          .padding({ left: 16, right: 16, bottom: 8 })
        if (this.rules.length === 0) {
          EmptyHint({ text: '暂无规则' })
        }
        List() {
          ForEach(this.rules, (r: KnowledgeRule) => {
            ListItem() {
              Column() {
                Text(r.name || `规则 #${r.id ?? ''}`)
                  .fontSize(AppTheme.fontBody)
                  .fontWeight(FontWeight.Medium)
                  .fontColor(this.tText)
                Text(r.description || '')
                  .fontSize(AppTheme.fontCaption)
                  .fontColor(this.tMuted)
                  .margin({ top: 4 })
                  .maxLines(2)
                Row({ space: 16 }) {
                  Toggle({ type: ToggleType.Switch, isOn: r.enabled !== false })
                    .selectedColor(this.tSuccess)
                    .onChange((isOn: boolean) => {
                      void this.toggleEnabled(r, isOn);
                    })
                  Text(r.enabled === false ? '已停用' : '已启用')
                    .fontSize(AppTheme.fontCaption)
                    .fontColor(r.enabled === false ? this.tMuted : this.tSuccess)
                  Text('编辑')
                    .fontSize(AppTheme.fontCaption)
                    .fontColor(this.tAccent)
                    .onClick(() => this.openEdit(r))
                  Text('删除')
                    .fontSize(AppTheme.fontCaption)
                    .fontColor(this.tDanger)
                    .onClick(() => {
                      this.deleteId = r.id ?? 0;
                      this.showDelete = true;
                    })
                }
                .margin({ top: 8 })
                .alignItems(VerticalAlign.Center)
              }
              .width('100%')
              .padding(AppTheme.spaceMd)
              .backgroundColor(this.tCard)
              .borderRadius(AppTheme.rXs)
            }
            .margin({ left: 16, right: 16, bottom: 8 })
          }, (r: KnowledgeRule) => `r-${r.id ?? r.name}`)
        }
        .layoutWeight(1)
      } else {
        if (this.templates.length === 0) {
          EmptyHint({ text: '暂无模板' })
        }
        List() {
          ForEach(this.templates, (t: KnowledgeTemplate) => {
            ListItem() {
              Column() {
                Text(t.name || `模板 #${t.id ?? ''}`)
                  .fontSize(AppTheme.fontBody)
                  .fontWeight(FontWeight.Medium)
                  .fontColor(this.tText)
                Text(t.category || '')
                  .fontSize(AppTheme.fontMicro)
                  .fontColor(this.tAccent)
                  .margin({ top: 2 })
                Text(t.description || t.content || '')
                  .fontSize(AppTheme.fontCaption)
                  .fontColor(this.tMuted)
                  .margin({ top: 4 })
                  .maxLines(4)
              }
              .width('100%')
              .padding(AppTheme.spaceMd)
              .backgroundColor(this.tCard)
              .borderRadius(AppTheme.rXs)
            }
            .margin({ left: 16, right: 16, bottom: 8 })
          }, (t: KnowledgeTemplate) => `t-${t.id ?? t.name}`)
        }
        .layoutWeight(1)
      }

      ConfirmDialogHost({
        visible: this.showDelete,
        title: '删除规则',
        message: '确认删除该知识库规则？',
        confirmText: '删除',
        danger: true,
        onConfirm: () => {
          void this.doDelete();
        },
        onCancel: () => {
          this.showDelete = false;
        }
      })

      if (this.showForm) {
        Column() {
          Blank().layoutWeight(1).onClick(() => {
            this.showForm = false;
          })
          Column({ space: 8 }) {
            Text(this.editId > 0 ? '编辑规则' : '新建规则')
              .fontSize(AppTheme.fontLead)
              .fontWeight(FontWeight.Medium)
              .fontColor(this.tText)
            TextInput({ text: this.formName, placeholder: '名称' })
              .onChange((v: string) => {
                this.formName = v;
              })
              .height(40)
              .backgroundColor(this.tElevated)
              .fontColor(this.tText)
            TextInput({ text: this.formDesc, placeholder: '描述' })
              .onChange((v: string) => {
                this.formDesc = v;
              })
              .height(40)
              .backgroundColor(this.tElevated)
              .fontColor(this.tText)
            TextArea({ text: this.formConfig, placeholder: '配置 / 内容' })
              .onChange((v: string) => {
                this.formConfig = v;
              })
              .height(100)
              .backgroundColor(this.tElevated)
              .fontColor(this.tText)
            Row() {
              Text('启用')
                .fontSize(AppTheme.fontBody)
                .fontColor(this.tText)
                .layoutWeight(1)
              Toggle({ type: ToggleType.Switch, isOn: this.formEnabled })
                .selectedColor(this.tSuccess)
                .onChange((isOn: boolean) => {
                  this.formEnabled = isOn;
                })
            }
            .width('100%')
            .padding({ top: 4, bottom: 4 })
            Row({ space: 12 }) {
              Button('取消')
                .layoutWeight(1)
                .backgroundColor(this.tElevated)
                .onClick(() => {
                  this.showForm = false;
                })
              Button('保存')
                .layoutWeight(1)
                .backgroundColor(this.tAccent)
                .onClick(() => {
                  void this.saveRule();
                })
            }
          }
          .width('100%')
          .padding(AppTheme.pagePadH)
          .backgroundColor(this.tCard)
          .borderRadius({ topLeft: AppTheme.rMd, topRight: AppTheme.rMd })
        }
        .width('100%')
        .height('100%')
        .backgroundColor(this.tScrim)
        .position({ x: 0, y: 0 })
        .zIndex(40)
      }
    }
    .width('100%')
    .height('100%')
    .backgroundColor(this.tBg)
  }
}
