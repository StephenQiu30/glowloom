"use client";

import { type ReactNode } from "react";

import { BasicFooter } from "./basic-footer";
import { BasicHeader, type LayoutAuthState, type LayoutViewer } from "./basic-header";
import { type StudioNavigation, type WorkspaceRole } from "@/lib/access-control";

export function BasicLayout({
  active,
  authState,
  children,
  currentStep,
  projectName,
  role,
  viewer,
}: {
  active?: StudioNavigation;
  authState: LayoutAuthState;
  children: ReactNode;
  currentStep?: number;
  projectName?: string;
  role?: WorkspaceRole;
  viewer?: LayoutViewer;
}) {
  const hasProjectContext = Boolean(projectName);
  const hasProgress = typeof currentStep === "number";

  return (
    <div
      className="basic-layout"
      data-auth-state={authState}
      data-has-progress={hasProgress ? "true" : "false"}
      data-has-project-context={hasProjectContext ? "true" : "false"}
    >
      <a className="basic-layout__skip-link" href="#main">
        跳到主要内容
      </a>
      <BasicHeader
        active={active}
        authState={authState}
        currentStep={currentStep}
        projectName={projectName}
        role={role}
        viewer={viewer}
      />
      <main className="basic-layout__main" id="main">
        {children}
      </main>
      <BasicFooter />
    </div>
  );
}
