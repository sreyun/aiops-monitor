/** Route names aligned with Android Routes in AIOpsApp.kt */
export class Routes {
  static readonly LOGIN = 'login';
  static readonly DASHBOARD = 'dashboard';
  static readonly ALERTS = 'alerts';
  static readonly MONITOR = 'monitor';
  static readonly OPERATIONS = 'operations';
  static readonly AI_COPILOT = 'ai_copilot';
  static readonly HOST_DETAIL = 'host_detail';
  static readonly SETTINGS = 'settings';
  static readonly TERMINAL = 'terminal';
  static readonly TERMINAL_PASSWORD = 'terminal_password';
  static readonly TERMINAL_SESSIONS = 'terminal_sessions';
  static readonly TERMINAL_REPLAY = 'terminal_replay';
  static readonly MESSAGES = 'messages';
  static readonly DUPLICATES = 'duplicates';
  static readonly INSTALL_AGENT = 'install_agent';
  static readonly COST_MANAGEMENT = 'cost_management';
  static readonly USERS_ADMIN = 'users_admin';
  static readonly ACTIVITY_AUDIT = 'activity_audit';
  static readonly KNOWLEDGE = 'knowledge';
  static readonly ALERT_EXTRAS = 'alert_extras';
  static readonly DASHBOARDS = 'dashboards';
  static readonly DASHBOARD_VIEW = 'dashboard_view';
  static readonly EXECUTION_DETAIL = 'execution_detail';
}

/**
 * Visual tokens aligned with Android Theme.kt + Components.kt AppBlue/Green/Orange/Red.
 * Default dark (Android AIOpsMonitorTheme darkTheme=true).
 */
export class AppTheme {
  static dark: boolean = true;
  static rev: number = 0;

  /** Fixed semantic brand (Android AppBlue) — used for metrics/status even in light mode */
  static readonly brand: string = '#4F7FFF';
  static readonly brandGreen: string = '#00A86B';
  static readonly brandOrange: string = '#F59E0B';
  static readonly brandRed: string = '#EF4444';

  /** Layout rhythm (Android 16dp page / 12dp card / 8dp chip gap) */
  static readonly spaceXs: number = 4;
  static readonly spaceSm: number = 8;
  static readonly spaceMd: number = 12;
  static readonly spaceLg: number = 16;
  static readonly spaceXl: number = 24;
  /** Typography (vp) — prefer these over magic font sizes */
  static readonly fontDisplay: number = 22;
  static readonly fontHero: number = 28;
  static readonly fontTitle: number = 18;
  static readonly fontLead: number = 16;
  static readonly fontSubtitle: number = 15;
  static readonly fontBody: number = 14;
  static readonly fontSmall: number = 13;
  static readonly fontCaption: number = 12;
  static readonly fontMicro: number = 11;
  static readonly fontTiny: number = 10;
  /** Chrome layout */
  static readonly pagePadH: number = 16;
  static readonly cardPad: number = 14;
  static readonly listGap: number = 8;
  static readonly navBarH: number = 54;
  static readonly headerIcon: number = 40;
  static readonly hairline: number = 1;
  static readonly fabSize: number = 48;
  static readonly sendBtn: number = 28;

  static bg: string = '#080C14';
  static card: string = '#111722';
  static elevated: string = '#192130';
  static text: string = '#E6E8F0';
  static muted: string = '#9AA3B8';
  static accent: string = '#4F7FFF';
  static accentSoft: string = '#1E3166';
  static accentOn: string = '#D6E2FF';
  static success: string = '#00A86B';
  static successSoft: string = '#0F3A2C';
  static warning: string = '#F59E0B';
  static warningSoft: string = '#3A2A10';
  static danger: string = '#EF4444';
  static dangerSoft: string = '#3A1614';
  static border: string = '#39445A';
  static borderSoft: string = '#222C3D';
  static scrim: string = '#99000000';
  static onAccent: string = '#FFFFFF';
  static navBar: string = '#111722';
  static shadow: string = '#40000000';

  /** Shape scale (Android AppShapes) */
  static readonly rChip: number = 6;
  static readonly rXs: number = 8;
  static readonly rSm: number = 12;
  static readonly rMd: number = 16;
  static readonly rLg: number = 22;
  static readonly rXl: number = 30;

  static applyDark(): void {
    AppTheme.dark = true;
    AppTheme.bg = '#080C14';
    AppTheme.card = '#111722';
    AppTheme.elevated = '#192130';
    AppTheme.text = '#E6E8F0';
    AppTheme.muted = '#9AA3B8';
    AppTheme.accent = '#4F7FFF';
    AppTheme.accentSoft = '#1E3166';
    AppTheme.accentOn = '#D6E2FF';
    AppTheme.success = '#00A86B';
    AppTheme.successSoft = '#0F3A2C';
    AppTheme.warning = '#F59E0B';
    AppTheme.warningSoft = '#3A2A10';
    AppTheme.danger = '#EF4444';
    AppTheme.dangerSoft = '#3A1614';
    AppTheme.border = '#39445A';
    AppTheme.borderSoft = '#222C3D';
    AppTheme.onAccent = '#FFFFFF';
    AppTheme.navBar = '#111722';
    AppTheme.scrim = '#99000000';
    AppTheme.shadow = '#66000000';
    AppTheme.rev += 1;
  }

  static applyLight(): void {
    AppTheme.dark = false;
    AppTheme.bg = '#F5F7FB';
    AppTheme.card = '#FFFFFF';
    AppTheme.elevated = '#F0F4FA';
    AppTheme.text = '#111827';
    AppTheme.muted = '#64748B';
    AppTheme.accent = '#2563EB';
    AppTheme.accentSoft = '#DBEAFE';
    AppTheme.accentOn = '#163B8F';
    AppTheme.success = '#059669';
    AppTheme.successSoft = '#C8F5E6';
    AppTheme.warning = '#D97706';
    AppTheme.warningSoft = '#FEF3C7';
    AppTheme.danger = '#D9261E';
    AppTheme.dangerSoft = '#FDDCD8';
    AppTheme.border = '#CBD5E1';
    AppTheme.borderSoft = '#E2E8F0';
    AppTheme.onAccent = '#FFFFFF';
    AppTheme.navBar = '#FFFFFF';
    AppTheme.scrim = '#80000000';
    AppTheme.shadow = '#1A000000';
    AppTheme.rev += 1;
  }

  static apply(dark: boolean): void {
    if (dark) {
      AppTheme.applyDark();
    } else {
      AppTheme.applyLight();
    }
  }
}

// ThemeSync.publish is called from AppState after apply — avoid circular import here.
