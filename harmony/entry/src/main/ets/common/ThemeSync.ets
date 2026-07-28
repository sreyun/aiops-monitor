import { AppTheme } from './Routes';

/** AppStorage keys — @StorageLink must use string literals matching these. */
export const THEME_REV_KEY: string = 'aiops_theme_rev';
export const THEME_DARK_KEY: string = 'aiops_theme_dark';

/**
 * Publish full palette into AppStorage.
 * Components should @StorageLink color keys (or theme rev) so ArkUI rebinds paints.
 * Static AppTheme fields alone never trigger UI refresh.
 */
export class ThemeSync {
  private static put(key: string, value: string | number | boolean): void {
    try {
      AppStorage.setOrCreate(key, value);
    } catch (_e) {
      // AppStorage may be unavailable before first UI frame
    }
  }

  static ensure(): void {
    ThemeSync.publish();
  }

  static publish(): void {
    ThemeSync.put('aiops_theme_rev', AppTheme.rev);
    ThemeSync.put('aiops_theme_dark', AppTheme.dark);
    ThemeSync.put('aiops_bg', AppTheme.bg);
    ThemeSync.put('aiops_card', AppTheme.card);
    ThemeSync.put('aiops_elevated', AppTheme.elevated);
    ThemeSync.put('aiops_text', AppTheme.text);
    ThemeSync.put('aiops_muted', AppTheme.muted);
    ThemeSync.put('aiops_accent', AppTheme.accent);
    ThemeSync.put('aiops_accent_soft', AppTheme.accentSoft);
    ThemeSync.put('aiops_accent_on', AppTheme.accentOn);
    ThemeSync.put('aiops_success', AppTheme.success);
    ThemeSync.put('aiops_success_soft', AppTheme.successSoft);
    ThemeSync.put('aiops_warning', AppTheme.warning);
    ThemeSync.put('aiops_warning_soft', AppTheme.warningSoft);
    ThemeSync.put('aiops_danger', AppTheme.danger);
    ThemeSync.put('aiops_danger_soft', AppTheme.dangerSoft);
    ThemeSync.put('aiops_border', AppTheme.border);
    ThemeSync.put('aiops_border_soft', AppTheme.borderSoft);
    ThemeSync.put('aiops_on_accent', AppTheme.onAccent);
    ThemeSync.put('aiops_nav_bar', AppTheme.navBar);
    ThemeSync.put('aiops_scrim', AppTheme.scrim);
    ThemeSync.put('aiops_shadow', AppTheme.shadow);
  }

  static stamp(rev: number): number {
    return rev;
  }
}
