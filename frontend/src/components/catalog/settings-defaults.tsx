"use client";

import { useRef, useState } from "react";
import { Controller, useFieldArray, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import {
  useInfiniteQuery,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { z } from "zod";
import { saveProjectModelDefaults } from "@/gen/api/settings";
import { ApiError } from "@/lib/request";
import { PROJECTS_KEY } from "@/components/project/queries";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { AdminField, AdminMessage, AdminSelect } from "./admin-ui";
import { MODELS_KEY, queryModels, type Model } from "./queries";
import {
  PROJECT_DEFAULTS_KEY,
  preferenceError,
  projectDefaultsSchema,
  queryProjectModelDefaults,
  type ProjectModelDefaults,
} from "./settings-preferences-queries";

const schema = z
  .object({
    selections: z
      .array(z.object({ capability: z.string().min(1), model: z.string() }))
      .max(32),
  })
  .refine(
    (value) =>
      new Set(value.selections.map((item) => item.capability)).size ===
      value.selections.length,
  );
type Values = z.infer<typeof schema>;
const labels: Record<string, string> = {
  "image.generate": "图片生成",
  "video.generate": "视频生成",
  "text.generate": "文本生成",
  "audio.generate": "音频生成",
};
function rows(snapshot: ProjectModelDefaults, models: Model[]) {
  const capabilities = new Set([
    ...Object.keys(snapshot.default_models),
    ...models.map((model) => model.capability),
  ]);
  return [...capabilities]
    .sort()
    .slice(0, 32)
    .map((capability) => ({
      capability,
      model: snapshot.default_models[capability] ?? "",
    }));
}

export function ProjectDefaultsForm({
  initial,
  models,
  reload,
  saved,
}: {
  initial: ProjectModelDefaults;
  models: Model[];
  reload: () => Promise<ProjectModelDefaults>;
  saved: (value: ProjectModelDefaults) => void;
}) {
  const [snapshot, setSnapshot] = useState(() => initial);
  const [message, setMessage] = useState("");
  const [error, setError] = useState(false);
  const [confirmReload, setConfirmReload] = useState(false);
  const [reading, setReading] = useState(false);
  const request = useRef<{ fingerprint: string; key: string } | null>(null);
  const form = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: { selections: rows(initial, models) },
  });
  const fields = useFieldArray({ control: form.control, name: "selections" });
  const available = models.filter(
    (model) =>
      model.status === "active" &&
      model.provider_status === "active" &&
      model.current_version &&
      model.current_price,
  );
  const newCapabilities = [
    ...new Set(available.map((model) => model.capability)),
  ].filter(
    (capability) =>
      !fields.fields.some((field) => field.capability === capability),
  );

  async function refresh() {
    setReading(true);
    try {
      const latest = await reload();
      setSnapshot(latest);
      form.reset({ selections: rows(latest, models) });
      request.current = null;
      setMessage("已载入服务端最新默认模型。");
      setError(false);
      setConfirmReload(false);
    } catch (failure) {
      setMessage(preferenceError(failure));
      setError(true);
    } finally {
      setReading(false);
    }
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle>项目默认模型</CardTitle>
        <CardDescription>
          新建生成草稿时优先选择对应能力的默认模型。已有草稿和任务保留自己的选择。
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-6">
        <p className="text-xs text-muted-foreground">
          项目修订 {snapshot.revision} · 选择来自当前项目的模型目录
        </p>
        {initial.revision !== snapshot.revision && (
          <p role="status" className="text-sm text-muted-foreground">
            项目配置已有变化。保存仍使用编辑开始时的修订，重新读取前会保留当前输入。
          </p>
        )}
        <form
          className="space-y-5"
          onSubmit={(event) =>
            void form.handleSubmit(async (values) => {
              const defaultModels = Object.fromEntries(
                values.selections
                  .filter((item) => item.model)
                  .map((item) => [item.capability, item.model]),
              );
              const body = {
                expected_revision: snapshot.revision,
                default_models: defaultModels,
              };
              const fingerprint = JSON.stringify(body);
              if (
                !request.current ||
                request.current.fingerprint !== fingerprint
              )
                request.current = { fingerprint, key: crypto.randomUUID() };
              try {
                const result = projectDefaultsSchema.safeParse(
                  await saveProjectModelDefaults(
                    { pid: snapshot.project_id },
                    body,
                    { headers: { "Idempotency-Key": request.current.key } },
                  ),
                );
                if (
                  !result.success ||
                  result.data.project_id !== snapshot.project_id ||
                  result.data.revision !== snapshot.revision + 1 ||
                  JSON.stringify(
                    Object.entries(result.data.default_models).sort(),
                  ) !== JSON.stringify(Object.entries(defaultModels).sort())
                )
                  throw new ApiError(502, "invalid_response");
                setSnapshot(result.data);
                form.reset({ selections: rows(result.data, models) });
                request.current = null;
                setMessage("项目默认模型已保存。");
                setError(false);
                saved(result.data);
              } catch (failure) {
                setMessage(preferenceError(failure));
                setError(true);
              }
            })(event)
          }
        >
          {fields.fields.map((field, index) => (
            <AdminField
              key={field.id}
              id={`default-model-${index}`}
              label={labels[field.capability] ?? field.capability}
            >
              <Controller
                control={form.control}
                name={`selections.${index}.model`}
                render={({ field: choice }) => {
                  const options = available
                    .filter((model) => model.capability === field.capability)
                    .map((model) => ({
                      value: `model:${model.key}`,
                      label: `${model.display_name} · ${model.provider_name}`,
                    }));
                  if (
                    choice.value &&
                    !options.some(
                      (item) => item.value === `model:${choice.value}`,
                    )
                  )
                    options.push({
                      value: `model:${choice.value}`,
                      label: `${choice.value}（未在已载入目录中找到）`,
                    });
                  return (
                    <AdminSelect
                      id={`default-model-${index}`}
                      value={choice.value ? `model:${choice.value}` : "manual"}
                      onChange={(value) =>
                        choice.onChange(
                          value === "manual" ? "" : value.slice(6),
                        )
                      }
                      disabled={form.formState.isSubmitting || reading}
                      options={[
                        { value: "manual", label: "不设默认，手动选择" },
                        ...options,
                      ]}
                    />
                  );
                }}
              />
            </AdminField>
          ))}
          {!fields.fields.length && (
            <p className="text-sm text-muted-foreground">
              项目尚无可选模型。管理员发布模型后可设置默认选择。
            </p>
          )}
          {newCapabilities.length > 0 && fields.fields.length < 32 && (
            <div className="flex flex-wrap gap-2">
              {newCapabilities.map((capability) => (
                <Button
                  key={capability}
                  type="button"
                  variant="outline"
                  disabled={form.formState.isSubmitting}
                  onClick={() => fields.append({ capability, model: "" })}
                >
                  添加 {labels[capability] ?? capability} 默认选择
                </Button>
              ))}
            </div>
          )}
          <AdminMessage message={message} error={error} />
          <div className="flex flex-wrap gap-3">
            <Button
              type="submit"
              disabled={
                !form.formState.isDirty ||
                form.formState.isSubmitting ||
                reading
              }
            >
              {form.formState.isSubmitting ? "保存中…" : "保存项目默认模型"}
            </Button>
            <Button
              type="button"
              variant="outline"
              disabled={form.formState.isSubmitting || reading}
              onClick={() =>
                form.formState.isDirty ? setConfirmReload(true) : void refresh()
              }
            >
              {reading ? "读取中…" : "重新读取"}
            </Button>
          </div>
        </form>
        <Dialog open={confirmReload} onOpenChange={setConfirmReload}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>放弃当前默认模型修改？</DialogTitle>
              <DialogDescription>
                重新读取会用服务端保存的选择替换当前未保存输入。
              </DialogDescription>
            </DialogHeader>
            <DialogFooter>
              <Button variant="outline" onClick={() => setConfirmReload(false)}>
                继续编辑
              </Button>
              <Button disabled={reading} onClick={() => void refresh()}>
                放弃修改并读取
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </CardContent>
    </Card>
  );
}

