import { LoginPanel } from '../components/LoginUI';
import { NavUtil } from '../common/NavUtil';

/** Standalone login — Android LoginScreen parity via shared LoginPanel. */
@Entry
@Component
struct LoginPage {
  @StorageLink('aiops_theme_rev') themeRev: number = 0;
  @StorageLink('aiops_bg') tBg: string = '#080C14';

  build() {
    Column() {
      LoginPanel({
        onSuccess: () => {
          NavUtil.replace('pages/Index');
        }
      })
    }
    .width('100%')
    .height('100%')
    .backgroundColor(this.tBg)
  }
}
