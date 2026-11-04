(() => {
  const s = document.currentScript;
  if (!s || !s.src) return;
  if (navigator.webdriver) {
    if (!window.plost) window.plost = window.litestats = { track: function () {} };
    return;
  }
  const domain = s.getAttribute("data-domain");
  const customApi = s.getAttribute("data-api");
  let endpoint, retryEndpoint;
  try {
    endpoint = new URL(customApi || "/api/send", s.src).href;
    retryEndpoint = new URL(customApi || "/e", s.src).href;
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
  function eventBody(data) {
    const payload = { website: data.d, hostname: location.hostname, screen: data.s, language: data.l, title: data.t, url: data.u, referrer: data.r, v: data.v, i: data.i, w: data.w };
    if (data.n !== "pageview") { payload.name = data.n; if (data.p) payload.data = data.p; }
    return JSON.stringify({ type: "event", payload: payload });
  }
  function deliver(data, attempt) {
    const body = customApi ? JSON.stringify(data) : eventBody(data);
    const plain = function () { if (!beacon(Object.assign({}, data, { x: "beacon" }), retryEndpoint)) retry(data); };
    if (typeof fetch !== "function") { plain(); return; }
    try {
      fetch(endpoint, { method: "POST", body, headers: { "Content-Type": customApi ? "text/plain;charset=UTF-8" : "application/json" }, credentials: "omit", keepalive: true }).then(function (response) {
        if (response.status !== 503 || attempt >= 2) return;
        return response.json().then(function (result) {
          if (result && result.error === "event_buffer_full") window.setTimeout(function () { deliver(data, attempt + 1); }, (attempt + 1) * 1000);
        });
      }, function () { retry(data); }).catch(function () {});
    } catch (e) { plain(); }
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
  let ready = false;
  function pageview(force) {
    if (!ready || document.prerendering) return;
    const url = location.href;
    const key = url.split("#")[0];
    if (!force && key === pageKey) return;
    pageKey = key;
    send({ u: url });
  }
  function start() {
    if (ready || document.readyState !== "complete") return;
    ready = true;
    pageview();
    if (document.documentElement && /404|not found/i.test(document.title)) send({ n: "404", p: { path: location.pathname } });
  }
  function begin() {
    if (document.readyState === "complete") start();
    else document.addEventListener("readystatechange", start);
  }
  if (document.prerendering) document.addEventListener("prerenderingchange", begin, { once: true });
  else begin();
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