export function SettingsDefaults({ projectId }: { projectId: string }) {
  const cache = useQueryClient();
  const defaults = useQuery({
    queryKey: [...PROJECT_DEFAULTS_KEY, projectId],
    queryFn: ({ signal }) => queryProjectModelDefaults(projectId, signal),
    retry: false,
  });
  const models = useInfiniteQuery({
    queryKey: [...MODELS_KEY, projectId, "settings"],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ signal, pageParam }) =>
      queryModels(projectId, undefined, signal, pageParam),
    getNextPageParam: (page) => page.next_cursor ?? undefined,
    retry: false,
  });
  if (defaults.isPending || models.isPending)
    return <p role="status">载入默认模型与项目目录…</p>;
  if (defaults.isError || models.isError)
    return (
      <div role="alert" className="space-y-3">
        <p>{preferenceError(defaults.error ?? models.error)}</p>
        <Button
          variant="outline"
          onClick={() => {
            void defaults.refetch();
            void models.refetch();
          }}
        >
          重新读取
        </Button>
      </div>
    );
  return (
    <div className="space-y-5">
      <ProjectDefaultsForm
        initial={defaults.data}
        models={models.data.pages.flatMap((page) => page.items)}
        reload={async () => {
          const result = await defaults.refetch();
          if (!result.data || result.error) throw result.error;
          return result.data;
        }}
        saved={(value) => {
          cache.setQueryData([...PROJECT_DEFAULTS_KEY, projectId], value);
          void cache.invalidateQueries({ queryKey: PROJECTS_KEY });
        }}
      />
      {models.hasNextPage && (
        <Button
          variant="outline"
          disabled={models.isFetchingNextPage}
          onClick={() => void models.fetchNextPage()}
        >
          加载更多可选模型
        </Button>
      )}
    </div>
  );
}
