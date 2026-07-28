import { AppTheme } from './Routes';

export class FormatUtil {
  static relTime(ts?: number): string {
    if (ts === undefined || ts === null || ts <= 0) {
      return '—';
    }
    const millis = ts > 1_000_000_000_000 ? ts : ts * 1000;
    const diff = Math.floor((Date.now() - millis) / 1000);
    if (diff < 60) {
      return '刚刚';
    }
    if (diff < 3600) {
      return `${Math.floor(diff / 60)}分钟前`;
    }
    if (diff < 86400) {
      return `${Math.floor(diff / 3600)}小时前`;
    }
    return `${Math.floor(diff / 86400)}天前`;
  }

  static levelLabel(level: string): string {
    const l = (level || '').toLowerCase();
    if (l === 'critical' || l === '严重') {
      return '严重';
    }
    if (l === 'warning' || l === '警告') {
      return '警告';
    }
    if (l === 'info' || l === 'information' || l === '信息') {
      return '信息';
    }
    return level || '信息';
  }

  static sevColor(level: string): string {
    const l = (level || '').toLowerCase();
    if (l === 'critical' || l === '严重' || l === 'error' || l === 'down' || l === 'offline') {
      return AppTheme.danger;
    }
    if (l === 'warning' || l === '警告' || l === 'warn') {
      return AppTheme.warning;
    }
    if (l === 'ok' || l === 'success' || l === 'up' || l === 'online' || l === 'info') {
      return AppTheme.success;
    }
    return AppTheme.accent;
  }

  /** Soft pill background (Android StatusPill alpha 0.15 style). */
  static sevSoft(level: string): string {
    const l = (level || '').toLowerCase();
    if (l === 'critical' || l === '严重' || l === 'error' || l === 'down' || l === 'offline') {
      return AppTheme.dangerSoft;
    }
    if (l === 'warning' || l === '警告' || l === 'warn') {
      return AppTheme.warningSoft;
    }
    if (l === 'ok' || l === 'success' || l === 'up' || l === 'online') {
      return AppTheme.successSoft;
    }
    return AppTheme.accentSoft;
  }

  /** Alert / incident workflow status color (Android AlertsScreen). */
  static statusColor(status: string): string {
    const s = (status || '').toLowerCase();
    if (s === 'acknowledged' || s === 'acked' || s === 'confirmed') {
      return AppTheme.accent;
    }
    if (s === 'silenced' || s === 'muted' || s === 'suppressed') {
      return AppTheme.warning;
    }
    if (s === 'resolved' || s === 'closed' || s === 'done') {
      return AppTheme.success;
    }
    return AppTheme.danger;
  }

  static statusSoft(status: string): string {
    const s = (status || '').toLowerCase();
    if (s === 'acknowledged' || s === 'acked' || s === 'confirmed') {
      return AppTheme.accentSoft;
    }
    if (s === 'silenced' || s === 'muted' || s === 'suppressed') {
      return AppTheme.warningSoft;
    }
    if (s === 'resolved' || s === 'closed' || s === 'done') {
      return AppTheme.successSoft;
    }
    return AppTheme.dangerSoft;
  }

  static alertStatusZh(status: string): string {
    const s = (status || '').toLowerCase();
    if (s === 'acknowledged' || s === 'acked' || s === 'confirmed') {
      return '已确认';
    }
    if (s === 'silenced' || s === 'muted' || s === 'suppressed') {
      return '已静默';
    }
    if (s === 'resolved' || s === 'closed') {
      return '已解决';
    }
    return '待处理';
  }

  static incidentStatusZh(status: string): string {
    const s = (status || '').toLowerCase();
    if (s === 'acknowledged' || s === 'acked') {
      return '已确认';
    }
    if (s === 'resolved') {
      return '已解决';
    }
    if (s === 'closed') {
      return '已关闭';
    }
    if (s === 'escalated') {
      return '已升级';
    }
    if (s === 'investigating' || s === 'in_progress') {
      return '处理中';
    }
    if (s === 'open' || s === 'active' || s === 'firing' || !s) {
      return '开放';
    }
    return status;
  }

