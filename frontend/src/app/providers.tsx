"use client";

import { ApplicationLayout } from "@/components/layout/application-layout";
import { ThemeProvider } from "next-themes";
import type { ReactNode } from "react";
import { Toaster } from "@/components/ui/sonner";
import { SessionProvider } from "@/components/auth/session";
import { TooltipProvider } from "@/components/ui/tooltip";
import { SpaceProvider } from "@/components/spaces/space-context";
import { ProjectCreationProvider } from "@/components/projects/project-creation";

export function Providers({
  children,
  initialSidebarOpen = true,
}: {
  children: ReactNode;
  initialSidebarOpen?: boolean;
}) {
  return (
    <ThemeProvider
      attribute="class"
      defaultTheme="dark"
      enableSystem
      disableTransitionOnChange
    >
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
      <Toaster />
    </ThemeProvider>
  );
}
