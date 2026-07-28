// Safe repo-wide rename of the display phrase "AIOps" -> "AIOps".
// The phrase with a space is always a human-readable product name, never an
// identifier/path (those use aiops-monitor / AIOpsMonitor / aiops_monitor).
const fs = require("fs");
const path = require("path");

const ROOT = "D:/个人专用/工具开发/aiops-monitor";
const SKIP_DIRS = new Set([".git", "node_modules", "vendor", ".workbuddy", "build", ".gradle"]);
const SKIP_FILES = new Set(["LICENSE"]); // copyright holder must stay intact
// binary / non-text extensions we never touch
const BINARY_EXT = new Set([
  ".png", ".jpg", ".jpeg", ".gif", ".ico", ".svg", ".webp", ".bmp",
  ".woff", ".woff2", ".ttf", ".eot", ".otf",
  ".exe", ".dll", ".so", ".dylib", ".apk", ".jar", ".aar", ".class",
  ".zip", ".tar", ".gz", ".7z", ".rar",
  ".pdf", ".mp4", ".webm", ".mov", ".mp3", ".wav",
  ".bin", ".dat", ".db", ".sqlite", ".key", ".pem", ".crt",
]);

const RE = /AIOps/g;
let changed = 0, scanned = 0, hits = 0;

function walk(dir) {
  let entries;
  try { entries = fs.readdirSync(dir, { withFileTypes: true }); }
  catch (e) { return; }
  for (const ent of entries) {
    const full = path.join(dir, ent.name);
    if (ent.isDirectory()) {
      if (SKIP_DIRS.has(ent.name)) continue;
      walk(full);
    } else if (ent.isFile()) {
      if (SKIP_FILES.has(ent.name)) continue;
      const ext = path.extname(ent.name).toLowerCase();
      if (BINARY_EXT.has(ext)) continue;
      scanned++;
      let buf;
      try { buf = fs.readFileSync(full); } catch (e) { return; }
      // only process if valid UTF-8-ish text
      const text = buf.toString("utf8");
      if (text.includes("AIOps")) {
        const newText = text.replace(RE, "AIOps");
        const before = (text.match(RE) || []).length;
        fs.writeFileSync(full, newText, "utf8");
        hits += before;
        changed++;
        console.log(`[${before}] ${path.relative(ROOT, full)}`);
      }
    }
  }
}

walk(ROOT);
console.log(`\nScanned ${scanned} text files. Changed ${changed} files. Total replacements ${hits}.`);
