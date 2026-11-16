"use client";

import { ConversationProvider } from "@/components/conversations/conversation-context";
import { ApplicationLayout } from "@/components/layout/application-layout";
import { ThemeProvider } from "next-themes";
import type { ReactNode } from "react";
import { Toaster } from "@/components/ui/sonner";
import { SessionProvider } from "@/components/auth/session";
import { TooltipProvider } from "@/components/ui/tooltip";
import { SpaceProvider } from "@/components/spaces/space-context";
import { ProjectCreationProvider } from "@/components/projects/project-creation";
import { NotificationProvider } from "@/components/notifications/notification-provider";

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
        <NotificationProvider>
          <SpaceProvider blockUntilReady={false}>
            <ConversationProvider>
              <ProjectCreationProvider>
                <TooltipProvider>
                  <ApplicationLayout initialSidebarOpen={initialSidebarOpen}>
                    {children}
                  </ApplicationLayout>
                </TooltipProvider>
              </ProjectCreationProvider>
            </ConversationProvider>
          </SpaceProvider>
          <Toaster />
        </NotificationProvider>
      </SessionProvider>
    </ThemeProvider>
  );
}
