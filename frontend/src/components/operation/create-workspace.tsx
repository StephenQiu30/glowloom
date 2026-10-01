"use client";

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { ModelParamsForm } from "@/components/catalog/model-params-form";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { MODELS_KEY, queryModels } from "@/components/catalog/queries";
import { queryProjectModelDefaults } from "@/components/catalog/settings-preferences-queries";
import { ProjectScope } from "@/components/workbench/project-scope";
import { QuoteConfirmDialog } from "./quote-confirm-dialog";
import {
  OPERATIONS_KEY,
  quoteGeneration,
  confirmGeneration,
  type GenerationItem,
} from "./queries";
import type { QuoteResponse } from "./use-quote";
import {
  createGenerationConfig,
  type GenerationConfig,
} from "@/components/canvas/generation-config";
import {
  loadGenerationDraft,
  createDraftCanvas,
  saveGenerationDraft,
  type DraftSelection,
  type DraftSaveRequest,
} from "./generation-drafts";
import { ReferenceInputs, generationReferenceRoles } from "./reference-inputs";

const modes = [
  { key: "image", label: "图片", capability: "image.generate" },
  { key: "image-edit", label: "图片编辑", capability: "image.edit" },
  { key: "video", label: "视频", capability: "video.generate" },
  { key: "audio", label: "音频", capability: "audio.tts" },
  { key: "text", label: "文本", capability: "text.structured" },
] as const;
export function CreateWorkspace() {
  const parameters = useSearchParams();
  const [kind, setKind] = useState(parameters.get("mode") ?? "image");
  const active = modes.find((item) => item.key === kind) ?? modes[0];
  return (
    <div className="mx-auto max-w-6xl space-y-8">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <h1 className="text-3xl font-medium tracking-tight">
            从一个想法开始。
          </h1>
          <p className="mt-3 text-sm text-muted-foreground">
            选择模型、填写参数，把结果带入你的故事。
          </p>
        </div>
        <Button variant="outline" asChild>
          <Link href="/canvas">打开无限画布</Link>
        </Button>
      </div>
      <div className="flex gap-2" aria-label="创作类型">
        {modes.map((item) => (
          <Button
            key={item.key}
            variant={active.key === item.key ? "default" : "ghost"}
            aria-pressed={active.key === item.key}
            onClick={() => setKind(item.key)}
          >
            {item.label}
          </Button>
        ))}
      </div>
      <ProjectScope>
        {(project) => (
          <GenerationForm
            key={`${project.id}:${active.key}`}
            projectId={project.id}
            capability={active.capability}
          />
        )}
      </ProjectScope>
    </div>
  );
}
function GenerationForm({
  projectId,
  capability,
}: {
  projectId: string;
  capability: string;
}) {
  const parameters = useSearchParams();
  const canvasId = parameters.get("canvas_id") ?? undefined,
    nodeId = parameters.get("node_id") ?? undefined;
  const [reloadVersion, setReloadVersion] = useState(0);
  const [reloadOpen, setReloadOpen] = useState(false);
  const saved = useQuery({
    queryKey: ["canvas", "form-draft", projectId, capability, canvasId, nodeId],
    queryFn: async ({ signal }) => {
      const [draft, defaults] = await Promise.all([
        loadGenerationDraft(projectId, capability, canvasId, nodeId, signal),
        queryProjectModelDefaults(projectId, signal),
      ]);
      return {
        ...draft,
        defaultModelKey: defaults.default_models[capability] ?? "",
      };
    },
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
    staleTime: Infinity,
  });
  if (saved.isPending) return <p role="status">正在恢复创作草稿…</p>;
  if (saved.isError)
    return (
      <div role="alert" className="space-y-3">
        <p>草稿或项目默认模型暂时无法读取。</p>
        <Button variant="outline" onClick={() => void saved.refetch()}>
          重新读取
        </Button>
      </div>
    );
  return (
    <>
      <GenerationSession
        key={`${projectId}:${capability}:${canvasId ?? ""}:${nodeId ?? ""}:${reloadVersion}`}
        projectId={projectId}
        capability={saved.data.config?.capability ?? capability}
        initial={saved.data}
        onReload={() => setReloadOpen(true)}
      />
      <Dialog open={reloadOpen} onOpenChange={setReloadOpen}>
        <DialogContent>
          <DialogTitle>重新读取服务器草稿</DialogTitle>
          <DialogDescription>
            这会放弃当前表单中尚未保存的修改，并读取服务器最近保存的内容。
          </DialogDescription>
          <DialogFooter>
            <Button variant="outline" onClick={() => setReloadOpen(false)}>
              继续编辑
            </Button>
            <Button
              disabled={saved.isFetching}
              onClick={() =>
                void saved.refetch().then((result) => {
                  if (result.isSuccess) {
                    setReloadVersion((value) => value + 1);
                    setReloadOpen(false);
                  }
                })
              }
            >
              放弃修改并重新读取
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
function GenerationSession({
  projectId,
  capability,
  initial,
  onReload,
}: {
  projectId: string;
  capability: string;
  initial: DraftSelection & { defaultModelKey?: string };
  onReload: () => void;
}) {
  const router = useRouter(),
    cache = useQueryClient();
  const alive = useRef(true);
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
    };
  }, []);
  const initialConfig = initial.config ?? createGenerationConfig(capability);
  const [modelId, setModelId] = useState(initialConfig.modelProfileId ?? "");
  const [defaultModelKey] = useState(initial.defaultModelKey ?? "");
  const [chosenMode, setChosenMode] = useState(initialConfig.mode);
  const [prompt, setPrompt] = useState(initialConfig.prompt);
  const [count, setCount] = useState(initialConfig.outputCount);
  const [inputs, setInputs] = useState(initialConfig.inputs);
  const [saving, setSaving] = useState(false),
    [savedNotice, setSavedNotice] = useState("");
  const [savedCanvasId, setSavedCanvasId] = useState(
    initial.document?.id ?? null,
  );
  const document = useRef(initial.document),
    draftId = useRef(initial.nodeId),
    createKey = useRef<string | null>(null),
    pendingSave = useRef<DraftSaveRequest | null>(null);
  const [savedConfig, setSavedConfig] = useState(initialConfig);
  const [error, setError] = useState<string | null>(null);
  const [quote, setQuote] = useState<QuoteResponse | null>(null);
  const lastRequest = useRef<{
    serialized: string;
    key: string;
    item: GenerationItem;
  } | null>(null);
  const confirmKey = useRef<string | null>(null);
  const catalog = useInfiniteQuery({
    queryKey: [...MODELS_KEY, projectId, capability],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) =>
      queryModels(projectId, capability, signal, pageParam),
    getNextPageParam: (page) => page.next_cursor ?? undefined,
    retry: false,
  });
  const models = catalog.data?.pages.flatMap((page) => page.items) ?? [];
  const selected =
    models.find((model) => model.id === modelId) ??
    (modelId
      ? undefined
      : defaultModelKey
        ? models.find((model) => model.key === defaultModelKey)
        : models[0]);
  const seekModel = Boolean(modelId || defaultModelKey) && !selected;
  const { hasNextPage, isFetchingNextPage, isError, fetchNextPage } = catalog;
  useEffect(() => {
    if (seekModel && hasNextPage && !isFetchingNextPage && !isError)
      void fetchNextPage();
  }, [seekModel, hasNextPage, isFetchingNextPage, isError, fetchNextPage]);
  const version = selected?.current_version;
  const mode = version?.modes.includes(chosenMode)
    ? chosenMode
    : (version?.modes[0] ?? "");
  const maxOutputs =
    typeof version?.limits.max_outputs === "number" &&
    Number.isInteger(version.limits.max_outputs)
      ? Math.min(8, version.limits.max_outputs)
      : 0;
  const quoteMutation = useMutation({
    mutationFn: ({ item, key }: { item: GenerationItem; key: string }) =>
      quoteGeneration(projectId, [item], key, ["本次创作"]),
    retry: false,
    onSuccess: (value) => {
      confirmKey.current = crypto.randomUUID();
      setQuote(value);
    },
  });
  async function persist(params: Record<string, string | number | boolean>) {
    if (saving || quoteMutation.isPending)
      throw new Error("请等待当前保存或报价完成。");
    setSaving(true);
    setSavedNotice("");
    try {
      const next: GenerationConfig = {
        version: 1,
        capability,
        modelProfileId: selected?.id ?? (modelId || null),
        mode,
        prompt,
        params,
        outputCount: count,
        inputs,
      };
      if (
        !pendingSave.current &&
        document.current &&
        draftId.current &&
        JSON.stringify(savedConfig) === JSON.stringify(next)
      ) {
        setSavedNotice("草稿已保存，可在画布中继续编辑。");
        return {
          canvas_id: document.current.id,
          node_id: draftId.current,
          revision: document.current.revision,
        };
      }
      if (!document.current) {
        createKey.current ??= crypto.randomUUID();
        document.current = await createDraftCanvas(
          projectId,
          createKey.current,
        );
      }
      if (
        pendingSave.current &&
        JSON.stringify(pendingSave.current.config) !== JSON.stringify(next)
      )
        throw new Error(
          "上次保存结果尚未确认。请先重新读取服务器草稿，再提交新修改。",
        );
      pendingSave.current ??= {
        canvasId: document.current.id,
        revision: document.current.revision,
        nodeId: draftId.current ?? crypto.randomUUID(),
        config: next,
        existing: !!draftId.current,
        key: crypto.randomUUID(),
      };
      const result = await saveGenerationDraft(projectId, pendingSave.current);
      if (!alive.current) throw new Error("工作区已切换，请从原项目继续。");
      document.current = result.document;
      draftId.current = result.nodeId;
      setSavedConfig(result.config);
      setSavedCanvasId(result.document.id);
      pendingSave.current = null;
      setSavedNotice("草稿已保存，可在画布中继续编辑。");
      return {
        canvas_id: result.document.id,
        node_id: result.nodeId,
        revision: result.document.revision,
      };
    } finally {
      if (alive.current) setSaving(false);
    }
  }
  async function submit(params: Record<string, string | number | boolean>) {
    if (!selected || !version || !mode || !prompt.trim()) {
      setError("请填写创作内容，并选择有可用版本的模型。");
      return;
    }
    const roles = generationReferenceRoles(
      selected.input_roles,
      version.limits,
    );
    if (
      inputs.some((input) => !roles.some((role) => role.name === input.role)) ||
      roles.some(
        (role) =>
          inputs.filter((input) => input.role === role.name).length >
          role.max_count,
      )
    ) {
      setError("请为每份参考素材选择当前模型支持的用途，并遵守数量上限。");
      return;
    }
    setError(null);
    let source;
    try {
      source = await persist(params);
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : "草稿保存失败。");
      return;
    }
    const item: GenerationItem = {
      source,
      model_key: selected.key,
      capability: selected.capability,
      mode,
      prompt: prompt.trim(),
      params,
      output_count: count,
      force_regenerate: false,
      media_inputs: inputs.map((input) => ({
        role: input.role,
        media_asset_id: input.mediaAssetId,
      })),
    };
    const serialized = JSON.stringify(item);
    const key =
      lastRequest.current?.serialized === serialized
        ? lastRequest.current.key
        : crypto.randomUUID();
    lastRequest.current = { serialized, key, item };
    await quoteMutation.mutateAsync({ item, key }).catch(() => {});
  }
  return (
    <div className="grid gap-10 lg:grid-cols-[minmax(0,1.25fr)_minmax(260px,0.75fr)]">
      <div className="space-y-6">
        <div className="space-y-3">
          <Label htmlFor="creation-prompt">创作内容</Label>
          <Textarea
            id="creation-prompt"
            value={prompt}
            onChange={(event) => setPrompt(event.target.value)}
            rows={8}
            maxLength={10000}
            placeholder="描述人物、场景、动作和你希望得到的画面…"
            className="resize-y text-base leading-7"
            disabled={saving || quoteMutation.isPending || !!quote}
          />
          <p className="text-xs text-muted-foreground">
            {prompt.length.toLocaleString()} / 10,000
          </p>
        </div>
        {version && selected && (
          <ReferenceInputs
            projectId={projectId}
            inputRoles={selected.input_roles}
            limits={version.limits}
            inputs={inputs}
            onChange={setInputs}
            disabled={saving || quoteMutation.isPending || !!quote}
          />
        )}
        {savedNotice && (
          <div className="space-y-2">
            <p role="status" className="text-sm text-muted-foreground">
              {savedNotice}
            </p>
            <Button asChild variant="outline">
              <Link
                href={`/projects/${projectId}/canvas?canvas=${savedCanvasId ?? ""}`}
              >
                在画布中继续
              </Link>
            </Button>
          </div>
        )}
        {(!selected || !version || !selected.current_price) && (
          <Button
            variant="outline"
            disabled={saving || quoteMutation.isPending || !!quote}
            onClick={() => {
              setError(null);
              void persist(
                Object.fromEntries(
                  Object.entries(savedConfig.params).filter(
                    (entry): entry is [string, string | number | boolean] =>
                      typeof entry[1] === "string" ||
                      typeof entry[1] === "number" ||
                      typeof entry[1] === "boolean",
                  ),
                ),
              ).catch((failure) =>
                setError(
                  failure instanceof Error ? failure.message : "草稿保存失败。",
                ),
              );
            }}
          >
            {saving ? "正在保存…" : "保存草稿"}
          </Button>
        )}
        {error && (
          <Button
            variant="outline"
            onClick={onReload}
            disabled={saving || quoteMutation.isPending}
          >
            重新读取服务器草稿
          </Button>
        )}
        <div className="rounded-xl bg-muted/40 p-5">
          <p className="text-sm leading-6 text-muted-foreground">
            报价后可以核对费用，再确认生成。结果会保存在当前项目的任务与素材中。
          </p>
          <Button variant="link" className="mt-2 px-0" asChild>
            <Link href={`/tasks?project_id=${projectId}`}>查看项目任务</Link>
          </Button>
        </div>
      </div>
      <div className="space-y-6">
        <div className="space-y-3">
          <Label htmlFor="creation-model">模型</Label>
          <select
            id="creation-model"
            value={selected?.id ?? modelId}
            onChange={(event) => {
              setModelId(event.target.value);
              setChosenMode("");
              setQuote(null);
            }}
            className="h-11 w-full rounded-lg border bg-background px-3 text-sm"
            disabled={
              catalog.isPending ||
              catalog.isError ||
              saving ||
              quoteMutation.isPending ||
              !!quote
            }
          >
            <option value="" disabled>
              选择模型
            </option>
            {models.map((model) => (
              <option key={model.id} value={model.id}>
                {model.display_name} · {model.provider_name}
              </option>
            ))}
          </select>
          {catalog.hasNextPage && (
            <Button
              variant="ghost"
              onClick={() => void catalog.fetchNextPage()}
              disabled={catalog.isFetchingNextPage}
            >
              加载更多模型
            </Button>
          )}
        </div>
        {catalog.isPending ||
        (seekModel && catalog.hasNextPage && !catalog.isError) ? (
          <p role="status" className="text-sm text-muted-foreground">
            正在读取模型…
          </p>
        ) : catalog.isError ? (
          <div role="alert" className="space-y-2">
            <p className="text-sm">模型目录读取失败。</p>
            <Button variant="outline" onClick={() => void catalog.refetch()}>
              重新读取
            </Button>
          </div>
        ) : !selected ? (
          <div className="space-y-3 rounded-lg bg-muted/40 p-5">
            <p className="text-sm text-muted-foreground">
              此项目尚无可用模型。
            </p>
            <Button variant="outline" asChild>
              <Link href={`/settings?project_id=${projectId}`}>
                查看模型配置
              </Link>
            </Button>
          </div>
        ) : !version || !selected.current_price ? (
          <p role="status" className="text-sm text-muted-foreground">
            此模型尚未发布版本或价格，暂时无法报价。
          </p>
        ) : (
          <>
            <div className="space-y-3">
              <Label htmlFor="creation-mode">模式</Label>
              <select
                id="creation-mode"
                value={mode}
                onChange={(event) => setChosenMode(event.target.value)}
                className="h-11 w-full rounded-lg border bg-background px-3 text-sm"
                disabled={saving || quoteMutation.isPending || !!quote}
              >
                {version.modes.map((value) => (
                  <option key={value} value={value}>
                    {value}
                  </option>
                ))}
              </select>
            </div>
            <div className="space-y-3">
              <Label htmlFor="creation-count">结果数量</Label>
              <input
                id="creation-count"
                type="number"
                min={1}
                max={maxOutputs}
                step={1}
                value={count}
                onChange={(event) => setCount(Number(event.target.value))}
                className="h-11 w-full rounded-lg border bg-background px-3 text-sm"
                disabled={saving || quoteMutation.isPending || !!quote}
              />
            </div>
            <ModelParamsForm
              modelKey={selected.key}
              profileVersionId={version.id}
              mode={mode}
              schema={version.param_schema}
              initialValues={Object.fromEntries(
                Object.entries(savedConfig.params).filter(
                  (entry): entry is [string, string | number | boolean] =>
                    typeof entry[1] === "string" ||
                    typeof entry[1] === "number" ||
                    typeof entry[1] === "boolean",
                ),
              )}
              disabled={
                quoteMutation.isPending ||
                saving ||
                !!quote ||
                !Number.isInteger(count) ||
                count < 1 ||
                count > maxOutputs
              }
              submitLabel={
                saving
                  ? "正在保存…"
                  : quoteMutation.isPending
                    ? "正在报价…"
                    : "保存并查看报价"
              }
              onSubmit={(params) => void submit(params)}
              onSaveDraft={(params) => {
                setError(null);
                void persist(params).catch((failure) =>
                  setError(
                    failure instanceof Error
                      ? failure.message
                      : "草稿保存失败。",
                  ),
                );
              }}
            />
          </>
        )}
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        {quoteMutation.isError && (
          <p role="alert" className="text-sm text-destructive">
            {quoteMutation.error.message}
          </p>
        )}
      </div>
      {quote && (
        <QuoteConfirmDialog
          open={!!quote}
          onOpenChange={(open) => {
            if (!open) setQuote(null);
          }}
          quote={quote}
          onRequote={() => {
            if (lastRequest.current) {
              const next = { ...lastRequest.current, key: crypto.randomUUID() };
              lastRequest.current = next;
              void quoteMutation
                .mutateAsync({ item: next.item, key: next.key })
                .catch(() => {});
            }
          }}
          onConfirm={async (excluded) => {
            if (!confirmKey.current) return;
            await confirmGeneration(
              projectId,
              quote,
              excluded,
              confirmKey.current,
            );
            await cache.invalidateQueries({
              queryKey: [...OPERATIONS_KEY, projectId],
            });
            if (alive.current) {
              setQuote(null);
              const id = quote.items.find(
                (item) =>
                  item.operation_id && !excluded.includes(item.operation_id),
              )?.operation_id;
              router.push(
                `/tasks?project_id=${projectId}${id ? `&task_id=${id}` : ""}`,
              );
            }
          }}
        />
      )}
    </div>
  );
}
