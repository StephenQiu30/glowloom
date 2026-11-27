"use client";

import { useTheme } from "next-themes";
import { useEffect } from "react";
import { Toaster as Sonner, toast, type ToasterProps } from "sonner";
import {
  CircleCheckIcon,
  InfoIcon,
  TriangleAlertIcon,
  CircleXIcon,
  Loader2Icon,
  XIcon,
} from "lucide-react";

const Toaster = ({ ...props }: ToasterProps) => {
  const { theme = "dark" } = useTheme();
  useEffect(() => {
    function dismissCurrent(event: KeyboardEvent) {
      if (event.key !== "Escape" || event.defaultPrevented) return;
      const focused = document.activeElement;
      if (
        document.querySelector('[role="dialog"], [role="alertdialog"]') &&
        !focused?.closest("[data-sonner-toast]")
      )
        return;
      const current = toast.getToasts().at(-1);
      if (!current || !("title" in current) || current.dismissible === false)
        return;
      event.preventDefault();
      event.stopPropagation();
      current.onDismiss?.(current);
      toast.dismiss(current.id);
    }
    document.addEventListener("keydown", dismissCurrent, true);
    return () => document.removeEventListener("keydown", dismissCurrent, true);
  }, []);

  return (
    <Sonner
      theme={theme as ToasterProps["theme"]}
      position="top-center"
      offset={{ top: "calc(84px + var(--global-announcement-height, 0px))" }}
      mobileOffset={{
        top: "calc(max(76px, env(safe-area-inset-top)) + var(--global-announcement-height, 0px))",
        left: 16,
        right: 16,
      }}
      duration={5000}
      visibleToasts={2}
      closeButton
      containerAriaLabel="通知"
      className="toaster group"
      icons={{
        success: <CircleCheckIcon className="size-4" />,
        info: <InfoIcon className="size-4" />,
        warning: <TriangleAlertIcon className="size-4" />,
        error: <CircleXIcon className="size-4" />,
        loading: <Loader2Icon className="size-4 animate-spin" />,
        close: <XIcon />,
      }}
      style={
        {
          pointerEvents: "auto",
          "--normal-bg": "var(--popover)",
          "--normal-text": "var(--popover-foreground)",
          "--normal-border": "var(--border)",
          "--border-radius": "var(--radius)",
        } as React.CSSProperties
      }
      toastOptions={{
        closeButtonAriaLabel: "关闭提示",
      }}
      {...props}
    />
  );
};

export { Toaster };
