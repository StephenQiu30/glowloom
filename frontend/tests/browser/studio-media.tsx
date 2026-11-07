/** Fixture only: source components + generated API clients + real Axios HTTP to a local Vite backend. */
import React, { useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import { ThemeProvider } from "next-themes";
import { StudioComposer } from "../../src/app/short-drama/studio-composer";
import type { StudioInput } from "../../src/app/short-drama/studio-input";
import {
  StyleLibraryPopover,
  type StyleDetail,
} from "../../src/components/styles/style-library-popover";
import { SpaceContext } from "../../src/components/spaces/space-context";
import { ConversationProvider } from "../../src/components/conversations/conversation-context";
import { request, setCSRFToken, setCreationScope } from "../../src/lib/request";
import { TooltipProvider } from "../../src/components/ui/tooltip";
import { Toaster } from "../../src/components/ui/sonner";
import { Button } from "../../src/components/ui/button";
import { Tabs, TabsList, TabsTrigger } from "../../src/components/ui/tabs";
import { Checkbox } from "../../src/components/ui/checkbox";
import { Field, FieldLabel } from "../../src/components/ui/field";
import {
  NativeSelect,
  NativeSelectOption,
} from "../../src/components/ui/native-select";
import "../../src/app/globals.css";

const owner = {
  actorId: "11111111-1111-4111-8111-111111111111",
  scopeId: "22222222-2222-4222-8222-000000000002",
};
const space = {
  id: owner.scopeId,
  kind: "personal",
  team_id: "",
  name: "合成个人空间",
  role: "owner",
  writable: true,
};
setCSRFToken("studio-fixture-csrf");
setCreationScope(owner.scopeId);
type Controls = {
  listError: boolean;
  mediaError: boolean;
  detailError: boolean;
  unavailable: boolean;
  saveLost: boolean;
  conflict: boolean;
  deleteBound: boolean;
  linkMode: string;
  latency: number;
};
type Snapshot = {
  fixture: boolean;
  controls: Controls;
  receipts: number;
  styles: number;
  events: {
    sequence: number;
    method: string;
    path: string;
    key?: string;
    status: number;
    note?: string;
    body?: unknown;
  }[];
};
function Fixture() {
  const [mode, setMode] = useState<StudioInput["kind"]>("create");
  const [generation, setGeneration] = useState(0);
  const [selected, setSelected] = useState<Readonly<StyleDetail> | null>(null);
  const [inputs, setInputs] = useState<StudioInput[]>([]);
  const [snapshot, setSnapshot] = useState<Snapshot>();
  const [error, setError] = useState("");
  const [disabled, setDisabled] = useState(false);
  useEffect(() => {
    let active = true;
    const poll = () =>
      request<Snapshot>("/api/fixture/studio-media")
        .then((data) => {
          if (active) setSnapshot(data);
        })
        .catch((e) => {
          if (active) setError(String(e));
        });
    void poll();
    const timer = setInterval(poll, 1000);
    return () => {
      active = false;
      clearInterval(timer);
    };
  }, []);
  async function control(update: Partial<Controls> & { reset?: boolean }) {
    try {
      const data = await request<Snapshot>("/api/fixture/studio-media", {
        method: "POST",
        data: update,
      });
      setSnapshot(data);
      setError("");
      if (update.reset) {
        setGeneration((n) => n + 1);
        setInputs([]);
        setSelected(null);
      }
    } catch (e) {
      setError(String(e));
    }
  }
  return (
    <main className="mx-auto min-h-dvh max-w-7xl space-y-6 p-3 md:p-8">
      <header className="space-y-2 rounded-xl border border-amber-700/50 bg-amber-950/20 p-4">
        <h1 className="text-xl font-semibold">
          短剧素材与风格 · 浏览器验收夹具
        </h1>
        <p className="text-sm text-muted-foreground">
          FIXTURE / 合成账号与数据。真实
          StudioComposer、StyleLibraryPopover、Dialog、个人库和 API/axios；本地
          HTTP 回执；无供应商、无真实计费。
        </p>
        <p className="text-xs text-muted-foreground">
          “开始”仅记录 input
          JSON。分析入口可准备独立上下文与合成复制，但模型列表为空、执行接口
          fail closed，不发送生成请求。刷新浏览器保留服务器回执，重启夹具清空。
        </p>
      </header>
      <section className="space-y-3" aria-label="真实短剧组件">
        <div className="flex flex-wrap items-center gap-3">
          <Tabs
            value={mode}
            onValueChange={(v) => setMode(v as StudioInput["kind"])}
          >
            <TabsList>
              <TabsTrigger value="create">剧本创作</TabsTrigger>
              <TabsTrigger value="upload">上传剧本</TabsTrigger>
              <TabsTrigger value="redraw">转绘</TabsTrigger>
            </TabsList>
          </Tabs>
          <Field orientation="horizontal" className="w-auto">
            <Checkbox
              id="fixture-disabled"
              checked={disabled}
              onCheckedChange={(v) => setDisabled(v === true)}
            />
            <FieldLabel htmlFor="fixture-disabled">只读 / 禁用</FieldLabel>
          </Field>
          <Button
            variant="outline"
            onClick={() => void control({ reset: true })}
          >
            重置合成数据
          </Button>
        </div>
        <StudioComposer
          key={`${generation}/${mode}`}
          mode={mode}
          disabled={disabled}
          owner={owner}
          onStart={(input) => setInputs((old) => [...old, input])}
        />
        <div className="flex flex-wrap items-center gap-3 rounded-xl border p-3">
          <span className="text-sm">独立风格入口（便于单独验收）</span>
          <StyleLibraryPopover
            key={generation}
            owner={owner}
            value={selected}
            disabled={disabled}
            onChange={setSelected}
          />
          <span role="status" className="text-sm text-muted-foreground">
            {selected
              ? `已选 ${selected.name} · revision ${selected.revision}`
              : "未选风格"}
          </span>
        </div>
      </section>
      <details className="rounded-xl border p-4" open>
        <summary className="cursor-pointer font-medium">
          受控状态与错误（fixture 控制台）
        </summary>
        <div className="mt-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          {(
            [
              ["listError", "风格列表 503"],
              ["mediaError", "个人素材库 503"],
              ["detailError", "详情 503"],
              ["unavailable", "全部风格参考图失效"],
              ["saveLost", "下一次写入成功但丢回执"],
              ["conflict", "下一次编辑 CAS 冲突"],
              ["deleteBound", "删除被绑定风格 409"],
            ] as const
          ).map(([key, label]) => (
            <Field key={key} orientation="horizontal">
              <Checkbox
                id={`fixture-${key}`}
                checked={snapshot?.controls[key] ?? false}
                onCheckedChange={(v) => void control({ [key]: v === true })}
              />
              <FieldLabel htmlFor={`fixture-${key}`}>{label}</FieldLabel>
            </Field>
          ))}
          <Field>
            <FieldLabel htmlFor="fixture-link">视频链接响应</FieldLabel>
            <NativeSelect
              id="fixture-link"
              value={snapshot?.controls.linkMode ?? "reject"}
              onChange={(e) => void control({ linkMode: e.target.value })}
            >
              <NativeSelectOption value="reject">
                不可解析 422
              </NativeSelectOption>
              <NativeSelectOption value="success">
                导入合成视频成功
              </NativeSelectOption>
              <NativeSelectOption value="lost">
                成功后丢回执 / 原 key 重试
              </NativeSelectOption>
            </NativeSelect>
          </Field>
          <Field>
            <FieldLabel htmlFor="fixture-delay">响应延时</FieldLabel>
            <NativeSelect
              id="fixture-delay"
              value={snapshot?.controls.latency ?? 200}
              onChange={(e) =>
                void control({ latency: Number(e.target.value) })
              }
            >
              <NativeSelectOption value={0}>即时</NativeSelectOption>
              <NativeSelectOption value={200}>200 ms</NativeSelectOption>
              <NativeSelectOption value={1800}>
                1800 ms（加载态）
              </NativeSelectOption>
            </NativeSelect>
          </Field>
        </div>
        <p className="mt-3 text-xs text-muted-foreground">
          开关生效后关闭并重开列表刷新；编辑冲突仅下一次；丢回执保留 key 和
          payload，点击“重试原保存”应返回
          REPLAY。失效参考可在“我的风格”编辑移除修复。链接示例：https://www.douyin.com/video/fixture（本地响应，不访问该地址）。
        </p>
        <p role="status" className="mt-2 text-xs">
          服务器风格 {snapshot?.styles ?? "…"} · 已记幂等回执{" "}
          {snapshot?.receipts ?? "…"}
        </p>
      </details>
      {error && <p role="alert">{error}</p>}
      <section className="grid min-w-0 gap-4 lg:grid-cols-2">
        <details className="min-w-0 rounded-xl border p-4" open>
          <summary className="font-medium">
            开始操作传出的 input JSON（不提交生成）· {inputs.length}
          </summary>
          <pre className="mt-3 max-h-96 overflow-auto rounded bg-muted p-3 text-xs break-all whitespace-pre-wrap">
            {JSON.stringify(
              inputs,
              (_key, value) =>
                value instanceof File
                  ? {
                      fixture_file: true,
                      name: value.name,
                      type: value.type,
                      size: value.size,
                    }
                  : value,
              2,
            )}
          </pre>
        </details>
        <details className="min-w-0 rounded-xl border p-4" open>
          <summary className="font-medium">选中风格真实详情快照</summary>
          <pre className="mt-3 max-h-96 overflow-auto rounded bg-muted p-3 text-xs break-all whitespace-pre-wrap">
            {JSON.stringify(selected, null, 2)}
          </pre>
        </details>
      </section>
      <details className="rounded-xl border p-4" open>
        <summary className="font-medium">
          HTTP 请求 / 回执日志（最多 100 条）
        </summary>
        <div className="mt-3 max-h-96 space-y-2 overflow-auto text-xs">
          {[...(snapshot?.events ?? [])].reverse().map((e) => (
            <div key={e.sequence} className="rounded bg-muted p-2 break-all">
              <p>
                #{e.sequence} {e.method} {e.path} → {e.status}
              </p>
              {e.key && <p>Idempotency-Key: {e.key}</p>}
              {e.note && <p className="text-amber-400">{e.note}</p>}
              {e.body !== undefined && (
                <pre className="whitespace-pre-wrap">
                  {JSON.stringify(e.body, null, 2)}
                </pre>
              )}
            </div>
          ))}
        </div>
      </details>
      <footer className="pb-6 text-xs text-muted-foreground">
        验收边界：此页能证明真实组件交互与 HTTP
        合同，不代表真实服务持久化、供应商解析或付费模型验收。所有未实现 /api
        路径均返回 fixture_route_not_allowed 404。
      </footer>
    </main>
  );
}
createRoot(document.getElementById("root")!).render(
  <ThemeProvider attribute="class" defaultTheme="dark" forcedTheme="dark">
    <TooltipProvider>
      <SpaceContext.Provider
        value={{
          spaces: [space],
          selected: space,
          pending: false,
          error: "",
          refresh: async () => {},
          select: async () => {},
        }}
      >
        <ConversationProvider>
          <Fixture />
        </ConversationProvider>
      </SpaceContext.Provider>
      <Toaster />
    </TooltipProvider>
  </ThemeProvider>,
);
