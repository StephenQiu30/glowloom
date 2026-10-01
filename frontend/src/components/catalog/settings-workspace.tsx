"use client";

import { useState } from "react";
import { useInfiniteQuery } from "@tanstack/react-query";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@/components/ui/empty";
import { ProjectScope } from "@/components/workbench/project-scope";
import { MODELS_KEY, queryModels } from "./queries";
import { AdminFailure } from "./admin-ui";
import { SettingsModels } from "./settings-models";
import { SettingsProviders } from "./settings-providers";
import { SettingsDefaults } from "./settings-defaults";
import { SettingsPrompts } from "./settings-prompts";

function ProjectModels({ projectId }: { projectId: string }) {
  const [search, setSearch] = useState("");
  const query = useInfiniteQuery({
    queryKey: [...MODELS_KEY, projectId, "settings"],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ signal, pageParam }) =>
      queryModels(projectId, undefined, signal, pageParam),
    getNextPageParam: (page) => page.next_cursor ?? undefined,
    retry: false,
  });
  if (query.isPending) return <p role="status">载入项目模型…</p>;
  if (query.isError)
    return (
      <AdminFailure error={query.error} retry={() => void query.refetch()} />
    );
  const all = query.data.pages.flatMap((page) => page.items);
  const items = all.filter((model) =>
    `${model.display_name} ${model.key} ${model.capability} ${model.provider_name}`
      .toLowerCase()
      .includes(search.trim().toLowerCase()),
  );
  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <p className="text-sm text-muted-foreground">
          项目当前可见的模型与生成配置。已载入 {all.length} 个模型。
        </p>
        <Input
          aria-label="搜索已载入模型"
          placeholder="搜索已载入模型"
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          className="max-w-sm"
        />
      </div>
      <div className="grid gap-4 md:grid-cols-2 2xl:grid-cols-3">
        {items.map((model) => (
          <Card key={model.id}>
            <CardHeader>
              <div className="flex items-start justify-between gap-3">
                <CardTitle>{model.display_name}</CardTitle>
                <Badge variant="outline">
                  {model.status === "active" &&
                  model.provider_status === "active"
                    ? "启用"
                    : "禁用"}
                </Badge>
              </div>
              <CardDescription>
                {model.provider_name} · {model.capability}
                <br />
                {model.key}
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-3 text-sm">
              <p>
                配置版本：
                {model.current_version
                  ? `v${model.current_version.version_no}`
                  : "尚未发布"}
              </p>
              <p>
                模式：{model.current_version?.modes.join("、") || "尚未配置"}
              </p>
              <p>参考输入：{model.input_roles.join("、") || "无"}</p>
              <p>
                凭据：{model.credential_present ? "已配置" : "未配置"}
                {model.credential_test_result
                  ? ` · ${model.credential_test_result}`
                  : ""}
              </p>
              <p>
                价格：
                {model.current_price
                  ? `v${model.current_price.version_no} · ${model.current_price.currency} / ${model.current_price.unit}`
                  : "尚无生效价格"}
              </p>
              {model.current_version && (
                <details>
                  <summary className="cursor-pointer text-muted-foreground">
                    查看生成配置
                  </summary>
                  <pre className="mt-3 max-h-64 overflow-auto rounded-lg bg-muted p-3 text-xs">
                    {JSON.stringify(
                      {
                        limits: model.current_version.limits,
                        parameters: model.current_version.param_schema,
                      },
                      null,
                      2,
                    )}
                  </pre>
                </details>
              )}
              {model.current_price && (
                <details>
                  <summary className="cursor-pointer text-muted-foreground">
                    查看价格规则
                  </summary>
                  <pre className="mt-3 max-h-64 overflow-auto rounded-lg bg-muted p-3 text-xs">
                    {JSON.stringify(model.current_price.rule, null, 2)}
                  </pre>
                </details>
              )}
            </CardContent>
          </Card>
        ))}
      </div>
      {!items.length && (
        <Empty>
          <EmptyHeader>
            <EmptyTitle>
              {all.length ? "没有匹配的模型" : "项目尚无可见模型"}
            </EmptyTitle>
            <EmptyDescription>
              {all.length
                ? "调整关键词，或载入下一页模型。"
                : "管理员发布并启用模型后，目录会展示实际配置。"}
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      )}
      {query.hasNextPage && (
        <Button
          variant="outline"
          disabled={query.isFetchingNextPage}
          onClick={() => void query.fetchNextPage()}
        >
          加载更多模型
        </Button>
      )}
    </div>
  );
}
export function SettingsWorkspace() {
  const [tab, setTab] = useState("project");
  return (
    <div className="space-y-8">
      <header className="space-y-3">
        <p className="text-xs tracking-widest text-muted-foreground">
          MODEL SETTINGS
        </p>
        <h1 className="text-3xl font-semibold tracking-tight">创作设置</h1>
        <p className="max-w-2xl text-sm leading-6 text-muted-foreground">
          设置项目默认模型与个人创作要求，查看模型目录和渠道配置。生成任务使用创建时冻结的配置和价格。
        </p>
      </header>
      <Tabs value={tab} onValueChange={setTab}>
        <div className="mb-6 max-w-full overflow-x-auto">
          <TabsList className="w-max">
            <TabsTrigger value="project">项目模型</TabsTrigger>
            <TabsTrigger value="defaults">默认模型</TabsTrigger>
            <TabsTrigger value="prompts">提示词偏好</TabsTrigger>
            <TabsTrigger value="providers">渠道管理</TabsTrigger>
            <TabsTrigger value="models">模型管理</TabsTrigger>
          </TabsList>
        </div>
        <div hidden={tab !== "project" && tab !== "defaults"}>
          <ProjectScope>
            {(project) => (
              <>
                <TabsContent
                  value="project"
                  forceMount
                  className="data-[state=inactive]:hidden"
                >
                  <ProjectModels projectId={project.id} />
                </TabsContent>
                <TabsContent
                  value="defaults"
                  forceMount
                  className="data-[state=inactive]:hidden"
                >
                  <SettingsDefaults projectId={project.id} />
                </TabsContent>
              </>
            )}
          </ProjectScope>
        </div>
        <TabsContent value="providers">
          <SettingsProviders />
        </TabsContent>
        <TabsContent value="models">
          <SettingsModels />
        </TabsContent>
        <TabsContent
          value="prompts"
          forceMount
          className="data-[state=inactive]:hidden"
        >
          <SettingsPrompts />
        </TabsContent>
      </Tabs>
    </div>
  );
}
