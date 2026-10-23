import { onlineProjections } from "@/components/canvas/runtime/lib/online-projections";
import { browserConnectionCsrf } from "@/components/canvas/runtime/lib/request";

type AccountEventCallbacks = {
  ready: () => void;
  changed: () => void;
  notificationChanged?: () => void;
};

export const NOTIFICATION_REFRESH_EVENT = "glowloom:notification:changed";

/** Each visible document holds one stream; reconnects begin with server revalidation. */
export function subscribeAccountEvents(callbacks: AccountEventCallbacks) {
  onlineProjections.reset();
  let stopped = false;
  let source: WebSocket | undefined;
  let retry: ReturnType<typeof setTimeout> | undefined;
  let watchdog: ReturnType<typeof setTimeout> | undefined;
  let delay = 1000;

  function active() {
    return window.document.visibilityState !== "hidden";
  }

  function close() {
    const previous = source;
    source = undefined;
    previous?.close();
    clearTimeout(watchdog);
    clearTimeout(retry);
  }

  function disconnect() {
    close();
    if (stopped || !active()) return;
    retry = setTimeout(connect, delay);
    delay = Math.min(delay * 2, 5000);
  }

  function touch() {
    clearTimeout(watchdog);
    watchdog = setTimeout(disconnect, 12000);
  }

  function connect() {
    if (stopped || source || !active()) return;
    open();
  }

  function open() {
    try {
      const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
      const current = new window.WebSocket(
        `${protocol}//${window.location.host}/api/account/connection`,
      );
      source = current;
      current.onopen = () => {
        if (stopped || source !== current) return;
        current.send(
          JSON.stringify({
            type: "OPEN",
            protocolVersion: 1,
            csrfToken: browserConnectionCsrf(),
          }),
        );
      };
      current.onmessage = (message) => {
        if (stopped || source !== current) return;
        try {
          const event: unknown = JSON.parse(String(message.data));
          if (!event || typeof event !== "object" || !("type" in event))
            return disconnect();
          switch (event.type) {
            case "ready":
              if (
                !("streamId" in event) ||
                typeof event.streamId !== "string" ||
                !("protocolVersion" in event) ||
                event.protocolVersion !== 1
              )
                return disconnect();
              onlineProjections.begin(event.streamId);
              touch();
              delay = 1000;
              callbacks.ready();
              break;
            case "projection.changed":
            case "sync.required":
              touch();
              if (!onlineProjections.accept(event)) disconnect();
              break;
            case "changed":
              touch();
              callbacks.changed();
              break;
            case "notification.changed":
              touch();
              callbacks.notificationChanged?.();
              break;
            case "heartbeat":
              touch();
              break;
            case "unavailable":
              disconnect();
              break;
            default:
              disconnect();
          }
        } catch {
          disconnect();
        }
      };
      current.onclose = (event) => {
        if (stopped || source !== current) return;
        if (event.code === 1008) {
          close();
          callbacks.changed();
          disconnect();
        } else disconnect();
      };
      current.onerror = () => {
        if (!stopped && source === current) disconnect();
      };
      touch();
    } catch {
      disconnect();
    }
  }

  function visibilityChanged() {
    if (!active()) close();
    else connect();
  }

  window.document.addEventListener("visibilitychange", visibilityChanged);
  window.addEventListener("focus", visibilityChanged);
  connect();
  return () => {
    stopped = true;
    onlineProjections.reset();
    window.document.removeEventListener("visibilitychange", visibilityChanged);
    window.removeEventListener("focus", visibilityChanged);
    close();
  };
}
