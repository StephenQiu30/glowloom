"use client";

import type { ReactNode } from "react";
import { usePathname } from "next/navigation";
import { BasicLayout } from "./basic-layout";
import { resolveScreen } from "./routes";

/** Mounted once under the root providers. Canvas and security pages own their layout. */
export function ApplicationLayout({
  children,
  initialSidebarOpen = true,
}: {
  children: ReactNode;
  initialSidebarOpen?: boolean;
}) {
  const pathname = usePathname();
  const screen = resolveScreen(pathname);
  if (!screen || screen.id === "canvas" || screen.id === "reset-password")
    return children;

  const authMode =
    screen.id === "login" || screen.id === "register" ? screen.id : undefined;
  return (
    <BasicLayout
      screen={authMode ? "home" : screen.id}
      contentKey={pathname}
      initialSidebarOpen={initialSidebarOpen}
      allowGuest={Boolean(
        authMode ||
        ["home", "explore", "assets", "invitations"].includes(screen.id),
      )}
      initialAuthMode={authMode}
    >
      {children}
    </BasicLayout>
  );
}
