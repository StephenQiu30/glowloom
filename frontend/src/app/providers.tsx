"use client";

import { ApplicationLayout } from "@/components/layout/application-layout";
import { ThemeProvider } from "next-themes";
import type { ReactNode } from "react";
import { Toaster } from "@/components/ui/sonner";
import { SessionProvider } from "@/components/auth/session";
import { TooltipProvider } from "@/components/ui/tooltip";
import { SpaceProvider } from "@/components/spaces/space-context";
import { ProjectCreationProvider } from "@/components/projects/project-creation";
import { usePathname } from "next/navigation";

export function Providers({
  children,
  initialSidebarOpen = true,
}: {
  children: ReactNode;
  initialSidebarOpen?: boolean;
}) {
  const pathname = usePathname();
  const preview =
    process.env.NODE_ENV === "development" && pathname === "/canvas/preview";
  return (
    <ThemeProvider
      attribute="class"
      defaultTheme="dark"
      enableSystem
      disableTransitionOnChange
    >
      {preview ? (
        <TooltipProvider>{children}</TooltipProvider>
      ) : (
        <SessionProvider>
          <SpaceProvider blockUntilReady={false}>
            <ProjectCreationProvider>
              <TooltipProvider>
                <ApplicationLayout initialSidebarOpen={initialSidebarOpen}>
                  {children}
                </ApplicationLayout>
              </TooltipProvider>
            </ProjectCreationProvider>
          </SpaceProvider>
        </SessionProvider>
      )}
      <Toaster />
    </ThemeProvider>
  );
}
