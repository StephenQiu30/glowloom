"use client";

import { type ReactNode } from "react";

import { BasicFooter } from "./basic-footer";
import { BasicHeader, type LayoutAuthState, type LayoutViewer } from "./basic-header";
import { type StudioNavigation, type WorkspaceRole } from "@/lib/access-control";

export function BasicLayout({
  active,
  authState,
  artwork,
  children,
  currentStep,
  projectName,
  role,
  viewer,
}: {
  active?: StudioNavigation;
  authState: LayoutAuthState;
  artwork?: ReactNode;
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
      className={artwork ? "basic-layout basic-layout--authentication" : "basic-layout"}
      data-auth-state={authState}
      data-has-progress={hasProgress ? "true" : "false"}
      data-has-project-context={hasProjectContext ? "true" : "false"}
    >
      <div className="basic-layout__body">
        <a className="basic-layout__skip-link" href="#main">
          跳到主要内容
        </a>
        <BasicHeader
          active={active}
          compact={Boolean(artwork)}
          authState={authState}
          currentStep={currentStep}
          projectName={projectName}
          role={role}
          viewer={viewer}
        />
        <main className="basic-layout__main" id="main" tabIndex={-1}>
          {children}
        </main>
        <BasicFooter compact={Boolean(artwork)} />
      </div>
      {artwork ? <aside className="basic-layout__artwork">{artwork}</aside> : null}
    </div>
  );
}
