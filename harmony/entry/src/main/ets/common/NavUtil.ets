import router from '@ohos.router';
import { hilog } from '@kit.PerformanceAnalysisKit';

/** Safe navigation wrappers — never crash UI on push failure. */
export class NavUtil {
  static push(url: string, params?: Record<string, string | number>): void {
    const task = params
      ? router.pushUrl({ url: url, params: params })
      : router.pushUrl({ url: url });
    task.catch((e: Error) => {
      hilog.error(0x0000, 'NavUtil', 'pushUrl failed %{public}s %{public}s', url, e.message);
    });
  }

  static replace(url: string, params?: Record<string, string | number>): void {
    const task = params
      ? router.replaceUrl({ url: url, params: params })
      : router.replaceUrl({ url: url });
    task.catch((e: Error) => {
      hilog.error(0x0000, 'NavUtil', 'replaceUrl failed %{public}s', e.message);
    });
  }

  static back(): void {
    try {
      router.back();
    } catch (e) {
      // ignore
    }
  }

  static paramString(key: string, fallback: string = ''): string {
    try {
      const params = router.getParams() as Record<string, Object>;
      if (params && params[key] !== undefined && params[key] !== null) {
        return String(params[key]);
      }
    } catch (e) {
      // ignore
    }
    return fallback;
  }

  static paramNumber(key: string, fallback: number = 0): number {
    const s = NavUtil.paramString(key, '');
    if (!s) {
      return fallback;
    }
    const n = Number(s);
    return isNaN(n) ? fallback : n;
  }
}
