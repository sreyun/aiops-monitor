// AIOps Monitor Service Worker
// Cache: app shell on install, stale-while-revalidate for static, network-only for API.
// Offline: cached shell + navigation fallback to "/" so the UI shows even offline.

const CACHE = "aiops-v2.4.1";
const SHELL = ["/", "/style.css", "/app.js", "/manifest.json", "/icon.svg"];

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

  // Navigation requests: network-first, fallback to cached "/" for offline
  if (req.mode === "navigate") {
    e.respondWith(
      fetch(req).catch(() => caches.match("/"))
    );
    return;
  }

  // Static assets: stale-while-revalidate
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
