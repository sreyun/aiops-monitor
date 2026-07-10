// Verify key-parity across all dashboard locale dictionaries (zh-CN / zh-TW / en).
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

const fs = require("fs");
const zh = extractObject(fs.readFileSync("cmd/server/web/i18n-dashboard.js", "utf8"), "var DICT_ZH =");
const en = extractObject(fs.readFileSync("cmd/server/web/i18n-dashboard.en.js", "utf8"), "window.DICT_EN =");
const tw = extractObject(fs.readFileSync("cmd/server/web/i18n-dashboard.zh-TW.js", "utf8"), "window.DICT_TW =");

const zhKeys = Object.keys(zh);
let ok = true;

function compare(name, dict) {
  const keys = Object.keys(dict);
  const missing = zhKeys.filter(k => !(k in dict));
  const extra = keys.filter(k => !(k in zh));
  const empty = keys.filter(k => dict[k] === "" || dict[k] == null);
  console.log(`\n[${name}] keys=${keys.length}  missing=${missing.length}  extra=${extra.length}  empty=${empty.length}`);
  if (missing.length) { console.log("  MISSING:"); missing.forEach(k => console.log("    " + k + " => " + zh[k])); }
  if (extra.length)   { console.log("  EXTRA:");   extra.forEach(k => console.log("    " + k + " => " + dict[k])); }
  if (empty.length)   { console.log("  EMPTY:");   empty.forEach(k => console.log("    " + k)); }
  if (missing.length || extra.length || empty.length) ok = false;
}

console.log("zh-CN key count :", zhKeys.length);
compare("en", en);
compare("zh-TW", tw);

console.log("\nPARITY_OK=" + ok);
if (!ok) process.exit(1);
