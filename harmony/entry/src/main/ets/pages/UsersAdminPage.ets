import { Api } from '../data/Api';
import { ApiClient } from '../data/ApiClient';
import { AccountUser, CreateUserRequest, UpdateUserRequest } from '../data/models/Models';
import {
  ConfirmDialogHost,
  EmptyHint,
  ErrorBanner,
  PageHeader,
  PrimaryButton
} from '../components/CommonUI';
import { NavUtil } from '../common/NavUtil';
import { AppTheme } from '../common/Routes';


@Entry
@Component
struct UsersAdminPage {
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

  @State users: AccountUser[] = [];
  @State error: string = '';
  @State showEditor: boolean = false;
  @State editorIsCreate: boolean = true;
  @State editUsername: string = '';
  @State formUser: string = '';
  @State formPass: string = '';
  @State formDisplay: string = '';
  @State formEmail: string = '';
  @State formRole: string = 'viewer';
  @State resetUser: string = '';
  @State resetPass: string = '';
  @State showReset: boolean = false;
  @State deleteTarget: string = '';
  @State showDelete: boolean = false;
  private roleOptions: string[] = ['viewer', 'operator', 'admin'];

  aboutToAppear(): void {
    void this.reload();
  }

  private async reload(): Promise<void> {
    this.error = '';
    try {
      this.users = await Api.users();
    } catch (e) {
      this.error = ApiClient.formatError(e as Object);
    }
  }

  private openCreate(): void {
    this.editorIsCreate = true;
    this.editUsername = '';
    this.formUser = '';
    this.formPass = '';
    this.formDisplay = '';
    this.formEmail = '';
    this.formRole = 'viewer';
    this.showEditor = true;
  }

  private openEdit(u: AccountUser): void {
    this.editorIsCreate = false;
    this.editUsername = u.username;
    this.formUser = u.username;
    this.formPass = '';
    this.formDisplay = u.display_name || '';
    this.formEmail = u.email || '';
    this.formRole = (u.role || 'viewer').trim() || 'viewer';
    this.showEditor = true;
  }

  private async saveEditor(): Promise<void> {
    if (this.editorIsCreate) {
      if (!this.formUser.trim() || !this.formPass) {
        this.error = '请填写用户名和密码';
        return;
      }
      const body: CreateUserRequest = {
        username: this.formUser.trim(),
        password: this.formPass,
        display_name: this.formDisplay.trim(),
        email: this.formEmail.trim(),
        role: this.formRole.trim() || 'viewer'
      };
      try {
        await Api.createUser(body);
        this.showEditor = false;
        await this.reload();
      } catch (e) {
        this.error = ApiClient.formatError(e as Object);
      }
      return;
    }
    const upd: UpdateUserRequest = {
      display_name: this.formDisplay.trim(),
      email: this.formEmail.trim(),
      role: this.formRole.trim() || 'viewer'
    };
    try {
      await Api.updateUser(this.editUsername, upd);
      this.showEditor = false;
      await this.reload();
    } catch (e) {
      this.error = ApiClient.formatError(e as Object);
    }
  }

  private async doDelete(): Promise<void> {
    try {
      await Api.deleteUser(this.deleteTarget);
      this.showDelete = false;
      this.deleteTarget = '';
      await this.reload();
    } catch (e) {
      this.error = ApiClient.formatError(e as Object);
    }
  }

  private async doReset(): Promise<void> {
    if (!this.resetPass) {
      this.error = '请填写新密码';
      return;
    }
    try {
      await Api.resetUserPassword(this.resetUser, this.resetPass);
      this.showReset = false;
      this.resetPass = '';
      this.resetUser = '';
    } catch (e) {
      this.error = ApiClient.formatError(e as Object);
    }
  }

