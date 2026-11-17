"use client";

import { useEffect, useEffectEvent, useState } from "react";
import { validateNotificationDisplay } from "@/api/notifications";
import { TimedAnnouncementDialog } from "./timed-announcement";
import type { DisplayAnnouncement } from "./contracts";

function canInterrupt() {
  const focused = document.activeElement;
  return (
    !document.hidden &&
    !document.querySelector(
      '[role="dialog"], [role="alertdialog"], [data-sonner-toast]',
    ) &&
    !(
      focused instanceof Element &&
      focused.closest(
        'input, textarea, [contenteditable="true"], [role="textbox"]',
      )
    )
  );
}

/** Claim once, then check the published revision immediately before showing it. */
export function AutomaticAnnouncement({
  announcement,
  onShown,
  onClose,
}: {
  announcement: DisplayAnnouncement;
  onShown: () => Promise<boolean>;
  onClose: () => void;
}) {
  const [open, setOpen] = useState(false);
  const shown = useEffectEvent(onShown);
  const close = useEffectEvent(onClose);
  useEffect(() => {
    let timer: ReturnType<typeof setTimeout>;
    let disposed = false;
    let claimed = false;
    let opened = false;
    async function check() {
      if (
        !announcement.endless &&
        announcement.ends_at &&
        new Date(announcement.ends_at).getTime() <= Date.now()
      ) {
        close();
        return;
      }
      if (!opened && !canInterrupt()) {
        timer = setTimeout(check, 500);
        return;
      }
      try {
        if (!claimed) claimed = await shown();
        if (disposed) return;
        if (!claimed) {
          close();
          return;
        }
        const result = await validateNotificationDisplay({
          id: announcement.id,
          revision: announcement.revision,
        });
        if (disposed) return;
        if (!result.valid) {
          close();
          return;
        }
        // Neither the claim nor the validity request may steal focus after typing starts.
        if (!opened && canInterrupt()) {
          opened = true;
          setOpen(true);
        }
        timer = setTimeout(check, opened ? 5000 : 500);
      } catch {
        if (!disposed) timer = setTimeout(check, 5000);
      }
    }
    timer = setTimeout(check, 0);
    return () => {
      disposed = true;
      clearTimeout(timer);
    };
  }, [
    announcement.id,
    announcement.revision,
    announcement.ends_at,
    announcement.endless,
  ]);
  return open ? (
    <TimedAnnouncementDialog announcement={announcement} onClose={onClose} />
  ) : null;
}