  static ticketStatusZh(status: string): string {
    const s = (status || '').toLowerCase();
    if (s === 'open' || s === 'new') {
      return '开放';
    }
    if (s === 'in_progress' || s === 'assigned') {
      return '处理中';
    }
    if (s === 'resolved') {
      return '已解决';
    }
    if (s === 'closed' || s === 'done') {
      return '已关闭';
    }
    return status || '—';
  }

  static healthZh(status: string): string {
    const s = (status || '').toLowerCase();
    if (s === 'ok' || s === 'healthy' || s === 'good' || s === 'normal') {
      return '正常';
    }
    if (s === 'warning' || s === 'degraded' || s === 'warn') {
      return '告警';
    }
    if (s === 'critical' || s === 'bad' || s === 'error' || s === 'fail') {
      return '严重';
    }
    if (s === 'unknown') {
      return '未知';
    }
    return status || '—';
  }

  /** Android thresholdColor: >=85 red, >=70 orange, else AppBlue */
  static thresholdColor(percent: number): string {
    if (percent >= 85) {
      return AppTheme.brandRed;
    }
    if (percent >= 70) {
      return AppTheme.brandOrange;
    }
    return AppTheme.brand;
  }

  static latencyColor(ms: number): string {
    if (ms <= 0) {
      return AppTheme.muted;
    }
    if (ms >= 1000) {
      return AppTheme.brandRed;
    }
    if (ms >= 300) {
      return AppTheme.brandOrange;
    }
    return AppTheme.brandGreen;
  }

  static pct(v?: number): string {
    if (v === undefined || v === null || isNaN(Number(v))) {
      return '—';
    }
    return `${Math.round(Number(v))}%`;
  }

  static uptimeZh(sec?: number): string {
    if (sec === undefined || sec === null || sec <= 0) {
      return '—';
    }
    const s = Math.floor(Number(sec));
    const days = Math.floor(s / 86400);
    const hours = Math.floor((s % 86400) / 3600);
    const mins = Math.floor((s % 3600) / 60);
    if (days >= 1) {
      return `${days}天 ${hours}时`;
    }
    return `${hours}时 ${mins}分`;
  }

  static firstSeenZh(ts?: number): string {
    if (!ts || ts <= 0) {
      return '—';
    }
    const d = new Date(ts * 1000);
    const y = d.getFullYear();
    const m = d.getMonth() + 1;
    const day = d.getDate();
    const hh = d.getHours();
    const mi = d.getMinutes();
    const pad = (n: number): string => (n < 10 ? `0${n}` : `${n}`);
    return `${y}-${pad(m)}-${pad(day)} ${pad(hh)}:${pad(mi)}`;
  }

  static bytesRate(v?: number): string {
    if (v === undefined || v === null || isNaN(Number(v))) {
      return '—';
    }
    const n = Number(v);
    if (n < 1024) {
      return `${n.toFixed(0)} B/s`;
    }
    if (n < 1024 * 1024) {
      return `${(n / 1024).toFixed(1)} KB/s`;
    }
    return `${(n / 1024 / 1024).toFixed(1)} MB/s`;
  }

  static bytes(v?: number): string {
    if (v === undefined || v === null || isNaN(Number(v))) {
      return '—';
    }
    const n = Number(v);
    if (n < 1024) {
      return `${n.toFixed(0)} B`;
    }
    if (n < 1024 * 1024) {
      return `${(n / 1024).toFixed(1)} KB`;
    }
    if (n < 1024 * 1024 * 1024) {
      return `${(n / 1024 / 1024).toFixed(1)} MB`;
    }
    return `${(n / 1024 / 1024 / 1024).toFixed(2)} GB`;
  }
}
