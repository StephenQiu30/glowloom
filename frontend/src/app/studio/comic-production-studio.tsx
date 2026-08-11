"use client";

import {
  AlertCircle,
  CheckCircle2,
  Layers3,
  Plus,
  Search,
} from "lucide-react";
import Link from "next/link";
import { useState } from "react";
import { Tabs } from "radix-ui";

import { PageHeader } from "@/components/studio/page-header";
import { StudioShell } from "@/components/studio/studio-shell";
import {
  Alert,
  AlertDescription,
  AlertTitle,
} from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useAuthSessionState } from "@/hooks/use-auth-session";
import {
  appApiErrorMessage,
  useAppendAssetVersionMutation,
  useAssetDeletePreflightMutation,
  useAssetReadinessQuery,
  useAssetsQuery,
  useAssetVersionsQuery,
  useCreateAssetMutation,
  useDeleteAssetMutation,
  useMeQuery,
  useMediaVersionsQuery,
  useProjectsQuery,
  useSetAssetArchivedMutation,
  useSetCurrentAssetVersionMutation,
  useUpdateAssetMutation,
} from "@/lib/server-state";

import {
  CreateAssetDialog,
  DeleteAssetDialog,
  EditAssetDialog,
  VersionDialog,
} from "./asset-dialogs";
import { AssetDetail, AssetList } from "./asset-panels";
import {
  type AssetKind,
  assetTypes,
  typeConfig,
} from "./asset-workspace-model";

