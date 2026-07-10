// Regenerate cmd/server/web/i18n-dashboard.en.js from the zh-CN DICT (authoritative)
// merged with the existing English translations, dropping invented keys and fixing
// the ui.by_category -> filter.by_category rename.
const fs = require("fs");

function extractObject(source, marker) {
  const idx = source.indexOf(marker);
  if (idx === -1) throw new Error("marker not found: " + marker);
  let i = source.indexOf("{", idx);
  if (i === -1) throw new Error("no opening brace after marker");
  let depth = 0, inStr = false, strCh = "", escape = false;
  for (let j = i; j < source.length; j++) {
    const c = source[j];
    if (inStr) {
      if (escape) { escape = false; continue; }
      if (c === "\\") { escape = true; continue; }
      if (c === strCh) { inStr = false; }
      continue;
    }
    if (c === '"' || c === "'") { inStr = true; strCh = c; continue; }
    if (c === "{") depth++;
    else if (c === "}") {
      depth--;
      if (depth === 0) return eval("(" + source.slice(i, j + 1) + ")");
    }
  }
  throw new Error("unbalanced braces");
}

const zh = extractObject(fs.readFileSync("cmd/server/web/i18n-dashboard.js", "utf8"), "var DICT_ZH =");
const enRaw = extractObject(fs.readFileSync("cmd/server/web/i18n-dashboard.en.js", "utf8"), "window.DICT_EN =");

const INVENTED = ["notify.type_diskio", "notify.type_iops", "notify.type_proc",
  "install.scenario_label", "install.principle_label", "settings.smtp_tls_label"];

const MANUAL_EN = {
  "filter.by_category": "Filter by category",
  "section.recent_alerts": "Recent Alerts",
  "section.recent_activity": "Recent Activity",
  "section.sample_points": "Sample points",
  "section.granularity": "Granularity",
  "section.stats_health": "Statistics & Health",
  "section.online_rate": "Online rate",
  "section.health_status": "System status",
  "section.health_ok": "Healthy",
  "section.health_error": "Abnormal",
  "section.total_alerts": "Active alerts",
  "section.unprocessed_alerts": "Unprocessed",
  "onboard.install_now": "Install your first Agent now",
  // New keys added for the multi-language rollout (audit)
  "ui.theme_toggle": "Theme",
  "ui.language": "Language",
  "filter.type_diskio": "Disk IO",
  "filter.type_iops": "Disk IOPS",
  "filter.type_proc": "Processes",
  "term.upload_failed": "Upload failed",
  "ui.login": "Log In",
  "ui.add_check": "+ Add Check",
  "ui.new_playbook": "+ New Playbook",
  "term.file_too_large": "File exceeds 100MB limit, use another method",
  // Keys referenced by HTML/JS but missing from the original dict (added)
  "empty.no_trend_data": "No trend data",
  "term_auth.password_too_short": "Password must be at least 8 characters",
  "term.downloading": "Downloading",
  "term.download_done": "Download complete",
  "auth.must_change_password": "Please change your password to continue",
  "config.title": "Alert Settings",
  "section.resource_heatmap": "Resource TOP10 Ranking",
  "section.realtime_top10": "Realtime TOP10",
  "profile.account_info": "Account Info",
  "ui.edit_forward": "Edit Forward",
  // 个人信息四 Tab 标签
  "profile.tab_info": "Profile",
  "profile.tab_password": "Password",
  "profile.tab_mfa": "MFA",
  "profile.tab_users": "Users"
};

// Normalize: drop invented, rename ui.by_category -> filter.by_category
const en = {};
for (const k of Object.keys(enRaw)) {
  if (INVENTED.includes(k)) continue;
  if (k === "ui.by_category") { en["filter.by_category"] = enRaw[k]; continue; }
  en[k] = enRaw[k];
}

const keys = Object.keys(zh);
const out = {};
const fallback = [];
for (const k of keys) {
  if (en[k] && String(en[k]).trim() !== "") out[k] = en[k];
  else if (MANUAL_EN[k]) out[k] = MANUAL_EN[k];
  else { out[k] = zh[k]; fallback.push(k); }
}

let body = "";
for (const k of keys) body += '  "' + k + '": ' + JSON.stringify(out[k]) + ",\n";
body = body.replace(/,\n$/, "\n");

const header =
`/**
 * AIOps Monitor Dashboard — English dictionary (window.DICT_EN)
 * Auto-generated mirror of i18n-dashboard.js (zh-CN DICT).
 * Key-parity with zh-CN is enforced by scripts/check_i18n_parity.js.
 */
window.DICT_EN = {
` + body + `};
`;

fs.writeFileSync("cmd/server/web/i18n-dashboard.en.js", header, "utf8");
console.log("en total keys:", keys.length);
console.log("fallback-to-zh:", fallback.length);
if (fallback.length) console.log(fallback);
