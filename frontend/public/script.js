(() => {
  const s = document.currentScript;
  if (!s || !s.src) return;
  const domain = s.getAttribute("data-domain");
  let endpoint, retryEndpoint;
  try {
    endpoint = new URL(s.getAttribute("data-api") || "/api/event", s.src).href;
    // A custom data-api (a proxy on the site itself) is used for retries too, since /e may not exist there.
    retryEndpoint = new URL(s.getAttribute("data-api") || "/e", s.src).href;
  } catch (e) { return; }
  const installed = window.__litestatsTrackers || (window.__litestatsTrackers = Object.create(null));
  const trackerKey = JSON.stringify([endpoint, domain]);
  if (installed[trackerKey]) return;
  installed[trackerKey] = true;
  const trackOutbound = s.getAttribute("data-outbound") !== "false";
  const trackFiles = s.getAttribute("data-file-downloads") !== "false";
  const visitor = (function () {
    const key = "litestats:visitor-id";
    try {
      const saved = localStorage.getItem(key);
      if (saved && /^[A-Za-z0-9_-]{8,64}$/.test(saved)) return saved;
      const id = window.crypto && typeof window.crypto.randomUUID === "function" ? window.crypto.randomUUID() : "v" + Date.now().toString(36) + Math.random().toString(36).slice(2, 12);
      localStorage.setItem(key, id);
      return id;
    } catch (e) { return ""; }
  })();
  let pageKey = "";
  const entryReferrer = document.referrer;
  function beacon(data, url) {
    try { return !!navigator.sendBeacon && navigator.sendBeacon(url, new Blob([JSON.stringify(data)], { type: "text/plain;charset=UTF-8" })); } catch (e) { return false; }
  }
  // When the request to /api/event fails before any answer (a content blocker or a broken network refused it), the
  // event goes again to the second address as an image, and if that is refused too, as a Beacon. Each retry says how it
  // travelled ("x"), and the server keeps only the first copy of an event id ("i"), so nothing is counted twice.
  function retry(data) {
    const viaBeacon = function () { beacon(Object.assign({}, data, { x: "beacon" }), retryEndpoint); };
    try {
      const copy = Object.assign({}, data, { x: "img" });
      let query = JSON.stringify(copy);
      if (query.length > 6000) { copy.t = ""; copy.r = String(copy.r || "").slice(0, 500); copy.u = String(copy.u).slice(0, 2000); query = JSON.stringify(copy); }
      const img = new Image();
      img.onerror = viaBeacon;
      img.src = retryEndpoint + (retryEndpoint.indexOf("?") < 0 ? "?" : "&") + "p=" + encodeURIComponent(query);
    } catch (e) { viaBeacon(); }
  }
  function deliver(data, attempt) {
    const body = JSON.stringify(data);
    if (typeof fetch !== "function") { if (!beacon(data, endpoint)) retry(data); return; }
    try {
      fetch(endpoint, { method: "POST", body, headers: { "Content-Type": "text/plain;charset=UTF-8" }, credentials: "omit", keepalive: true }).then(function (response) {
        // Only this explicit rejection guarantees that the event was not queued.
        // Generic 5xx responses are ambiguous and are not repeated.
        if (response.status !== 503 || attempt >= 2) return;
        return response.json().then(function (result) {
          if (result && result.error === "event_buffer_full") window.setTimeout(function () { deliver(data, attempt + 1); }, (attempt + 1) * 1000);
        });
      }, function () { retry(data); }).catch(function () {});
    } catch (e) { if (!beacon(data, endpoint)) retry(data); }
  }
  function send(payload) {
    if (document.prerendering) return;
    try {
      const data = Object.assign({
        n: "pageview", d: domain, u: location.href, r: entryReferrer, w: innerWidth,
        v: visitor || undefined, t: document.title || "", l: navigator.language || "",
        s: (screen.width && screen.height) ? (screen.width + "x" + screen.height) : "",
        i: Date.now().toString(36) + Math.random().toString(36).slice(2, 10)
      }, payload || {});
      deliver(data, 0);
    } catch (e) {}
  }
  function pageview(force) {
    if (document.prerendering) return;
    const url = location.href;
    const key = url.split("#")[0];
    if (!force && key === pageKey) return;
    pageKey = key;
    send({ u: url });
  }
  function start() {
    pageview();
    if (document.documentElement && /404|not found/i.test(document.title)) send({ n: "404", p: { path: location.pathname } });
  }
  if (document.prerendering) document.addEventListener("prerenderingchange", start, { once: true });
  else start();
  ["pushState", "replaceState"].forEach(function (name) {
    const original = history[name];
    history[name] = function () { const result = original.apply(this, arguments); pageview(); return result; };
  });
  window.addEventListener("popstate", function () { pageview(); });
  window.addEventListener("pageshow", function (ev) { if (ev.persisted) pageview(true); });
  if (trackOutbound || trackFiles) document.addEventListener("click", function (ev) {
    const a = ev.target && ev.target.closest ? ev.target.closest("a") : null;
    if (!a || !a.href) return;
    let url;
    try { url = new URL(a.href); } catch (e) { return; }
    if (trackOutbound && url.host && url.host !== location.host) send({ n: "Outbound Link: Click", p: { url: url.href } });
    if (trackFiles && /\.(pdf|zip|csv|docx?|xlsx?|pptx?|rar|7z|mp4|mp3)(\?|#|$)/i.test(url.pathname)) send({ n: "File Download", p: { url: url.href } });
  }, true);
  window.plost = window.litestats = { track: function (name, props) { send({ n: name, p: props || {} }); } };
})();
