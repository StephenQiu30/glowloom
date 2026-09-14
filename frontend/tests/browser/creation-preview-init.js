// Only loaded by the isolated agent-browser fixture session, never by the app.
// Keep mocked XHR same-origin so browser CORS remains enabled during UI tests.
const originalOpen = XMLHttpRequest.prototype.open;
XMLHttpRequest.prototype.open = function (method, url, ...options) {
  const target = new URL(url, window.location.href);
  if (target.origin === "http://127.0.0.1:8686") {
    url = `${window.location.origin}/__browser_fixture${target.pathname}${target.search}`;
  }
  return originalOpen.call(this, method, url, ...options);
};
