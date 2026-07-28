import { ApiClient } from '../data/ApiClient';
import { PushClient } from '../data/push/PushClient';
import { SessionStore } from '../data/SessionStore';
import { MeResponse } from '../data/models/Models';
import { Api } from '../data/Api';
import { AppTheme } from './Routes';
import { ThemeSync } from './ThemeSync';
import { window } from '@kit.ArkUI';

type AuthListener = () => void;

/** Global app session state shared across pages. */
export class AppState {
  static me: MeResponse | null = null;
  static loggedIn: boolean = false;
  static bootstrapped: boolean = false;
  /** Transient network failure while cookie still valid — show retry instead of login. */
  static needsRetry: boolean = false;
  static statusText: string = '';
  static forceLoginToken: number = 0;
  static themeRev: number = 0;
  private static listeners: AuthListener[] = [];
  private static mainWindow: window.Window | null = null;

  static setMainWindow(win: window.Window | null): void {
    AppState.mainWindow = win;
    AppState.applyWindowChrome();
  }

  static applyWindowChrome(): void {
    if (!AppState.mainWindow) {
      return;
    }
    try {
      AppState.mainWindow.setWindowBackgroundColor(AppTheme.bg);
    } catch (_e) {
      // ignore chrome apply errors
    }
  }

  static addAuthListener(fn: AuthListener): void {
    AppState.listeners.push(fn);
  }

  static removeAuthListener(fn: AuthListener): void {
    const next: AuthListener[] = [];
    for (let i = 0; i < AppState.listeners.length; i++) {
      if (AppState.listeners[i] !== fn) {
        next.push(AppState.listeners[i]);
      }
    }
    AppState.listeners = next;
  }

  private static notifyAuth(): void {
    for (let i = 0; i < AppState.listeners.length; i++) {
      try {
        AppState.listeners[i]();
      } catch (e) {
        // ignore listener errors
      }
    }
  }

  private static commitTheme(dark: boolean): void {
    AppTheme.apply(dark);
    AppState.themeRev = AppTheme.rev;
    ThemeSync.publish();
    AppState.applyWindowChrome();
    AppState.notifyAuth();
  }

  static async loadTheme(): Promise<void> {
    try {
      const dark = await SessionStore.isDarkTheme();
      AppState.commitTheme(dark);
    } catch (e) {
      AppTheme.applyDark();
      ThemeSync.publish();
    }
  }

  /** Set absolute dark/light — used by Settings toggle. */
  static async setDarkThemeMode(dark: boolean): Promise<void> {
    if (AppTheme.dark === dark) {
      // Still persist in case store drifted
      await SessionStore.setDarkTheme(dark);
      ThemeSync.publish();
      return;
    }
    await SessionStore.setDarkTheme(dark);
    AppState.commitTheme(dark);
  }

  static async toggleTheme(): Promise<void> {
    const dark = await SessionStore.isDarkTheme();
    await AppState.setDarkThemeMode(!dark);
  }


  /**
   * Cold start: restore cookie → GET /me.
   * - 401 → clear session, show login
   * - network blip → keep cookie, needsRetry=true (do NOT force login form)
   */
  static async bootstrap(): Promise<boolean> {
    AppState.needsRetry = false;
    AppState.statusText = '';
    const restored = await ApiClient.restoreFromStore();
    if (!restored) {
      AppState.loggedIn = false;
      AppState.bootstrapped = true;
      return false;
    }
    try {
      const me = await Api.me();
      AppState.me = me;
      AppState.loggedIn = true;
      AppState.needsRetry = false;
      ApiClient.markAuthorized();
      PushClient.start();
      AppState.bootstrapped = true;
      return true;
    } catch (e) {
      const msg = ApiClient.formatError(e as Object);
      AppState.statusText = msg;
      if (ApiClient.isUnauthorizedPending() || !ApiClient.isSessionAlive()) {
        await ApiClient.clearPersistedSession();
        AppState.me = null;
        AppState.loggedIn = false;
        AppState.needsRetry = false;
        AppState.bootstrapped = true;
        return false;
      }
      // Transient failure: keep session materials, ask UI to retry
      AppState.me = null;
      AppState.loggedIn = false;
      AppState.needsRetry = true;
      AppState.bootstrapped = true;
      return false;
    }
  }

  static async retryBootstrap(): Promise<boolean> {
    return await AppState.bootstrap();
  }

  static async onLoginSuccess(username: string): Promise<void> {
    ApiClient.markAuthorized();
    await ApiClient.persistCookie();
    await SessionStore.setLastUsername(username);
    AppState.needsRetry = false;
    AppState.statusText = '';
    try {
      AppState.me = await Api.me();
    } catch (e) {
      AppState.me = { username: username };
    }
    AppState.loggedIn = true;
    PushClient.start();
    AppState.notifyAuth();
  }

  static async logout(): Promise<void> {
    PushClient.stop();
    try {
      await Api.logout();
    } catch (e) {
      // ignore logout transport errors
    }
    await ApiClient.clearPersistedSession();
    AppState.me = null;
    AppState.loggedIn = false;
    AppState.needsRetry = false;
    AppState.statusText = '';
    AppState.forceLoginToken++;
    AppState.notifyAuth();
  }

  static wireUnauthorized(): void {
    ApiClient.onUnauthorized = () => {
      PushClient.stop();
      void SessionStore.clearCookie();
      ApiClient.clearSessionMemory();
      AppState.me = null;
      AppState.loggedIn = false;
      AppState.needsRetry = false;
      AppState.statusText = '会话已失效，请重新登录';
      AppState.forceLoginToken++;
      AppState.notifyAuth();
    };
  }
}
