"use client";
import { useSyncExternalStore } from "react";
import type { CanvasYjsLiveSession } from "./yjs-live-session";

export function YjsPresence({
  client,
  cursors = false,
  connected = true,
}: {
  client: CanvasYjsLiveSession;
  cursors?: boolean;
  connected?: boolean;
}) {
  const peers = useSyncExternalStore(
    client.subscribePresence,
    client.getPresence,
    client.getPresence,
  );
  if (!cursors)
    return (
      <span aria-label="在线协作页面" className="text-xs text-muted-foreground">
        {connected ? `${peers.length + 1} 个页面在线` : "协作连接已断开"}
      </span>
    );
  return (
    <div
      className="pointer-events-none absolute inset-0 z-[70]"
      aria-label="协作者光标"
    >
      {peers
        .filter((peer) => peer.cursor)
        .map((peer) => (
          <div
            key={peer.clientId}
            className="absolute w-max text-xs"
            style={{
              transform: `translate(${peer.cursor!.x}px, ${peer.cursor!.y}px)`,
              color: `hsl(${parseInt(peer.actorId.slice(0, 6), 16) % 360} 70% 45%)`,
            }}
          >
            <svg
              width="16"
              height="20"
              viewBox="0 0 16 20"
              fill="currentColor"
              aria-hidden="true"
            >
              <path d="M1 1v15l4-4 3 7 3-2-3-7h6z" stroke="white" />
            </svg>
            <span className="rounded bg-background/90 px-1">
              协作者 {peer.actorId.slice(0, 4)}
            </span>
          </div>
        ))}
    </div>
  );
}