  build() {
    Column() {
      PageHeader({
        title: '用户管理',
        showBack: true,
        onBack: () => NavUtil.back(),
        trailing: '刷新',
        onTrailing: () => {
          void this.reload();
        }
      })
      ErrorBanner({ message: this.error })

      Text('管理员可创建/改角色/删除用户。删除后对方 App 会话会失效。')
        .fontSize(AppTheme.fontCaption)
        .fontColor(this.tMuted)
        .padding({ left: 16, right: 16, bottom: 8 })

      PrimaryButton({
        label: '新建用户',
        onTap: () => this.openCreate()
      })
        .padding({ left: 16, right: 16, bottom: 12 })

      if (this.users.length === 0) {
        EmptyHint({ text: '暂无用户或无权限' })
      } else {
        List() {
          ForEach(this.users, (u: AccountUser) => {
            ListItem() {
              Column() {
                Row() {
                  Column() {
                    Text(u.display_name || u.username)
                      .fontSize(AppTheme.fontBody)
                      .fontWeight(FontWeight.Medium)
                      .fontColor(this.tText)
                    Text(`${u.username} · ${u.role ?? ''}${u.mfa_enabled ? ' · MFA' : ''}`)
                      .fontSize(AppTheme.fontCaption)
                      .fontColor(this.tMuted)
                      .margin({ top: 4 })
                    if (u.email) {
                      Text(u.email)
                        .fontSize(AppTheme.fontMicro)
                        .fontColor(this.tMuted)
                        .margin({ top: 2 })
                    }
                  }
                  .alignItems(HorizontalAlign.Start)
                  .layoutWeight(1)
                }
                .width('100%')
                Row({ space: 16 }) {
                  Text('编辑')
                    .fontSize(AppTheme.fontCaption)
                    .fontColor(this.tAccent)
                    .onClick(() => this.openEdit(u))
                  Text('重置密码')
                    .fontSize(AppTheme.fontCaption)
                    .fontColor(this.tAccent)
                    .onClick(() => {
                      this.resetUser = u.username;
                      this.resetPass = '';
                      this.showReset = true;
                    })
                  Text('删除')
                    .fontSize(AppTheme.fontCaption)
                    .fontColor(this.tDanger)
                    .onClick(() => {
                      this.deleteTarget = u.username;
                      this.showDelete = true;
                    })
                }
                .margin({ top: 8 })
              }
              .width('100%')
              .padding(AppTheme.spaceMd)
              .backgroundColor(this.tCard)
              .borderRadius(AppTheme.rXs)
            }
            .margin({ left: 16, right: 16, bottom: 8 })
          }, (u: AccountUser) => u.username)
        }
        .layoutWeight(1)
      }

      ConfirmDialogHost({
        visible: this.showDelete,
        title: '删除用户',
        message: `确认删除用户 ${this.deleteTarget}？其会话将被踢出。`,
        confirmText: '删除',
        danger: true,
        onConfirm: () => {
          void this.doDelete();
        },
        onCancel: () => {
          this.showDelete = false;
        }
      })

      if (this.showEditor) {
        Column() {
          Blank().layoutWeight(1).onClick(() => {
            this.showEditor = false;
          })
          Column({ space: 8 }) {
            Text(this.editorIsCreate ? '新建用户' : `编辑 ${this.editUsername}`)
              .fontSize(AppTheme.fontLead)
              .fontWeight(FontWeight.Medium)
              .fontColor(this.tText)
            if (this.editorIsCreate) {
              TextInput({ text: this.formUser, placeholder: '用户名' })
                .onChange((v: string) => {
                  this.formUser = v;
                })
                .height(40)
                .backgroundColor(this.tElevated)
                .fontColor(this.tText)
              TextInput({ text: this.formPass, placeholder: '密码' })
                .type(InputType.Password)
                .onChange((v: string) => {
                  this.formPass = v;
                })
                .height(40)
                .backgroundColor(this.tElevated)
                .fontColor(this.tText)
            }
            TextInput({ text: this.formDisplay, placeholder: '显示名' })
              .onChange((v: string) => {
                this.formDisplay = v;
              })
              .height(40)
              .backgroundColor(this.tElevated)
              .fontColor(this.tText)
            TextInput({ text: this.formEmail, placeholder: '邮箱' })
              .onChange((v: string) => {
                this.formEmail = v;
              })
              .height(40)
              .backgroundColor(this.tElevated)
              .fontColor(this.tText)
            Row({ space: 8 }) {
              ForEach(this.roleOptions, (r: string) => {
                Text(r)
                  .fontSize(AppTheme.fontCaption)
                  .fontColor(this.formRole === r ? this.tOnAccent : this.tText)
                  .padding({ left: 12, right: 12, top: 6, bottom: 6 })
                  .backgroundColor(this.formRole === r ? this.tAccent : this.tElevated)
                  .borderRadius(AppTheme.rXs)
                  .onClick(() => {
                    this.formRole = r;
                  })
              }, (r: string) => `role-${r}`)
            }
            Row({ space: 12 }) {
              Button('取消')
                .layoutWeight(1)
                .backgroundColor(this.tElevated)
                .onClick(() => {
                  this.showEditor = false;
                })
              Button('保存')
                .layoutWeight(1)
                .backgroundColor(this.tAccent)
                .onClick(() => {
                  void this.saveEditor();
                })
            }
            .margin({ top: 4 })
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

      if (this.showReset) {
        Column() {
          Blank().layoutWeight(1).onClick(() => {
            this.showReset = false;
          })
          Column() {
            Text(`重置密码 · ${this.resetUser}`)
              .fontSize(AppTheme.fontLead)
              .fontWeight(FontWeight.Medium)
              .fontColor(this.tText)
              .margin({ bottom: 12 })
            TextInput({ text: this.resetPass, placeholder: '新密码' })
              .type(InputType.Password)
              .onChange((v: string) => {
                this.resetPass = v;
              })
              .height(40)
              .backgroundColor(this.tElevated)
              .fontColor(this.tText)
            Row({ space: 12 }) {
              Button('取消')
                .layoutWeight(1)
                .backgroundColor(this.tElevated)
                .onClick(() => {
                  this.showReset = false;
                })
              Button('保存')
                .layoutWeight(1)
                .backgroundColor(this.tAccent)
                .onClick(() => {
                  void this.doReset();
                })
            }
            .margin({ top: 12 })
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
