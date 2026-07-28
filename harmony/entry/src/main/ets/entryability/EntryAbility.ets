import { AbilityConstant, UIAbility, Want } from '@kit.AbilityKit';
import { window } from '@kit.ArkUI';
import { SessionStore } from '../data/SessionStore';
import { AppState } from '../common/AppState';
import { ApiClient } from '../data/ApiClient';
import { hilog } from '@kit.PerformanceAnalysisKit';
import { PushClient } from '../data/push/PushClient';
import { ThemeSync } from '../common/ThemeSync';

export default class EntryAbility extends UIAbility {
  private pendingDeepLink: string = '';
  private contentLoaded: boolean = false;

  onCreate(want: Want, _launchParam: AbilityConstant.LaunchParam): void {
    ThemeSync.ensure();
    this.captureDeepLink(want);
  }

  onNewWant(want: Want, _launchParam: AbilityConstant.LaunchParam): void {
    this.captureDeepLink(want);
    if (this.pendingDeepLink) {
      ApiClient.pendingDeepLink = this.pendingDeepLink;
    }
  }

  private captureDeepLink(want: Want): void {
    const params = want.parameters;
    if (!params) {
      return;
    }
    const route = params['aiops_nav_route'];
    if (route !== undefined && route !== null) {
      this.pendingDeepLink = String(route);
    }
  }

  onWindowStageCreate(windowStage: window.WindowStage): void {
    SessionStore.init(this.context).then(async () => {
      await AppState.loadTheme();
      AppState.wireUnauthorized();
      if (this.pendingDeepLink) {
        ApiClient.pendingDeepLink = this.pendingDeepLink;
      }
      windowStage.getMainWindow().then((win: window.Window) => {
        AppState.setMainWindow(win);
      }).catch(() => {
        // ignore
      });
      windowStage.loadContent('pages/Index', (err) => {
        if (err.code) {
          hilog.error(0x0000, 'EntryAbility', 'loadContent failed: %{public}s', JSON.stringify(err));
        } else {
          this.contentLoaded = true;
        }
      });
    }).catch((e: Error) => {
      hilog.error(0x0000, 'EntryAbility', 'SessionStore init failed: %{public}s', e.message);
      AppState.wireUnauthorized();
      windowStage.loadContent('pages/Index', (_err) => {
        this.contentLoaded = true;
      });
    });
  }

  onForeground(): void {
    // Resume push if session still valid
    if (ApiClient.isSessionAlive() && AppState.loggedIn) {
      PushClient.start();
    }
  }

  onBackground(): void {
    // Keep WS if possible; PushClient handles reconnect. No forced stop.
  }

  onDestroy(): void {
    PushClient.stop();
  }
}