export function ComicProductionStudio() {
  const sessionState = useAuthSessionState();
  const authenticated = sessionState === "authenticated";
  const me = useMeQuery(undefined, { skip: !authenticated });
  const workspaceId = me.data?.workspace.id;
  const projects = useProjectsQuery(workspaceId ?? "", { skip: !workspaceId });
  const projectItems = projects.data?.items ?? [];
  const [selectedProjectId, setSelectedProjectId] = useState<string | null>(null);
  const effectiveProject =
    projectItems.find((project) => project.id === selectedProjectId) ??
    projectItems.find((project) => project.status === "active") ??
    projectItems[0];
  const assets = useAssetsQuery(effectiveProject?.id ?? "", {
    skip: !effectiveProject,
  });
  const media = useMediaVersionsQuery(workspaceId ?? "", { skip: !workspaceId });
  const [selectedKind, setSelectedKind] = useState<AssetKind>("character");
  const [query, setQuery] = useState("");
  const [selectedAssetId, setSelectedAssetId] = useState<string | null>(null);
  const allAssets = assets.data?.items ?? [];
  const normalizedQuery = query.trim().toLocaleLowerCase("zh-CN");
  const visibleAssets = allAssets.filter(
    (asset) =>
      asset.kind === selectedKind &&
      (!normalizedQuery ||
        asset.name.toLocaleLowerCase("zh-CN").includes(normalizedQuery) ||
        asset.aliases.some((alias) =>
          alias.toLocaleLowerCase("zh-CN").includes(normalizedQuery),
        )),
  );
  const selectedAsset =
    visibleAssets.find((asset) => asset.id === selectedAssetId) ?? visibleAssets[0];
  const versions = useAssetVersionsQuery(selectedAsset?.id ?? "", {
    skip: !selectedAsset,
  });
  const versionItems = versions.data?.items ?? [];
  const currentVersion =
    versionItems.find((version) => version.id === selectedAsset?.current_version_id) ??
    versionItems[0];
  const readiness = useAssetReadinessQuery(currentVersion?.id ?? "", {
    refetchOnMountOrArgChange: true,
    skip: !currentVersion,
  });
  const [createAsset, createState] = useCreateAssetMutation();
  const [appendVersion, appendState] = useAppendAssetVersionMutation();
  const [setAssetArchived, archiveState] = useSetAssetArchivedMutation();
  const [updateAsset, updateState] = useUpdateAssetMutation();
  const [setCurrentAssetVersion, currentVersionState] =
    useSetCurrentAssetVersionMutation();
  const [loadDeletePreflight, deletePreflightState] =
    useAssetDeletePreflightMutation();
  const [deleteAsset, deleteState] = useDeleteAssetMutation();
  const [createOpen, setCreateOpen] = useState(false);
  const [editOpen, setEditOpen] = useState(false);
  const [versionOpen, setVersionOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deletePreflight, setDeletePreflight] =
    useState<API.AssetDeletePreflightResponse>();
  const [notice, setNotice] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const mediaVersions = media.data?.items ?? [];
  const mediaById = new Map(mediaVersions.map((item) => [item.id, item]));
  const characterAssets = allAssets.filter(
    (asset) => asset.kind === "character" && asset.status === "active",
  );

  async function submitCreate(request: API.AssetCreateRequest): Promise<boolean> {
    if (!effectiveProject) return false;
    setActionError(null);
    try {
      const created = await createAsset({
        projectId: effectiveProject.id,
        body: request,
      }).unwrap();
      setSelectedKind(created.kind);
      setSelectedAssetId(created.id);
      setCreateOpen(false);
      setNotice(`资产身份已创建：${created.name}`);
      return true;
    } catch (error: unknown) {
      setActionError(appApiErrorMessage(error));
      return false;
    }
  }

  async function submitVersion(
    request: API.AssetVersionCreateRequest,
  ): Promise<boolean> {
    if (!selectedAsset) return false;
    setActionError(null);
    try {
      const created = await appendVersion({
        assetId: selectedAsset.id,
        body: request,
      }).unwrap();
      setVersionOpen(false);
      const statusLabel =
        created.readiness.status === "ready"
          ? "可用于生成"
          : created.readiness.status === "blocked"
            ? "已阻断"
            : "草稿未完整";
      setNotice(`版本 v${created.version.version_no} 已保存，准备度为${statusLabel}。`);
      return true;
    } catch (error: unknown) {
      setActionError(appApiErrorMessage(error));
      return false;
    }
  }

  async function toggleArchive() {
    if (!selectedAsset) return;
    setActionError(null);
    setNotice(null);
    try {
      const updated = await setAssetArchived({
        assetId: selectedAsset.id,
        expectedRevision: selectedAsset.revision,
        archived: selectedAsset.status === "active",
      }).unwrap();
      await assets.refetch().unwrap();
      setNotice(updated.status === "archived" ? "资产已归档。" : "资产已恢复。");
    } catch (error: unknown) {
      setActionError(appApiErrorMessage(error));
    }
  }

  async function submitEdit(request: API.AssetUpdateRequest): Promise<boolean> {
    if (!selectedAsset || !effectiveProject) return false;
    setActionError(null);
    try {
      const updated = await updateAsset({
        projectId: effectiveProject.id,
        assetId: selectedAsset.id,
        body: request,
      }).unwrap();
      setEditOpen(false);
      setNotice(`资产身份已更新：${updated.name}`);
      return true;
    } catch (error: unknown) {
      setActionError(appApiErrorMessage(error));
      return false;
    }
  }

  async function selectCurrentVersion(version: API.AssetVersionResponse) {
    if (!selectedAsset || !effectiveProject) return;
    setActionError(null);
    setNotice(null);
    try {
      await setCurrentAssetVersion({
        projectId: effectiveProject.id,
        assetId: selectedAsset.id,
        body: {
          version_id: version.id,
          expected_current_version_id: selectedAsset.current_version_id,
          expected_revision: selectedAsset.revision,
        },
      }).unwrap();
      await assets.refetch().unwrap();
      setNotice(`资产已切换到版本 v${version.version_no}；既有镜头引用保持不变。`);
    } catch (error: unknown) {
      setActionError(appApiErrorMessage(error));
    }
  }

  async function prepareDelete() {
    if (!selectedAsset) return;
    setDeletePreflight(undefined);
    setDeleteOpen(true);
    setActionError(null);
    try {
      setDeletePreflight(await loadDeletePreflight(selectedAsset.id).unwrap());
    } catch (error: unknown) {
      setActionError(appApiErrorMessage(error));
    }
  }

  async function confirmDelete() {
    if (!selectedAsset || !effectiveProject || !deletePreflight?.allowed) return;
    setActionError(null);
    try {
      await deleteAsset({
        projectId: effectiveProject.id,
        assetId: selectedAsset.id,
        expectedRevision: selectedAsset.revision,
      }).unwrap();
      setDeleteOpen(false);
      setDeletePreflight(undefined);
      setSelectedAssetId(null);
      setNotice(`空资产“${selectedAsset.name}”已删除。`);
    } catch (error: unknown) {
      setActionError(appApiErrorMessage(error));
    }
  }

  const activeCount = allAssets.filter((asset) => asset.status === "active").length;
  const versionedCount = allAssets.filter((asset) => asset.current_version_id).length;
  const pageError = me.error ?? projects.error ?? assets.error ?? media.error;

  return (
    <StudioShell
      active="assets"
      projectName={effectiveProject?.name}
      topAction={
        authenticated ? (
          effectiveProject?.status === "active" ? (
            <Button
              className="h-10 bg-primary px-4 text-white hover:bg-primary/85"
              onClick={() => setCreateOpen(true)}
            >
              <Plus aria-hidden="true" />新建资产
            </Button>
          ) : (
            <Button disabled className="h-10" variant="outline">
              项目已归档
            </Button>
          )
        ) : (
          <Button
            asChild
            className="h-10 bg-primary px-4 text-white hover:bg-primary/85"
          >
            <Link href="/login">登录后管理</Link>
          </Button>
        )
      }
    >
      {notice ? (
        <div
          className="pointer-events-none fixed top-24 right-6 z-50 flex items-center gap-2 rounded-xl border border-emerald-200 bg-white px-4 py-3 text-sm shadow-lg shadow-slate-950/10"
          role="status"
        >
          <CheckCircle2 className="size-4 text-emerald-600" aria-hidden="true" />
          {notice}
        </div>
      ) : null}

      <div className="mx-auto max-w-[1420px] px-5 py-8 md:px-8">
        <PageHeader
          actions={projectItems.length > 0 ? (
            <div className="grid min-w-56 gap-2">
              <Label htmlFor="assetProject">当前项目</Label>
              <Select
                value={effectiveProject?.id ?? ""}
                onValueChange={(value) => {
                  setSelectedProjectId(value);
                  setSelectedAssetId(null);
                }}
              >
                <SelectTrigger id="assetProject"><SelectValue /></SelectTrigger>
                <SelectContent>
                  {projectItems.map((project) => (
                    <SelectItem key={project.id} value={project.id}>
                      {project.name}{project.status === "archived" ? "（已归档）" : ""}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          ) : null}
          badges={[{ label: "生产事实层" }]}
          description="管理角色、场景、道具、服装、声音与视觉风格的稳定身份、不可变版本和生产准备度。"
          title="资产库"
        />

        {sessionState === "checking" || (authenticated && me.isLoading) ? (
          <Card className="mt-7">
            <CardContent className="p-6 text-sm text-slate-500">
              正在读取创作空间…
            </CardContent>
          </Card>
        ) : null}
        {sessionState === "anonymous" ? (
          <Alert className="mt-7 border-amber-200 bg-amber-50 p-5 text-amber-800">
            <AlertCircle aria-hidden="true" />
            <AlertTitle>需要登录</AlertTitle>
            <AlertDescription className="text-amber-700">
              资产身份与版本受 Workspace 隔离保护。
              <Link className="ml-1 font-medium underline" href="/login">
                前往登录
              </Link>
            </AlertDescription>
          </Alert>
        ) : null}
        {authenticated && pageError ? (
          <Alert
            className="mt-7 border-rose-200 bg-rose-50 p-5 text-rose-800"
            variant="destructive"
          >
            <AlertCircle aria-hidden="true" />
            <AlertTitle>资产事实暂时无法读取</AlertTitle>
            <AlertDescription>{appApiErrorMessage(pageError)}</AlertDescription>
          </Alert>
        ) : null}
        {actionError ? (
          <Alert
            className="mt-5 border-rose-200 bg-rose-50 p-4 text-rose-800"
            variant="destructive"
          >
            <AlertCircle aria-hidden="true" />
            <AlertTitle>操作未完成</AlertTitle>
            <AlertDescription>{actionError}</AlertDescription>
          </Alert>
        ) : null}

        {workspaceId && !projects.isLoading && projectItems.length === 0 ? (
          <Card className="mt-7">
            <CardContent className="grid min-h-56 place-items-center p-8 text-center">
              <div>
                <Layers3 className="mx-auto size-7 text-slate-300" aria-hidden="true" />
                <h2 className="mt-3 text-lg font-semibold">先创建一个漫剧项目</h2>
                <p className="mt-2 text-sm text-slate-500">
                  资产必须归属于明确项目，不能游离在工作区之外。
                </p>
                <Button asChild className="mt-5" variant="outline">
                  <Link href="/projects">前往项目管理</Link>
                </Button>
              </div>
            </CardContent>
          </Card>
        ) : null}

        {effectiveProject ? (
          <>
            <section className="mt-7 grid gap-4 sm:grid-cols-3" aria-label="资产概览">
              <Card>
                <CardHeader>
                  <CardDescription>资产身份</CardDescription>
                  <CardTitle className="text-2xl">{assets.data?.total ?? 0}</CardTitle>
                </CardHeader>
              </Card>
              <Card>
                <CardHeader>
                  <CardDescription>当前使用</CardDescription>
                  <CardTitle className="text-2xl text-emerald-700">{activeCount}</CardTitle>
                </CardHeader>
              </Card>
              <Card>
                <CardHeader>
                  <CardDescription>已有当前版本</CardDescription>
                  <CardTitle className="text-2xl text-foreground">
                    {versionedCount}
                  </CardTitle>
                </CardHeader>
              </Card>
            </section>

            <Tabs.Root
              className="mt-6"
              onValueChange={(value) => {
                setSelectedKind(value as AssetKind);
                setSelectedAssetId(null);
              }}
              value={selectedKind}
            >
              <Tabs.List
                aria-label="资产类型"
                className="flex gap-1 overflow-x-auto rounded-xl border border-slate-200 bg-white p-1"
              >
                {assetTypes.map((item) => {
                  const Icon = item.icon;
                  const count = allAssets.filter(
                    (asset) => asset.kind === item.id,
                  ).length;
                  return (
                    <Tabs.Trigger
                      className="flex min-w-24 flex-1 items-center justify-center gap-2 rounded-lg px-3 py-2.5 text-sm text-slate-500 outline-none transition hover:bg-slate-50 data-[state=active]:bg-slate-100 data-[state=active]:font-medium data-[state=active]:text-foreground"
                      key={item.id}
                      value={item.id}
                    >
                      <Icon className="size-4" aria-hidden="true" />
                      {item.label}
                      <span className="text-xs text-slate-400">{count}</span>
                    </Tabs.Trigger>
                  );
                })}
              </Tabs.List>
            </Tabs.Root>

            <div className="mt-5 grid items-start gap-5 lg:grid-cols-[320px_minmax(0,1fr)]">
              <div className="grid gap-3">
                <div className="relative">
                  <Search
                    className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-slate-400"
                    aria-hidden="true"
                  />
                  <Input
                    aria-label="搜索资产"
                    className="bg-white pl-9"
                    onChange={(event) => setQuery(event.target.value)}
                    placeholder="搜索名称或别名"
                    value={query}
                  />
                </div>
                <AssetList
                  assets={visibleAssets}
                  isLoading={assets.isLoading}
                  onSelect={setSelectedAssetId}
                  selectedId={selectedAsset?.id}
                />
              </div>
              {selectedAsset ? (
                <AssetDetail
                  asset={selectedAsset}
                  isArchiving={archiveState.isLoading || assets.isFetching}
                  isChangingCurrent={currentVersionState.isLoading}
                  mediaById={mediaById}
                  onAddVersion={() => setVersionOpen(true)}
                  onDelete={() => void prepareDelete()}
                  onEdit={() => setEditOpen(true)}
                  onSetCurrent={(version) => void selectCurrentVersion(version)}
                  onToggleArchive={toggleArchive}
                  onUpgradeCompleted={(shotCount) => {
                    setActionError(null);
                    setNotice(`已为 ${shotCount} 个镜头创建新的规格版本。`);
                  }}
                  onUpgradeError={(message) => setActionError(message || null)}
                  readiness={currentVersion ? readiness.data : undefined}
                  readinessError={currentVersion ? readiness.error : undefined}
                  readinessLoading={currentVersion ? readiness.isLoading : false}
                  versions={versionItems}
                  versionsLoading={versions.isLoading}
                />
              ) : (
                <Card>
                  <CardContent className="grid min-h-80 place-items-center p-8 text-center">
                    <div>
                      <Layers3
                        className="mx-auto size-7 text-slate-300"
                        aria-hidden="true"
                      />
                      <p className="mt-3 text-sm font-medium">
                        选择或新建一个{typeConfig(selectedKind).singular}资产
                      </p>
                      <p className="mt-1 text-xs text-slate-500">
                        右侧将显示版本、媒体和准备度事实。
                      </p>
                    </div>
                  </CardContent>
                </Card>
              )}
            </div>
          </>
        ) : null}
      </div>

      <CreateAssetDialog
        currentKind={selectedKind}
        isSubmitting={createState.isLoading}
        key={selectedKind}
        onOpenChange={setCreateOpen}
        onSubmit={submitCreate}
        open={createOpen}
      />
      {selectedAsset ? (
        <>
          <EditAssetDialog
            asset={selectedAsset}
            isSubmitting={updateState.isLoading}
            onOpenChange={setEditOpen}
            onSubmit={submitEdit}
            open={editOpen}
          />
          <VersionDialog
            asset={selectedAsset}
            characters={characterAssets}
            isSubmitting={appendState.isLoading}
            mediaVersions={mediaVersions}
            onOpenChange={setVersionOpen}
            onSubmit={submitVersion}
            open={versionOpen}
          />
          <DeleteAssetDialog
            asset={selectedAsset}
            isDeleting={deleteState.isLoading}
            isLoading={deletePreflightState.isLoading}
            onConfirm={confirmDelete}
            onOpenChange={(open) => {
              setDeleteOpen(open);
              if (!open) setDeletePreflight(undefined);
            }}
            open={deleteOpen}
            preflight={deletePreflight}
          />
        </>
      ) : null}
    </StudioShell>
  );
}
