// Validate that every I18N.t("k") and data-i18n*="k" key used in app.js /
// index.html exists in BOTH the zh-CN and en dictionaries.
const fs = require("fs");
function extractObject(source, marker) {
  const idx = source.indexOf(marker);
  if (idx === -1) throw new Error("marker not found: " + marker);
  let i = source.indexOf("{", idx), depth = 0, inStr = false, strCh = "", esc = false;
  for (let j = i; j < source.length; j++) {
    const c = source[j];
    if (inStr) { if (esc) { esc = false; continue; } if (c === "\\") { esc = true; continue; } if (c === strCh) inStr = false; continue; }
    if (c === '"' || c === "'") { inStr = true; strCh = c; continue; }
    if (c === "{") depth++; else if (c === "}") { depth--; if (depth === 0) return eval("(" + source.slice(i, j + 1) + ")"); }
  }
  throw new Error("unbalanced");
}
const zh = extractObject(fs.readFileSync("cmd/server/web/i18n-dashboard.js", "utf8"), "var DICT_ZH =");
const en = extractObject(fs.readFileSync("cmd/server/web/i18n-dashboard.en.js", "utf8"), "window.DICT_EN =");
const app = fs.readFileSync("cmd/server/web/app.js", "utf8");
const html = fs.readFileSync("cmd/server/web/index.html", "utf8");

const used = new Set();
let m;
const reT = /I18N\.t\(\s*"([^"]+)"/g;
while ((m = reT.exec(app))) used.add(m[1]);
const reAttr = /data-i18n(?:-[a-z]+)?="([^"]+)"/g;
while ((m = reAttr.exec(html))) used.add(m[1]);
// also scan app.js template strings for data-i18n usages
while ((m = reAttr.exec(app))) used.add(m[1]);

const missingZh = [], missingEn = [];
for (const k of used) {
  if (!(k in zh)) missingZh.push(k);
  if (!(k in en)) missingEn.push(k);
}
console.log("distinct keys used :", used.size);
console.log("missing in zh-CN   :", missingZh.length);
console.log("missing in en      :", missingEn.length);
if (missingZh.length) console.log("\n--- used but MISSING in zh-CN ---\n  " + missingZh.join("\n  "));
if (missingEn.length) console.log("\n--- used but MISSING in en ---\n  " + missingEn.join("\n  "));
console.log("\nKEYS_OK=" + (missingZh.length === 0 && missingEn.length === 0));
