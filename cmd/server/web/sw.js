// AIOps Monitor Service Worker
// Cache: app shell on install, stale-while-revalidate for static, network-only for API.
// Offline: cached shell + navigation fallback to "/" so the UI shows even offline.

const CACHE = "AIOps-v5.6.1";
const SHELL = ["/", "/style.css", "/theme-init.js", "/app.js", "/manifest.json", "/icon.svg"];

self.addEventListener("install", e => {
  e.waitUntil(
    caches.open(CACHE).then(c => c.addAll(SHELL)).then(() => self.skipWaiting())
  );
});

self.addEventListener("activate", e => {
  e.waitUntil(
    caches.keys().then(keys =>
      Promise.all(keys.filter(k => k !== CACHE).map(k => caches.delete(k)))
    ).then(() => self.clients.claim())
  );
});

self.addEventListener("fetch", e => {
  const req = e.request;
  if (req.method !== "GET") return;

  const url = new URL(req.url);
  if (url.pathname.startsWith("/api/")) return;

  // Navigation + code assets (index / app.js / style.css / any .js/.css):
  // NETWORK-FIRST so a new deploy applies on the very next load when online;
  // fall back to cache only when offline. (The previous stale-while-revalidate
  // strategy left users one reload behind after every frontend update.)
  const p = url.pathname;
  if (req.mode === "navigate" || p === "/" || p.endsWith(".js") || p.endsWith(".css")) {
    e.respondWith(
      fetch(req).then(res => {
        if (res && res.status === 200) {
          const clone = res.clone();
          caches.open(CACHE).then(c => c.put(req, clone));
        }
        return res;
      }).catch(() => caches.match(req).then(c => c || caches.match("/")))
    );
    return;
  }

  // Other static assets (icons / manifest / fonts): stale-while-revalidate.
  e.respondWith(
    caches.open(CACHE).then(async cache => {
      const cached = await cache.match(req);
      const fetchPromise = fetch(req).then(res => {
        if (res && res.status === 200) cache.put(req, res.clone());
        return res;
      }).catch(() => cached);
      return cached || fetchPromise;
    })
  );
});
