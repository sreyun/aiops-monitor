// 主题预置：在页面渲染前同步应用 localStorage 中保存的明/暗主题，避免首屏闪烁。
// 由 index.html 的内联脚本外置而来 —— 目的是让 CSP 得以移除 script-src 'unsafe-inline'。
// 该文件必须在 <head> 中以同步（非 defer/async）方式引入，才能在 body 渲染前生效。
// 默认浅色：首次访问无本地偏好时使用 light（不再默认 dark）。
(function () {
  var LIGHT = "#f5f7fa", DARK = "#0a0d13";
  try {
    // iframe 内嵌：提前加 class，避免侧栏/顶栏闪一下
    if (/(?:^|[?&])embed=1(?:&|$)/.test(location.search)) {
      document.documentElement.classList.add("embed-mode");
    }
    var t = localStorage.getItem("aiops_theme") || "light";
    if (t !== "light" && t !== "dark") t = "light";
    document.documentElement.setAttribute("data-theme", t);
    document.documentElement.style.colorScheme = t;
    var meta = document.querySelector('meta[name="theme-color"][data-aiops-theme]');
    if (meta) meta.setAttribute("content", t === "light" ? LIGHT : DARK);
  } catch (e) {
    document.documentElement.setAttribute("data-theme", "light");
    document.documentElement.style.colorScheme = "light";
  }
})();
