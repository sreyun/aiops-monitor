// Regenerate cmd/server/web/i18n-dashboard.zh-TW.js from the zh-CN DICT (authoritative)
// using OpenCC Simplified -> Traditional (Taiwan standard, phrase-level "twp"),
// with a MANUAL_TW override map for terms OpenCC handles imperfectly.
//
// Run:  NODE_PATH=<managed>/node_modules node scripts/build_i18n_tw.js
const fs = require("fs");
const OPENCC_PATH = process.env.OPENCC_PATH || "opencc-js";
const OpenCC = require(OPENCC_PATH);

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

// Taiwan standard with phrase (vocabulary) conversion: 软件->軟體, 网络->網路, etc.
const convert = OpenCC.Converter({ from: "cn", to: "twp" });

// Post-conversion fixes for terms where OpenCC's default mapping is not ideal
// for this product's UI wording. Applied as whole-string replacements after twp.
const MANUAL_TW = {
  // full-value overrides keyed by zh-CN source value (exact match)
};

// Term-level touch-ups applied to every converted string (safe, unambiguous).
const TERM_FIX = [
  // OpenCC twp already covers most; keep this list conservative.
  ["登入密碼", "登入密碼"],
];

const keys = Object.keys(zh);
const out = {};
for (const k of keys) {
  const src = zh[k];
  if (MANUAL_TW[src] != null) { out[k] = MANUAL_TW[src]; continue; }
  let v = convert(src);
  for (const [a, b] of TERM_FIX) v = v.split(a).join(b);
  out[k] = v;
}

let body = "";
for (const k of keys) body += '  "' + k + '": ' + JSON.stringify(out[k]) + ",\n";
body = body.replace(/,\n$/, "\n");

const header =
`/**
 * AIOps Monitor Dashboard — Traditional Chinese (Taiwan) dictionary (window.DICT_TW)
 * Auto-generated from i18n-dashboard.js (zh-CN DICT) via OpenCC (cn -> twp).
 * Regenerate with scripts/build_i18n_tw.js. Key-parity enforced by check_i18n_parity.js.
 */
window.DICT_TW = {
` + body + `};
`;

fs.writeFileSync("cmd/server/web/i18n-dashboard.zh-TW.js", header, "utf8");
console.log("zh-TW total keys:", keys.length);
// sample a few IT terms to eyeball quality
["ui.overview", "nav.hosts", "config.title", "install.title", "term.uploading",
 "settings.smtp_tls", "ui.theme_toggle", "ui.language"].forEach(k => {
  if (zh[k] != null) console.log("  " + k + ": " + zh[k] + "  ->  " + out[k]);
});
