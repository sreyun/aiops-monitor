// AIOps Monitor Service Worker
// Cache strategy: app shell (HTML/CSS/JS) cached on install, stale-while-revalidate;
// API calls always go to network (real-time data); non-GET requests bypass cache.

const CACHE = "aiops-v2.3.4";
const SHELL = ["/", "/style.css", "/app.js", "/manifest.json", "/icon.svg"];

// Install: pre-cache the app shell
self.addEventListener("install", e => {
  e.waitUntil(
    caches.open(CACHE).then(c => c.addAll(SHELL)).then(() => self.skipWaiting())
  );
});

// Activate: clean old caches
self.addEventListener("activate", e => {
  e.waitUntil(
    caches.keys().then(keys =>
      Promise.all(keys.filter(k => k !== CACHE).map(k => caches.delete(k)))
    ).then(() => self.clients.claim())
  );
});

// Fetch: stale-while-revalidate for static assets, network-first for API
self.addEventListener("fetch", e => {
  const req = e.request;
  if (req.method !== "GET") return;

  const url = new URL(req.url);
  // API and WebSocket upgrade requests always hit network
  if (url.pathname.startsWith("/api/")) return;

  // Static assets: stale-while-revalidate
  e.respondWith(
    caches.open(CACHE).then(async cache => {
      const cached = await cache.match(req);
      const fetchPromise = fetch(req).then(res => {
        if (res && res.status === 200) cache.put(req, res.clone());
        return res;
      }).catch(() => cached); // offline fallback
      return cached || fetchPromise;
    })
  );
});
