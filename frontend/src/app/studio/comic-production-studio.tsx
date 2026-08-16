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

import { LayoutContainer } from "@/components/layout/layout-container";
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
  useAssetBibleQuery,
  useAssetDisablePreflightMutation,
  useAssetDeletePreflightMutation,
  useAssetRenamePreflightMutation,
  useAssetStateDisablePreflightMutation,
  useAssetReadinessQuery,
  useAssetsQuery,
  useAssetVersionsQuery,
  useCreateAssetMutation,
  useCreateAssetStateMutation,
  useDeleteAssetMutation,
  useDisableAssetMutation,
  useDisableAssetStateMutation,
  useEnableAssetMutation,
  useEnableAssetStateMutation,
  useMeQuery,
  useMediaVersionsQuery,
  useProjectsQuery,
  useSetAssetArchivedMutation,
  useCurrentAssetVersionPreflightMutation,
  useRenameAssetMutation,
  useSetCurrentAssetVersionMutation,
  useUpdateAssetMutation,
  useUpdateAssetStateMutation,
} from "@/lib/server-state";

import { AssetImpactDialog, RenameAssetDialog } from "./asset-impact";
import {
  CreateAssetDialog,
  CreateStateDialog,
  DeleteAssetDialog,
  EditAssetDialog,
  EditStateDialog,
  VersionDialog,
} from "./asset-dialogs";
import {
  ArchivedAssetCard,
  AssetDetail,
  AssetList,
  AssetStateBar,
} from "./asset-panels";
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
  const assetBible = useAssetBibleQuery(effectiveProject?.id ?? "", {
    skip: !effectiveProject,
  });
  const media = useMediaVersionsQuery(workspaceId ?? "", { skip: !workspaceId });
  const [selectedKind, setSelectedKind] = useState<AssetKind>("character");
  const [query, setQuery] = useState("");
  const [selectedAssetId, setSelectedAssetId] = useState<string | null>(null);
  const [selectedStateId, setSelectedStateId] = useState<string | null>(null);
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
  const bibleItems = assetBible.data?.items ?? [];
  const statesByAssetId = new Map(
    bibleItems.map(({ asset, states }) => [
      asset.id,
      states.map(({ state }) => state),
    ]),
  );
  const selectedBible = bibleItems.find(
    ({ asset }) => asset.id === selectedAsset?.id,
  );
  const selectedStates = selectedBible?.states.map(({ state }) => state) ?? [];
  const selectedState =
    selectedStates.find((state) => state.id === selectedStateId) ??
    selectedStates.find((state) => state.state_key === "base") ??
    selectedStates[0];
  const versions = useAssetVersionsQuery(selectedState?.id ?? "", {
    skip: !selectedState,
  });
  const versionItems = versions.data?.items ?? [];
  const currentVersion =
    versionItems.find((version) => version.id === selectedState?.current_version_id) ??
    versionItems[0];
  const readiness = useAssetReadinessQuery(currentVersion?.id ?? "", {
    refetchOnMountOrArgChange: true,
    skip: !currentVersion,
  });
  const [createAsset, createState] = useCreateAssetMutation();
  const [createAssetState, createAssetStateStatus] =
    useCreateAssetStateMutation();
  const [appendVersion, appendState] = useAppendAssetVersionMutation();
  const [setAssetArchived, archiveState] = useSetAssetArchivedMutation();
  const [updateAsset, updateState] = useUpdateAssetMutation();
  const [loadRenameImpact, renameImpactState] = useAssetRenamePreflightMutation();
  const [renameAsset, renameState] = useRenameAssetMutation();
  const [loadDisableImpact, disableImpactState] = useAssetDisablePreflightMutation();
  const [disableAsset, disableAssetStatus] = useDisableAssetMutation();
  const [enableAsset, enableState] = useEnableAssetMutation();
  const [updateAssetState, updateAssetStateStatus] = useUpdateAssetStateMutation();
  const [loadStateImpact, stateImpactStatus] =
    useAssetStateDisablePreflightMutation();
  const [disableAssetState, disableAssetStateStatus] =
    useDisableAssetStateMutation();
  const [enableAssetState, enableAssetStateStatus] =
    useEnableAssetStateMutation();
  const [loadCurrentImpact, currentImpactState] =
    useCurrentAssetVersionPreflightMutation();
  const [setCurrentAssetVersion, currentVersionState] =
    useSetCurrentAssetVersionMutation();
  const [loadDeletePreflight, deletePreflightState] =
    useAssetDeletePreflightMutation();
  const [deleteAsset, deleteState] = useDeleteAssetMutation();
  const [createOpen, setCreateOpen] = useState(false);
  const [editOpen, setEditOpen] = useState(false);
  const [versionOpen, setVersionOpen] = useState(false);
  const [stateOpen, setStateOpen] = useState(false);
  const [stateEditOpen, setStateEditOpen] = useState(false);
  const [stateDisableOpen, setStateDisableOpen] = useState(false);
  const [stateImpact, setStateImpact] = useState<API.AssetImpactResponse>();
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [renameOpen, setRenameOpen] = useState(false);
  const [disableOpen, setDisableOpen] = useState(false);
  const [disableImpact, setDisableImpact] = useState<API.AssetImpactResponse>();
  const [currentOpen, setCurrentOpen] = useState(false);
  const [currentImpact, setCurrentImpact] = useState<API.AssetImpactResponse>();
  const [pendingVersion, setPendingVersion] = useState<API.AssetVersionResponse>();
  const [deletePreflight, setDeletePreflight] =
    useState<API.AssetDeletePreflightResponse>();
  const [notice, setNotice] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const mediaVersions = media.data?.items ?? [];
  const mediaById = new Map(mediaVersions.map((item) => [item.id, item]));
  const characterAssets = allAssets.filter(
    (asset) =>
      asset.kind === "character" &&
      asset.status === "active" &&
      asset.availability === "enabled",
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
      setSelectedStateId(null);
      setCreateOpen(false);
      setNotice(`资产身份已创建：${created.name}`);
      return true;
    } catch (error: unknown) {
      setActionError(appApiErrorMessage(error));
      return false;
    }
  }

  async function submitState(
    request: API.AssetStateCreateRequest,
  ): Promise<boolean> {
    if (!selectedAsset || !effectiveProject) return false;
    setActionError(null);
    try {
      const created = await createAssetState({
        projectId: effectiveProject.id,
        assetId: selectedAsset.id,
        body: request,
      }).unwrap();
      setSelectedStateId(created.state.id);
      setStateOpen(false);
      setNotice(`剧情状态已创建：${created.state.label}`);
      return true;
    } catch (error: unknown) {
      setActionError(appApiErrorMessage(error));
      return false;
    }
  }

  async function submitStateEdit(
    request: API.AssetStateUpdateRequest,
  ): Promise<boolean> {
    if (!selectedAsset || !selectedState || !effectiveProject) return false;
    setActionError(null);
    try {
      await updateAssetState({
        projectId: effectiveProject.id,
        assetId: selectedAsset.id,
        stateId: selectedState.id,
        body: request,
      }).unwrap();
      setStateEditOpen(false);
      setNotice("剧情状态信息已更新。");
      return true;
    } catch (error: unknown) {
      setActionError(appApiErrorMessage(error));
      return false;
    }
  }

  async function toggleStateAvailability() {
    if (!selectedAsset || !selectedState || !effectiveProject) return;
    setActionError(null);
    setNotice(null);
    if (selectedState.status === "disabled") {
      try {
        await enableAssetState({
          projectId: effectiveProject.id,
          assetId: selectedAsset.id,
          stateId: selectedState.id,
          body: {
            expected_revision: selectedState.revision,
            idempotency_key: `enable-asset-state:${crypto.randomUUID()}`,
          },
        }).unwrap();
        setNotice("剧情状态已启用。");
      } catch (error: unknown) {
        setActionError(appApiErrorMessage(error));
      }
      return;
    }
    setStateImpact(undefined);
    setStateDisableOpen(true);
    try {
      setStateImpact(
        await loadStateImpact({
          stateId: selectedState.id,
          body: { expected_revision: selectedState.revision },
        }).unwrap(),
      );
    } catch (error: unknown) {
      setActionError(appApiErrorMessage(error));
    }
  }

  async function confirmStateDisable() {
    if (!selectedAsset || !selectedState || !effectiveProject || !stateImpact) {
      return;
    }
    setActionError(null);
    try {
      await disableAssetState({
        projectId: effectiveProject.id,
        assetId: selectedAsset.id,
        stateId: selectedState.id,
        body: {
          expected_revision: selectedState.revision,
          impact_hash: stateImpact.impact_hash,
          idempotency_key: `disable-asset-state:${crypto.randomUUID()}`,
        },
      }).unwrap();
      setStateDisableOpen(false);
      setStateImpact(undefined);
      setNotice("剧情状态已停用；历史引用保持可追溯。");
    } catch (error: unknown) {
      setActionError(appApiErrorMessage(error));
    }
  }

  async function submitVersion(
    request: API.AssetVersionCreateRequest,
  ): Promise<boolean> {
    if (!selectedAsset || !selectedState || !effectiveProject) return false;
    setActionError(null);
    try {
      const created = await appendVersion({
        projectId: effectiveProject.id,
        assetId: selectedAsset.id,
        stateId: selectedState.id,
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

  async function preflightRename(
    newName: string,
  ): Promise<API.AssetImpactResponse | undefined> {
    if (!selectedAsset) return undefined;
    setActionError(null);
    try {
      return await loadRenameImpact({
        assetId: selectedAsset.id,
        body: {
          new_name: newName,
          expected_revision: selectedAsset.revision,
        },
      }).unwrap();
    } catch (error: unknown) {
      setActionError(appApiErrorMessage(error));
      return undefined;
    }
  }

  async function applyRename(
    newName: string,
    impact: API.AssetImpactResponse,
  ): Promise<boolean> {
    if (!selectedAsset || !effectiveProject) return false;
    setActionError(null);
    try {
      const result = await renameAsset({
        projectId: effectiveProject.id,
        assetId: selectedAsset.id,
        body: {
          new_name: newName,
          expected_revision: selectedAsset.revision,
          impact_hash: impact.impact_hash,
          idempotency_key: `rename-asset:${crypto.randomUUID()}`,
        },
      }).unwrap();
      setNotice(`资产已重命名为“${result.asset.name}”，旧名称已保留为别名。`);
      return true;
    } catch (error: unknown) {
      setActionError(appApiErrorMessage(error));
      return false;
    }
  }

  async function toggleAvailability() {
    if (!selectedAsset || !effectiveProject) return;
    setActionError(null);
    setNotice(null);
    if (selectedAsset.availability === "disabled") {
      try {
        await enableAsset({
          projectId: effectiveProject.id,
          assetId: selectedAsset.id,
          body: {
            expected_revision: selectedAsset.revision,
            idempotency_key: `enable-asset:${crypto.randomUUID()}`,
          },
        }).unwrap();
        setNotice("资产已启用，可重新用于新生产任务。");
      } catch (error: unknown) {
        setActionError(appApiErrorMessage(error));
      }
      return;
    }
    setDisableImpact(undefined);
    setDisableOpen(true);
    try {
      setDisableImpact(
        await loadDisableImpact({
          assetId: selectedAsset.id,
          body: { expected_revision: selectedAsset.revision },
        }).unwrap(),
      );
    } catch (error: unknown) {
      setActionError(appApiErrorMessage(error));
    }
  }

  async function confirmDisable() {
    if (!selectedAsset || !effectiveProject || !disableImpact) return;
    setActionError(null);
    try {
      await disableAsset({
        projectId: effectiveProject.id,
        assetId: selectedAsset.id,
        body: {
          expected_revision: selectedAsset.revision,
          impact_hash: disableImpact.impact_hash,
          idempotency_key: `disable-asset:${crypto.randomUUID()}`,
        },
      }).unwrap();
      setDisableOpen(false);
      setDisableImpact(undefined);
      setNotice("资产已停用；历史版本和引用保持可追溯。");
    } catch (error: unknown) {
      setActionError(appApiErrorMessage(error));
    }
  }

  async function selectCurrentVersion(version: API.AssetVersionResponse) {
    if (!selectedAsset || !selectedState || !effectiveProject) return;
    setActionError(null);
    setNotice(null);
    setPendingVersion(version);
    setCurrentImpact(undefined);
    setCurrentOpen(true);
    try {
      setCurrentImpact(await loadCurrentImpact({
        stateId: selectedState.id,
        body: {
          version_id: version.id,
          expected_current_version_id: selectedState.current_version_id,
          expected_revision: selectedState.revision,
        },
      }).unwrap());
    } catch (error: unknown) {
      setActionError(appApiErrorMessage(error));
    }
  }

  async function confirmCurrentVersion() {
    if (
      !selectedAsset ||
      !selectedState ||
      !effectiveProject ||
      !pendingVersion ||
      !currentImpact
    ) return;
    setActionError(null);
    try {
      await setCurrentAssetVersion({
        projectId: effectiveProject.id,
        stateId: selectedState.id,
        body: {
          version_id: pendingVersion.id,
          expected_current_version_id: selectedState.current_version_id,
          expected_revision: selectedState.revision,
          impact_hash: currentImpact.impact_hash,
          idempotency_key: `select-asset-version:${crypto.randomUUID()}`,
        },
      }).unwrap();
      setCurrentOpen(false);
      setCurrentImpact(undefined);
      await assetBible.refetch().unwrap();
      setNotice(`资产已切换到版本 v${pendingVersion.version_no}；既有镜头引用保持不变。`);
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
  const versionedCount = bibleItems.reduce(
    (total, { states }) =>
      total + states.filter(({ state }) => state.current_version_id).length,
    0,
  );
  const pageError =
    me.error ?? projects.error ?? assets.error ?? assetBible.error ?? media.error;

  return (
    <StudioShell
      active="assets"
      projectName={effectiveProject?.name}
    >
      {notice ? (
        <div
          className="pointer-events-none fixed top-24 right-6 z-50 flex items-center gap-2 bg-foreground px-4 py-3 text-sm text-background shadow-lg"
          role="status"
        >
          <CheckCircle2 className="size-4" aria-hidden="true" />
          {notice}
        </div>
      ) : null}

      <LayoutContainer className="py-8">
        <PageHeader
          actions={(
            <div className="flex flex-wrap items-end gap-2">
              {projectItems.length > 0 ? (
                <div className="grid min-w-56 gap-2">
                  <Label htmlFor="assetProject">当前项目</Label>
                  <Select
                    value={effectiveProject?.id ?? ""}
                    onValueChange={(value) => {
                      setSelectedProjectId(value);
                      setSelectedAssetId(null);
                      setSelectedStateId(null);
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
              {authenticated ? (
                effectiveProject?.status === "active" ? (
                  <Button className="h-10 bg-primary px-4 text-white hover:bg-primary/85" onClick={() => setCreateOpen(true)}>
                    <Plus aria-hidden="true" />新建资产
                  </Button>
                ) : (
                  <Button disabled className="h-10" variant="outline">项目已归档</Button>
                )
              ) : (
                <Button asChild className="h-10 bg-primary px-4 text-white hover:bg-primary/85"><Link href="/login">登录后管理</Link></Button>
              )}
            </div>
          )}
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

            <div
              aria-label="资产类型"
              className="mt-6 flex gap-1 overflow-x-auto bg-muted/45 p-1"
              role="tablist"
            >
              {assetTypes.map((item) => {
                const Icon = item.icon;
                const count = allAssets.filter(
                  (asset) => asset.kind === item.id,
                ).length;
                const active = selectedKind === item.id;
                return (
                  <Button
                    aria-selected={active}
                    className="min-w-24 flex-1 justify-center"
                    key={item.id}
                    role="tab"
                    size="sm"
                    type="button"
                    variant={active ? "secondary" : "ghost"}
                    onClick={() => {
                      setSelectedKind(item.id);
                      setSelectedAssetId(null);
                      setSelectedStateId(null);
                    }}
                  >
                    <Icon className="size-4" aria-hidden="true" />
                    {item.label}
                    <span className="text-xs text-muted-foreground">{count}</span>
                  </Button>
                );
              })}
            </div>

            <div className="mt-5 grid items-start gap-5 lg:grid-cols-[320px_minmax(0,1fr)]">
              <div className="grid gap-3">
                <div className="relative">
                  <Search
                    className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-slate-400"
                    aria-hidden="true"
                  />
                  <Input
                    aria-label="搜索资产"
                    className="bg-transparent pl-9"
                    onChange={(event) => setQuery(event.target.value)}
                    placeholder="搜索名称或别名"
                    value={query}
                  />
                </div>
                <AssetList
                  assets={visibleAssets}
                  statesByAssetId={statesByAssetId}
                  isLoading={assets.isLoading}
                  onSelect={(assetId) => {
                    setSelectedAssetId(assetId);
                    setSelectedStateId(null);
                  }}
                  selectedId={selectedAsset?.id}
                />
              </div>
              {selectedAsset && selectedState ? (
                <div className="grid gap-5">
                  <AssetStateBar
                    assetAvailability={selectedAsset.availability}
                    assetStatus={selectedAsset.status}
                    isChangingState={
                      disableAssetStateStatus.isLoading ||
                      enableAssetStateStatus.isLoading
                    }
                    onCreate={() => setStateOpen(true)}
                    onEdit={() => setStateEditOpen(true)}
                    onSelect={setSelectedStateId}
                    onToggleState={() => void toggleStateAvailability()}
                    selectedId={selectedState.id}
                    selectedState={selectedState}
                    states={selectedStates}
                  />
                  <AssetDetail
                    asset={selectedAsset}
                    currentState={selectedState}
                    isArchiving={archiveState.isLoading || assets.isFetching}
                    isChangingCurrent={currentVersionState.isLoading}
                    isChangingAvailability={
                      disableAssetStatus.isLoading || enableState.isLoading
                    }
                    mediaById={mediaById}
                    onAddVersion={() => setVersionOpen(true)}
                    onDelete={() => void prepareDelete()}
                    onEdit={() => setEditOpen(true)}
                    onRename={() => setRenameOpen(true)}
                    onSetCurrent={(version) => void selectCurrentVersion(version)}
                    onToggleArchive={toggleArchive}
                    onToggleAvailability={() => void toggleAvailability()}
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
                </div>
              ) : selectedAsset?.status === "archived" ? (
                <ArchivedAssetCard
                  asset={selectedAsset}
                  isRestoring={archiveState.isLoading || assets.isFetching}
                  onDelete={() => void prepareDelete()}
                  onEdit={() => setEditOpen(true)}
                  onRestore={() => void toggleArchive()}
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
      </LayoutContainer>

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
          <RenameAssetDialog
            asset={selectedAsset}
            isApplying={renameState.isLoading}
            isLoading={renameImpactState.isLoading}
            onApply={applyRename}
            onOpenChange={setRenameOpen}
            onPreflight={preflightRename}
            open={renameOpen}
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
          {selectedState ? (
            <>
              <CreateStateDialog
                asset={selectedAsset}
                isSubmitting={createAssetStateStatus.isLoading}
                onOpenChange={setStateOpen}
                onSubmit={submitState}
                open={stateOpen}
              />
              <EditStateDialog
                isSubmitting={updateAssetStateStatus.isLoading}
                onOpenChange={setStateEditOpen}
                onSubmit={submitStateEdit}
                open={stateEditOpen}
                state={selectedState}
              />
              <VersionDialog
                asset={selectedAsset}
                state={selectedState}
                characters={characterAssets}
                isSubmitting={appendState.isLoading}
                mediaVersions={mediaVersions}
                onOpenChange={setVersionOpen}
                onSubmit={submitVersion}
                open={versionOpen}
              />
            </>
          ) : null}
        </>
      ) : null}
      <AssetImpactDialog
        confirmLabel="确认停用"
        description="停用会阻止新的引用和生产任务，但不会删除历史版本、分镜或生成请求。"
        impact={disableImpact}
        isApplying={disableAssetStatus.isLoading}
        isLoading={disableImpactState.isLoading}
        onConfirm={confirmDisable}
        onOpenChange={(open) => {
          setDisableOpen(open);
          if (!open) setDisableImpact(undefined);
        }}
        open={disableOpen}
        title="确认停用资产"
      />
      <AssetImpactDialog
        confirmLabel="确认切换"
        description="只更新该剧情状态的当前版本；既有分镜继续固定到原资产版本。"
        impact={currentImpact}
        isApplying={currentVersionState.isLoading}
        isLoading={currentImpactState.isLoading}
        onConfirm={confirmCurrentVersion}
        onOpenChange={(open) => {
          setCurrentOpen(open);
          if (!open) {
            setCurrentImpact(undefined);
            setPendingVersion(undefined);
          }
        }}
        open={currentOpen}
        title="确认切换当前版本"
      />
      <AssetImpactDialog
        confirmLabel="确认停用"
        description="停用该剧情状态会阻止它参与新的资产绑定和生产任务，既有版本与引用不删除。"
        impact={stateImpact}
        isApplying={disableAssetStateStatus.isLoading}
        isLoading={stateImpactStatus.isLoading}
        onConfirm={confirmStateDisable}
        onOpenChange={(open) => {
          setStateDisableOpen(open);
          if (!open) setStateImpact(undefined);
        }}
        open={stateDisableOpen}
        title="确认停用剧情状态"
      />
    </StudioShell>
  );
}
