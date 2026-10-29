// Animates a Slot of the Exceptions page opening and closing: on desktop it
// grows from its cell to its whole day line and shrinks back, and later
// days slide rather than jump. Without View Transitions, or with reduced
// motion, it opens at once. Boosted visits run this again: listen once.
if (!window.slotGridAnimated) {
  window.slotGridAnimated = true;
  const reduced = matchMedia("(prefers-reduced-motion: reduce)");
  document.addEventListener("click", (event) => {
    const summary = event.target.closest(".slot-day summary");
    if (!summary || !document.startViewTransition || reduced.matches) {
      return;
    }
    event.preventDefault();
    const details = summary.parentElement;
    // The Slot, the open one of its day it closes, every day, and what
    // follows the days.
    const named = new Set([details.parentElement]);
    for (const open of document.querySelectorAll(".slot-day details[open]")) {
      if (open.name === details.name) {
        named.add(open.parentElement);
      }
    }
    for (const el of document.querySelectorAll(".slot-day, #exceptions-done")) {
      named.add(el);
    }
    [...named].forEach((el, i) => (el.style.viewTransitionName = "slot-" + i));
    document.documentElement.classList.add("slot-toggle");
    const transition = document.startViewTransition(() => {
      details.open = !details.open;
    });
    transition.finished.finally(() => {
      named.forEach((el) => (el.style.viewTransitionName = ""));
      document.documentElement.classList.remove("slot-toggle");
    });
  });
}
