// Faro, Grafana Frontend Observability, sending what the browser saw: JavaScript errors,
// web vitals and sessions, each with the page's lang, whether the browser translated it,
// and the signed-in User's id; a "page translated" event when it does; and an
// "htmx request" event for each htmx request, carrying the server span's trace id from
// X-Trace-Id. Nothing else: no console, no form contents, no query strings. The Layout
// loads it only when DANDORI_FARO_URL is set (ADR 0020, docs/agents/grafana.md).
(() => {
  const { url, version } = document.currentScript.dataset;
  const { initializeFaro, ErrorsInstrumentation, WebVitalsInstrumentation, SessionInstrumentation } = GrafanaFaroWebSdk;
  const html = document.documentElement;
  // Chrome's translator marks <html> with translated-ltr or translated-rtl, and changes its lang.
  const translated = () => /\btranslated-(ltr|rtl)\b/.test(html.className);
  // A query string can hold what the User typed into a search: every URL leaves without one.
  const withoutQuery = (v) => {
    if (typeof v === "string") return v.replace(/(https?:\/\/[^\s?#"']*)[?#][^\s"')]*/g, "$1");
    if (v && typeof v === "object") for (const k of Object.keys(v)) v[k] = withoutQuery(v[k]);
    return v;
  };
  const faro = initializeFaro({
    url,
    app: { name: "dandori", version },
    instrumentations: [new ErrorsInstrumentation(), new WebVitalsInstrumentation(), new SessionInstrumentation()],
    // Read as each signal is sent, so after a boosted swap they are the new page's: its
    // lang, as the Layout's own script sets it, and its User, from the shell's data-user.
    metas: [() => ({
      page: { url: location.origin + location.pathname, attributes: { lang: html.lang, translated: String(translated()) } },
      user: ((id) => (id ? { id } : undefined))(document.querySelector("[data-user]")?.dataset.user),
    })],
    beforeSend: withoutQuery,
  });
  let wasTranslated = translated();
  new MutationObserver(() => {
    if (translated() && !wasTranslated) faro.api.pushEvent("page translated", { lang: html.lang });
    wasTranslated = translated();
  }).observe(html, { attributes: true, attributeFilter: ["class"] });
  document.addEventListener("htmx:after:request", (e) => {
    const { request, response } = e.detail.ctx;
    faro.api.pushEvent("htmx request", {
      method: request.method,
      path: new URL(response.raw.url).pathname,
      status: String(response.status),
      trace_id: response.headers.get("X-Trace-Id") ?? "",
    }, undefined, { skipDedupe: true });
  });
})();
