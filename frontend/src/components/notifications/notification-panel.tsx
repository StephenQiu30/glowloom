"use client";

import { Suspense, useRef } from "react";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { useIsMobile } from "@/hooks/use-mobile";
import { NotificationInbox } from "./notification-inbox";

export function NotificationPanel({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const mobile = useIsMobile();
  const returnFocus = useRef<HTMLElement | null>(null);
  const interactedOutside = useRef(false);
  return (
    <Sheet open={open} onOpenChange={onOpenChange} modal={mobile}>
      <SheetContent
        variant="notification"
        aria-describedby={undefined}
        onOpenAutoFocus={() => {
          returnFocus.current =
            document.activeElement instanceof HTMLElement
              ? document.activeElement
              : null;
          interactedOutside.current = false;
        }}
        onCloseAutoFocus={(event) => {
          if (interactedOutside.current) return;
          const target =
            document.querySelector<HTMLElement>(
              "[data-notification-trigger]",
            ) ?? returnFocus.current;
          if (target?.isConnected) {
            event.preventDefault();
            target.focus();
          }
        }}
        onInteractOutside={(event) => {
          if (
            event.target instanceof Element &&
            event.target.closest(
              '[role="dialog"], [role="alertdialog"], [data-notification-trigger]',
            )
          )
            event.preventDefault();
          else interactedOutside.current = true;
        }}
      >
        <SheetHeader>
          <SheetTitle>消息中心</SheetTitle>
        </SheetHeader>
        <Suspense>
          <NotificationInbox />
        </Suspense>
      </SheetContent>
    </Sheet>
  );
}
