/* openLitfass – Service Worker
   App-Shell (HTML/CSS/JS/Logo) cache-first, Daten (data/*.json)
   network-first mit Fallback auf die zuletzt geladene Version: die Seite
   startet offline und zeigt dann die letzten bekannten Termine.
   __BUILD_ID__ ersetzt build-site bei jedem Build, damit jedes Deployment
   einen neuen Cache bekommt und alte Caches gelöscht werden. */
const CACHE = "openlitfass-__BUILD_ID__";
const SHELL = [
  "./",
  "index.html",
  "css/tokens.css",
  "css/theme.css",
  "css/style.css",
  "js/settings.js",
  "js/map.js",
  "js/image.js",
  "js/app.js",
  "favicon.svg",
  "manifest.webmanifest"
];

self.addEventListener("install", (event) => {
  event.waitUntil(caches.open(CACHE).then((c) => c.addAll(SHELL)).then(() => self.skipWaiting()));
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches.keys()
      .then((keys) => Promise.all(keys.filter((k) => k.startsWith("openlitfass-") && k !== CACHE).map((k) => caches.delete(k))))
      .then(() => self.clients.claim())
  );
});

self.addEventListener("fetch", (event) => {
  const req = event.request;
  if (req.method !== "GET") return;
  const url = new URL(req.url);
  if (url.origin !== self.location.origin) return;

  // Markdown pages (seite.html?page=…, pages/*.md and their renderer) are
  // not in the shell: like the data they are fetched from the network and
  // kept, so a page visited once also opens offline.
  const markdownPage = url.pathname.endsWith("/seite.html") ||
    url.pathname.includes("/pages/") || url.pathname.includes("/vendor/markdown-pages/");
  if (url.pathname.includes("/data/") || markdownPage) {
    event.respondWith(
      fetch(req)
        .then((res) => {
          const copy = res.clone();
          caches.open(CACHE).then((c) => c.put(req, copy));
          return res;
        })
        .catch(() => caches.match(req))
    );
    return;
  }

  event.respondWith(caches.match(req).then((hit) => hit || fetch(req)));
});
