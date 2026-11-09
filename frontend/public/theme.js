(() => {
  try {
    const theme = localStorage.getItem('plost:theme') || 'system';
    document.documentElement.dataset.theme = theme === 'dark' || (theme === 'system' && matchMedia('(prefers-color-scheme: dark)').matches) ? 'dark' : 'light';
    document.documentElement.dataset.accent = localStorage.getItem('litestats:accent') || localStorage.getItem('plost:accent') || 'purple';
    document.documentElement.dataset.siteLayout = localStorage.getItem('litestats:site-layout') === 'grid' ? 'grid' : 'compact';
    // Same order and detection as frontend/src/lib/i18n.tsx (LOCALES / browserLocale), so the document language is
    // right before React loads. A saved choice wins; otherwise the first browser language Plost speaks.
    const locales = ["zh-CN", "zh-TW", "en", "ru", "vi", "id", "pt-BR", "fa", "ar", "ja", "ko", "th", "es", "fr", "de", "tr", "ur", "it", "pl", "nl"];
    const rtl = { ar: 1, fa: 1, ur: 1 };
    const stored = localStorage.getItem("plost:locale");
    let locale = locales.indexOf(stored) >= 0 ? stored : "";
    if (!locale) {
      const languages = navigator.languages || [navigator.language || ""];
      for (let i = 0; i < languages.length && !locale; i++) {
        const lower = String(languages[i] || "").toLowerCase();
        if (lower.startsWith("zh")) locale = /^zh-(tw|hk|mo|hant)/.test(lower) ? "zh-TW" : "zh-CN";
        else if (lower.startsWith("pt")) locale = "pt-BR";
        else if (locales.indexOf(lower.split("-")[0]) >= 0) locale = lower.split("-")[0];
      }
      locale = locale || "zh-CN";
    }
    document.documentElement.lang = locale;
    document.documentElement.dir = rtl[locale] ? "rtl" : "ltr";
  } catch (_) {}
})();
