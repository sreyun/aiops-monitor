/* ============================================================
   AIOps Monitor · 营销网站交互逻辑
   ============================================================ */
"use strict";
(function(){

/* 导航栏滚动效果 */
var navbar = document.querySelector(".navbar");
if (navbar) {
  window.addEventListener("scroll", function() {
    navbar.classList.toggle("scrolled", window.scrollY > 20);
  });
}

/* 移动端菜单 */
var toggle = document.querySelector(".nav-toggle");
var links = document.querySelector(".nav-links");
if (toggle && links) {
  toggle.addEventListener("click", function() {
    links.classList.toggle("open");
  });
}

/* 滚动渐入动画 */
var observer = new IntersectionObserver(function(entries) {
  entries.forEach(function(e) {
    if (e.isIntersecting) {
      e.target.classList.add("visible");
      observer.unobserve(e.target);
    }
  });
}, { threshold: 0.1, rootMargin: "0px 0px -60px 0px" });

document.querySelectorAll(".reveal").forEach(function(el) {
  observer.observe(el);
});

/* 错开动画延迟 */
document.querySelectorAll(".pain-card, .feature-card").forEach(function(el, i) {
  el.style.transitionDelay = (i % 4) * 80 + "ms";
});

/* 数字滚动动画 */
function animateNumber(el, target, suffix) {
  var start = 0;
  var duration = 1500;
  var startTime = null;
  suffix = suffix || "";
  function step(ts) {
    if (!startTime) startTime = ts;
    var progress = Math.min((ts - startTime) / duration, 1);
    var eased = 1 - Math.pow(1 - progress, 3);
    el.textContent = Math.floor(eased * target) + suffix;
    if (progress < 1) requestAnimationFrame(step);
  }
  requestAnimationFrame(step);
}

var numObserver = new IntersectionObserver(function(entries) {
  entries.forEach(function(e) {
    if (e.isIntersecting && e.target.dataset.count) {
      animateNumber(e.target, parseInt(e.target.dataset.count), e.target.dataset.suffix || "");
      numObserver.unobserve(e.target);
    }
  });
}, { threshold: 0.5 });

document.querySelectorAll("[data-count]").forEach(function(el) {
  numObserver.observe(el);
});

/* 平滑滚动到锚点 */
document.querySelectorAll('a[href^="#"]').forEach(function(a) {
  a.addEventListener("click", function(e) {
    var href = this.getAttribute("href");
    if (href.length > 1) {
      var target = document.querySelector(href);
      if (target) {
        e.preventDefault();
        target.scrollIntoView({ behavior: "smooth", block: "start" });
      }
    }
  });
});

})();
