const fs = require('fs');

// 读取 HTML 文件，提取所有 ID
const html = fs.readFileSync('cmd/server/web/index.html', 'utf8');
const htmlIds = new Set();
const htmlIdRegex = /id="([^"]+)"/g;
let match;
while ((match = htmlIdRegex.exec(html)) !== null) {
  htmlIds.add(match[1]);
}

console.log('HTML 中定义的 IDs:');
console.log(Array.from(htmlIds).sort().join(', '));
console.log('\n');

// 读取 JS 文件，提取所有 $() 调用
const js = fs.readFileSync('cmd/server/web/app.js', 'utf8');
const jsIds = new Set();
const jsIdRegex = /\$\(["']([^"']+)["']\)/g;
while ((match = jsIdRegex.exec(js)) !== null) {
  jsIds.add(match[1]);
}

console.log('JS 中引用的 IDs:');
console.log(Array.from(jsIds).sort().join(', '));
console.log('\n');

// 检查是否有 JS 引用但 HTML 中不存在的 ID
const missing = [];
jsIds.forEach(id => {
  if (!htmlIds.has(id)) {
    missing.push(id);
  }
});

if (missing.length > 0) {
  console.log('❌ 缺失的 IDs (JS 引用但 HTML 中不存在):');
  console.log(missing.join(', '));
} else {
  console.log('✅ 所有 JS 引用的 ID 都在 HTML 中存在');
}
