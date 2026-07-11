// 主题预置：在页面渲染前同步应用 localStorage 中保存的明/暗主题，避免首屏闪烁。
// 由 index.html 的内联脚本外置而来 —— 目的是让 CSP 得以移除 script-src 'unsafe-inline'。
// 该文件必须在 <head> 中以同步（非 defer/async）方式引入，才能在 body 渲染前生效。
(function () {
  try {
    var t = localStorage.getItem("aiops_theme") || "dark";
    document.documentElement.setAttribute("data-theme", t);
  } catch (e) {
    document.documentElement.setAttribute("data-theme", "dark");
  }
})();
