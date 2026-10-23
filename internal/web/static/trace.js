// Follows a running Run: appends each Trace Event the server streams, and
// reloads the panel once the Run has ended to show its final status.
for (const list of document.querySelectorAll("[data-trace-stream]")) {
  const source = new EventSource(list.dataset.traceStream);
  source.addEventListener("trace", (e) => list.insertAdjacentHTML("beforeend", e.data));
  source.addEventListener("end", () => {
    source.close();
    location.reload();
  });
}
