"use client";

import { type ReactNode } from "react";

import { BasicLayout } from "@/components/layout/basic-layout";
import { layoutContainerClassName } from "@/components/layout/layout-container";
import { useAuthSessionState } from "@/features/identity/use-auth-session";
import { type StudioNavigation } from "@/lib/access-control";
import { clearAccessToken } from "@/lib/auth-session";
import { useMeQuery, useLogoutMutation } from "@/features/identity/endpoints";

export type { StudioNavigation } from "@/lib/access-control";
export const studioContainerClassName = layoutContainerClassName;

export function StudioShell({
  active,
  children,
  projectName,
  currentStep,
  viewer,
}: {
  active: StudioNavigation;
  children: ReactNode;
  projectName?: string;
  currentStep?: number;
  viewer?: { displayName: string; workspaceName: string };
}) {
  const [logout, logoutState] = useLogoutMutation();
  async function handleLogout() {
    try {
      await logout().unwrap();
    } finally {
      clearAccessToken();
      window.location.replace("/login");
    }
  }
  const sessionState = useAuthSessionState();
  const authenticated = sessionState === "authenticated";
  const me = useMeQuery(undefined, { skip: !authenticated });
  const role = me.data?.workspace.role;
  const resolvedViewer =
    viewer ??
    (me.data
      ? {
          displayName: me.data.user.display_name?.trim() || me.data.user.email,
          workspaceName: me.data.workspace.name,
        }
      : undefined);

  return (
    <BasicLayout
      active={active}
      authState={
        sessionState === "checking" ? "loading" : authenticated ? "authenticated" : "anonymous"
      }
      currentStep={currentStep}
      projectName={projectName}
      role={role}
      onLogout={handleLogout}
      loggingOut={logoutState.isLoading}
      viewer={resolvedViewer}
    >
      {children}
    </BasicLayout>
  );
}
