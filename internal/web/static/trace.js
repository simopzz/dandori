// Keeps a running Run's newest Trace Events in view: when one streams in
// while the page is scrolled to the bottom, the page scrolls down to show
// it. Scrolled up to read, the page stays put. htmx's show:bottom would
// scroll every time.
new MutationObserver((records) => {
  let added = 0;
  for (const record of records) {
    for (const node of record.addedNodes) {
      if (node instanceof HTMLElement && node.matches(".trace-event")) added += node.offsetHeight;
    }
  }
  if (added === 0) return;
  const page = document.documentElement;
  const below = page.scrollHeight - added - (window.scrollY + window.innerHeight);
  if (below < 64) window.scrollTo({ top: page.scrollHeight });
}).observe(document.documentElement, { childList: true, subtree: true });
