// Dandori is a local app whose data lives in the database, so the service
// worker only makes the app installable: every request goes to the network.
self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", (event) => event.waitUntil(self.clients.claim()));
self.addEventListener("fetch", () => {});
