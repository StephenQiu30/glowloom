"use client";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useForm, Controller, useWatch } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { LibrarySelect } from "@/components/media/library-select";
import {
  defaultLibraryFilter,
  folderAncestry,
  libraryUUID,
  type LibraryIdentity,
  type LibraryScope,
} from "@/components/media/library-model";
import { freshLibrary, listLibrary } from "@/components/media/library-queries";
import {
  getProject,
  type ProjectListedSummary,
} from "@/components/project/queries";
import type { TransferWriter } from "./use-transfer";
import { transferInputSchema } from "./transfer-model";
const formSchema = z
  .object({
    projectId: z.union([z.literal(""), libraryUUID]),
    folderId: z.union([z.literal("root"), libraryUUID]),
  })
  .strict();
export type TransferSelection = {
  items: { id: string; revision: number }[];
  sourceRevision: number;
};
export function TransferForm({
  identity,
  selection,
  projects,
  hasMoreProjects,
  projectsLoading,
  onMoreProjects,
  writer,
}: {
  identity: LibraryIdentity;
  selection: TransferSelection;
  projects: ProjectListedSummary[];
  hasMoreProjects: boolean;
  projectsLoading: boolean;
  onMoreProjects: () => void;
  writer: TransferWriter;
}) {
  const [error, setError] = useState<string>();
  const form = useForm<z.infer<typeof formSchema>>({
      resolver: zodResolver(formSchema),
      defaultValues: { projectId: "", folderId: "root" },
    }),
    [projectId, folderId] = useWatch({
      control: form.control,
      name: ["projectId", "folderId"],
    });
  const target: LibraryScope =
    identity.scope.kind === "project"
      ? { kind: "personal" }
      : { kind: "project", project_id: projectId };
  const hasTarget =
    target.kind === "personal" || libraryUUID.safeParse(projectId).success;
  const scopeProjectId =
    identity.scope.kind === "project" ? identity.scope.project_id : projectId;
  const targetPage = useQuery({
    queryKey: [
      "media-transfer-target",
      identity.origin,
      identity.actorId,
      identity.orgId,
      target.kind,
      projectId,
    ],
    queryFn: async ({ signal }) => {
      const page = await listLibrary(
        target,
        { ...defaultLibraryFilter(target), page_size: 1 },
        signal,
      );
      if (
        page.current_actor_id !== identity.actorId ||
        page.current_org_id !== identity.orgId
      )
        throw new Error("当前目标素材库身份已变化，请重新打开。");
      return page;
    },
    enabled: hasTarget,
    retry: false,
    staleTime: 0,
    gcTime: 0,
  });
  const project = useQuery({
    queryKey: [
      "media-transfer-project",
      identity.origin,
      identity.actorId,
      identity.orgId,
      scopeProjectId,
    ],
    queryFn: ({ signal }) => getProject(scopeProjectId, signal),
    enabled: libraryUUID.safeParse(scopeProjectId).success,
    retry: false,
    staleTime: 0,
    gcTime: 0,
  });
  const writable =
    project.data &&
    !project.data.is_delete &&
    (target.kind === "personal" || project.data.status === "active");
  const disabled =
    writer.locked ||
    form.formState.isSubmitting ||
    !targetPage.data ||
    !targetPage.isFetchedAfterMount ||
    targetPage.isError ||
    !project.isFetchedAfterMount ||
    project.isError ||
    !writable;
  async function submit() {
    if (disabled || !targetPage.data || !project.data) return;
    setError(undefined);
    try {
      const [source, latestTarget, latestProject] = await Promise.all([
        freshLibrary(identity),
        listLibrary(target, { ...defaultLibraryFilter(target), page_size: 1 }),
        getProject(scopeProjectId),
      ]);
      if (source.revision !== selection.sourceRevision)
        throw new Error(
          "来源素材库版本已变化。所选项与草稿保留，请关闭后读取当前来源并重新选择。",
        );
      if (
        latestTarget.current_actor_id !== identity.actorId ||
        latestTarget.current_org_id !== identity.orgId
      )
        throw new Error("目标素材库身份已变化，当前不会提交。");
      if (
        latestTarget.revision !== targetPage.data.revision ||
        latestProject.revision !== project.data.revision
      ) {
        await Promise.all([targetPage.refetch(), project.refetch()]);
        throw new Error(
          "目标目录、素材库或项目版本已变化，请审阅新版本后再次确认。",
        );
      }
      const folder =
        folderId === "root"
          ? null
          : latestTarget.folders.find((folder) => folder.id === folderId);
      if (folderId !== "root" && !folder)
        throw new Error("目标目录已不存在，请选择当前真实目录。");
      await writer.submit({
        action: "create",
        body: transferInputSchema.parse({
          source: identity.scope,
          target,
          items: selection.items,
          expected_source_revision: selection.sourceRevision,
          expected_target_revision: latestTarget.revision,
          expected_project_revision: latestProject.revision,
          target_folder_id: folder?.id ?? null,
          expected_folder_revision: folder?.revision ?? 0,
        }),
      });
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "迁移当前不能提交。");
    }
  }
  return (
    <form
      onSubmit={form.handleSubmit(submit)}
      className="space-y-4"
      aria-label="迁移选中素材"
    >
      <h2 className="font-medium">
        {identity.scope.kind === "personal"
          ? "加入项目素材库"
          : "保存到个人素材库"}
      </h2>
      <p className="text-sm text-muted-foreground">
        保留来源原件及已发布项目内容。项目目标不接管个人备注或收藏；成功后使用新的目标素材
        ID。
      </p>
      {identity.scope.kind === "personal" && (
        <div className="space-y-2">
          <Label>目标项目</Label>
          <Controller
            name="projectId"
            control={form.control}
            render={({ field }) => (
              <LibrarySelect
                label="迁移目标项目"
                value={field.value}
                options={projects.map((project) => ({
                  value: project.id,
                  label: project.name,
                  disabled: project.status !== "active" || project.is_delete,
                }))}
                disabled={writer.locked || form.formState.isSubmitting}
                onChange={(value) => {
                  field.onChange(value);
                  form.setValue("folderId", "root");
                  setError(undefined);
                }}
              />
            )}
          />
          {!projects.length && <p>当前未读取到可写项目。</p>}
          {hasMoreProjects && (
            <Button
              type="button"
              variant="outline"
              disabled={projectsLoading || writer.locked}
              onClick={onMoreProjects}
            >
              加载更多目标项目
            </Button>
          )}
        </div>
      )}
      {hasTarget && (
        <>
          <div className="space-y-2">
            <Label>目标目录</Label>
            <Controller
              name="folderId"
              control={form.control}
              render={({ field }) => (
                <LibrarySelect
                  label="迁移目标目录"
                  value={field.value}
                  disabled={
                    writer.locked ||
                    form.formState.isSubmitting ||
                    !targetPage.data ||
                    targetPage.isError
                  }
                  onChange={field.onChange}
                  options={[
                    { value: "root", label: "根目录" },
                    ...(targetPage.data?.folders ?? []).map((folder) => ({
                      value: folder.id,
                      label: folderAncestry(targetPage.data!.folders, folder.id)
                        .map((part) => part.name)
                        .join(" / "),
                    })),
                  ]}
                />
              )}
            />
          </div>
          {(targetPage.error || project.error) && (
            <p role="alert">
              {targetPage.error?.message ?? project.error?.message}
              <Button
                type="button"
                variant="outline"
                onClick={() =>
                  void Promise.all([targetPage.refetch(), project.refetch()])
                }
              >
                重新读取目标
              </Button>
            </p>
          )}
          {project.data?.status === "archived" && target.kind === "project" && (
            <p role="alert">目标项目已归档，当前不能加入素材。</p>
          )}
          {targetPage.data && project.data && (
            <p className="text-sm">
              来源库版本 {selection.sourceRevision} · 目标库版本{" "}
              {targetPage.data.revision} · 项目版本 {project.data.revision}
            </p>
          )}
        </>
      )}
      <details>
        <summary>审阅全部 {selection.items.length} 项及冻结版本</summary>
        <Textarea
          aria-label="全部所选素材ID与冻结版本"
          readOnly
          rows={6}
          value={selection.items
            .map((item) => `${item.id} · 条目版本 ${item.revision}`)
            .join("\n")}
        />
      </details>
      {error && <p role="alert">{error}</p>}
      <Button type="submit" disabled={disabled}>
        确认独立迁移 {selection.items.length} 项
      </Button>
    </form>
  );
}
