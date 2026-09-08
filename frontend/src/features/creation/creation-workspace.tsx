"use client";

import Link from "next/link";
import { LoaderCircle } from "lucide-react";
import { LayoutContainer } from "@/components/layout/layout-container";
import { PageHeader } from "@/components/studio/page-header";
import { StudioShell } from "@/components/studio/studio-shell";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { useMeQuery } from "@/features/identity/endpoints";
import { useProjectQuery } from "@/features/project/endpoints";
import { useCurrentScriptDocumentQuery } from "@/features/script/endpoints";
import { useAuthSessionState } from "@/hooks/use-auth-session";
import { appApiErrorMessage } from "@/lib/server-state";
import { TextCreationWorkspace } from "./text-creation-workspace";

export function CreationWorkspace({ projectId, initialRunId }: { projectId: string; initialRunId?: string }) {
  const session = useAuthSessionState();
  const authenticated = session === "authenticated";
  const me = useMeQuery(undefined, { skip: !authenticated });
  const project = useProjectQuery(projectId, { skip: !authenticated });
  const script = useCurrentScriptDocumentQuery(projectId, { skip: !authenticated });
  const source = script.data;
  const error = me.error ?? project.error ?? ((script.error as { code?: string })?.code === "not_found" ? undefined : script.error);
  return <StudioShell active="projects" projectName={project.data?.name} viewer={me.data ? { displayName: me.data.user.display_name || me.data.user.email, workspaceName: me.data.workspace.name } : undefined}><LayoutContainer className="space-y-7 py-8 sm:py-10">
    <PageHeader title="文本创作" description="从固定原稿到逐场导演分镜，审阅每一步，保存可追溯的正式结果。" breadcrumbs={[{ label: "项目", href: "/projects" }, { label: project.data?.name ?? "项目", href: `/projects/${projectId}` }, { label: "文本创作" }]} />
    {session === "checking" || (authenticated && (project.isLoading || me.isLoading || script.isLoading)) ? <LoaderCircle className="animate-spin" aria-label="正在读取创作项目" /> : !authenticated ? <Link className="underline" href="/login">登录后开始创作</Link> : error ? <Alert variant="destructive"><AlertDescription>{appApiErrorMessage(error)}</AlertDescription></Alert> : project.data && <TextCreationWorkspace projectId={projectId} initialRunId={initialRunId} canWrite={Boolean(me.data && me.data.workspace.role !== "viewer" && project.data.status === "active")} source={source ? { revisionId: source.revision.id, contentHash: source.revision.normalized_hash, title: source.document.title, text: source.revision.normalized_text } : undefined} />}
  </LayoutContainer></StudioShell>;
}
