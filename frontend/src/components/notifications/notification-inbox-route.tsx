"use client";

import { useEffect, useRef } from "react";
import { useRouter } from "next/navigation";
import { useNotificationCenter } from "./notification-provider";

/** Deep links open the same root panel as the bell, including selected details. */
export function NotificationInboxRoute() {
  const center = useNotificationCenter();
  const router = useRouter();
  const opened = useRef(false);
  const setOpen = center?.setInboxOpen;
  useEffect(() => {
    setOpen?.(true);
  }, [setOpen]);
  useEffect(() => {
    if (center?.inboxOpen) opened.current = true;
    else if (opened.current) router.replace("/", { scroll: false });
  }, [center?.inboxOpen, router]);
  return null;
}
