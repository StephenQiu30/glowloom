"use client";

import Link from "next/link";
import { AlertCircle } from "lucide-react";

import { StudioShell } from "@/components/identity/studio-shell";
import { useAuthSessionState } from "@/components/identity/use-auth-session";
import { useMeQuery } from "@/components/identity/endpoints";
import { useEpisodesQuery, useProjectQuery } from "@/components/project/endpoints";
import { PageLoading } from "@/components/system/page-loading";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { LayoutContainer } from "@/layout/layout-container";
import { appApiErrorMessage } from "@/lib/server-state";

import { CanvasBoard } from "./canvas-board";
import { useApplyCanvasOperationsMutation, useCanvasDocumentQuery } from "./endpoints";

export function CanvasWorkspace({ projectId }: { projectId: string }) {
  const sessionState = useAuthSessionState();
  const authenticated = sessionState === "authenticated";
  const meQuery = useMeQuery(undefined, { skip: !authenticated });
  const projectQuery = useProjectQuery(projectId, { skip: !authenticated });
  const episodesQuery = useEpisodesQuery(projectId, { skip: !authenticated });
  const canvasQuery = useCanvasDocumentQuery(projectId, { skip: !authenticated });
  const [applyOperations] = useApplyCanvasOperationsMutation();
  const error = meQuery.error ?? projectQuery.error ?? episodesQuery.error ?? canvasQuery.error;

  return (
    <StudioShell active="projects" projectName={projectQuery.data?.name}>
      <LayoutContainer className="py-8 sm:py-10">
        {sessionState === "checking" ? (
          <PageLoading label="正在读取登录状态" />
        ) : !authenticated ? (
          <Alert>
            <AlertCircle aria-hidden="true" />
            <AlertTitle>需要登录</AlertTitle>
            <AlertDescription>
              <Link className="underline" href="/login">
                登录后查看项目画布
              </Link>
            </AlertDescription>
          </Alert>
        ) : error ? (
          <Alert variant="destructive">
            <AlertCircle aria-hidden="true" />
            <AlertTitle>项目画布暂时无法读取</AlertTitle>
            <AlertDescription>{appApiErrorMessage(error)}</AlertDescription>
          </Alert>
        ) : !projectQuery.data || !episodesQuery.data || !canvasQuery.data ? (
          <PageLoading label="正在加载项目画布" />
        ) : (
          <CanvasBoard
            canEdit={meQuery.data?.workspace.role !== "viewer" && projectQuery.data.status === "active"}
            document={canvasQuery.data}
            episodes={episodesQuery.data}
            onApply={(operations) =>
              applyOperations({
                projectId,
                body: { operations, idempotency_key: crypto.randomUUID() },
              }).unwrap()
            }
            project={projectQuery.data}
          />
        )}
      </LayoutContainer>
    </StudioShell>
  );
}
