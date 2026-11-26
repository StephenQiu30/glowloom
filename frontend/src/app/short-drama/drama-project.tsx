"use client";

import Link from "next/link";
import dynamic from "next/dynamic";
import useSWR from "swr";
import { useEffect, useState } from "react";
import { getProject } from "@/api/projects";
import { getScriptWorkspace } from "@/api/script";
import { useOptionalSession } from "@/components/auth/session";
import { useLoginDialog } from "@/components/layout/basic-layout";
import { PageHeader } from "@/components/layout/page-header";
import { ContentState } from "@/components/layout/content-state";
import { canvasHref, requireProject } from "@/components/projects/project";
import { useSpace } from "@/components/spaces/space-context";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { RequestError } from "@/lib/request";
import { ScriptPanel } from "./script-panel";
import { ProductionAssets } from "./production-assets";
import { VideoPanel } from "./video-panel";
import { clearStudioInput, readStudioInput } from "./studio-input";
import { createStudioDraft } from "./studio-draft";
import { useStudioReferenceCopy } from "./use-studio-reference-copy";

const ConversationWorkspace = dynamic(
  () =>
    import("@/components/conversations/conversation-workspace").then(
      (m) => m.ConversationWorkspace,
    ),
  {
    loading: () => <p role="status">正在打开短剧 Agent…</p>,
  },
);

type Tab = "script" | "create" | "assets" | "video";

export function DramaProject(props: {
  projectID: string;
  initialTab: Tab;
  initialSession?: string;
}) {
  const session = useOptionalSession();
  const space = useSpace();
  const login = useLoginDialog();
  if (!session)
    return (
      <ContentState
        title="登录后打开短剧项目"
        actions={<Button onClick={login}>登录 / 注册</Button>}
      />
    );
  if (space.pending || !space.selected)
    return (
      <ContentState
        state="loading"
        loadingLayout="conversation"
        title="正在核对项目空间"
      />
    );
  const owner = `${session.actor.id}/${space.selected.id}`;
  return (
    <ProjectWorkspace
      key={`${owner}/${props.projectID}`}
      {...props}
      owner={owner}
      writable={space.selected.writable}
    />
  );
}

function ProjectWorkspace({
  projectID,
  initialTab,
  initialSession,
  owner,
  writable,
}: {
  projectID: string;
  initialTab: Tab;
  initialSession?: string;
  owner: string;
  writable: boolean;
}) {
  const [studioInput] = useState(() => readStudioInput(owner, projectID));
  useEffect(() => {
    if (studioInput) clearStudioInput(studioInput);
  }, [studioInput]);
  const [tab, setTab] = useState(initialTab);
  const [sessionID, setSessionID] = useState(
    initialTab === "video" ? undefined : initialSession,
  );
  const [generated, setGenerated] = useState("");
  const [scriptLocked, setScriptLocked] = useState(false);
  const [agentLocked, setAgentLocked] = useState(false);
  const [videoLocked, setVideoLocked] = useState(false);
  const [assetsLocked, setAssetsLocked] = useState(false);
  const locked = scriptLocked || agentLocked || videoLocked || assetsLocked;
  const project = useSWR(
    [owner, projectID, "short-drama-project"],
    async () => {
      const value = requireProject(await getProject({ pid: projectID }), true);
      if (
        value.id !== projectID ||
        value.is_delete ||
        value.status === "copying"
      )
        throw new RequestError("invalid_receipt");
      return value;
    },
  );
  const script = useSWR([owner, projectID, "script-workspace"], async () => {
    const response = await getScriptWorkspace({ pid: projectID });
    if (
      response.state?.project_id !== projectID ||
      !Number.isSafeInteger(response.state.revision) ||
      response.state.revision! < 0
    )
      throw new RequestError("invalid_receipt");
    return response.state;
  });
  const [draft, setDraft] = useState(() => createStudioDraft(studioInput));
  const referenceCopy = useStudioReferenceCopy(
    studioInput,
    projectID,
    owner,
    writable && Boolean(project.data) && project.data?.status !== "archived",
    (assets) => setDraft(createStudioDraft(studioInput, assets)),
  );
  if (
    accessDenied(project.error) ||
    accessDenied(script.error) ||
    (project.error && !project.data) ||
    (script.error && !script.data)
  )
    return (
      <ContentState
        state="error"
        title="无法打开短剧项目"
        actions={
          <Button
            onClick={() => {
              void project.mutate();
              void script.mutate();
            }}
          >
            重试
          </Button>
        }
      />
    );
  if (!project.data || !script.data)
    return (
      <ContentState
        state="loading"
        loadingLayout="conversation"
        title="正在读取短剧项目"
      />
    );
  if (
    !referenceCopy.assets &&
    (!writable || project.data.status === "archived")
  )
    return (
      <ContentState state="error" title="当前项目不可写，无法接收参考素材" />
    );
  if (!referenceCopy.assets)
    return (
      <ContentState
        state={referenceCopy.error ? "error" : "loading"}
        title={referenceCopy.error || "正在把参考素材保存到本项目"}
        actions={
          referenceCopy.error ? (
            <>
              <Button onClick={referenceCopy.retry}>重试原复制</Button>
              <Button asChild variant="outline">
                <Link href="/assets?view=reference">查看资产转移记录</Link>
              </Button>
            </>
          ) : undefined
        }
      />
    );

  const canvas = canvasHref(project.data);
  const canWrite = writable && project.data.status !== "archived";
  function selectTab(next: Tab) {
    setTab(next);
    const id = next === "create" ? sessionID : undefined;
    const query = new URLSearchParams({ tab: next });
    if (id) query.set("session", id);
    window.history.replaceState(null, "", `/short-drama/${projectID}?${query}`);
  }
  const activeTab = tab;
  return (
    <div className="mx-auto flex w-full max-w-7xl min-w-0 flex-col gap-5 pb-8">
      <PageHeader title={project.data.name}>
        <Button variant="ghost" asChild>
          <Link href="/short-drama">短剧首页</Link>
        </Button>
        <Button variant="outline" asChild>
          <Link href={canvas}>打开短剧画布</Link>
        </Button>
      </PageHeader>
      <Tabs
        className="min-w-0"
        value={activeTab}
        onValueChange={(value) => {
          if (!locked) selectTab(value as Tab);
        }}
      >
        <TabsList variant="secondary" aria-label="短剧项目阶段">
          <TabsTrigger value="script" disabled={locked}>
            剧本
          </TabsTrigger>
          <TabsTrigger value="create" disabled={locked}>
            剧本创作
          </TabsTrigger>
          <TabsTrigger value="assets" disabled={locked}>
            制作资产库
          </TabsTrigger>
          <TabsTrigger value="video" disabled={locked}>
            转绘与后期
          </TabsTrigger>
        </TabsList>
        <TabsContent value="script" forceMount hidden={activeTab !== "script"}>
          <ScriptPanel
            projectID={projectID}
            owner={owner}
            head={script.data}
            writable={canWrite}
            generatedText={generated}
            initialInput={
              studioInput?.kind === "upload" ? studioInput : undefined
            }
            onChange={script.mutate}
            onLock={setScriptLocked}
          />
        </TabsContent>
        <TabsContent value="create">
          <h2 className="mb-2 font-semibold">剧本创作</h2>
          <p className="mb-3 text-sm text-muted-foreground">
            写下创作想法，选择模型与预算。回复完成后可以带入剧本草稿审阅。
            {draft.references.some(
              (reference) => reference.mediaKind === "video",
            ) && " 视频参考将读取带时间戳的采样画面，不包含完整音轨理解。"}
          </p>
          <ConversationWorkspace
            embedded
            ownerKey={owner}
            projectID={projectID}
            sessionID={sessionID}
            enabled={canWrite}
            initialDraft={draft}
            onDraftChange={setDraft}
            onChooseProject={noProjectChange}
            onLock={setAgentLocked}
            onAccepted={(id) => {
              setSessionID(id);
              window.history.replaceState(
                null,
                "",
                `/short-drama/${projectID}?tab=create&session=${id}`,
              );
            }}
            onUseReply={
              canWrite
                ? (text) => {
                    setGenerated(text);
                    selectTab("script");
                  }
                : undefined
            }
          />
        </TabsContent>
        <TabsContent value="assets">
          <ProductionAssets
            projectID={projectID}
            owner={owner}
            writable={canWrite}
            canvas={canvas}
            onLock={setAssetsLocked}
          />
        </TabsContent>
        <TabsContent value="video" forceMount hidden={activeTab !== "video"}>
          <VideoPanel
            owner={owner}
            project={project.data}
            writable={canWrite}
            canvas={canvas}
            initialInput={
              studioInput?.kind === "redraw" ? studioInput : undefined
            }
            styleAssets={referenceCopy.assets.filter(
              (asset) => asset.kind === "image",
            )}
            onLock={setVideoLocked}
          />
        </TabsContent>
      </Tabs>
    </div>
  );
}

const noProjectChange = () => {};
function accessDenied(error: unknown) {
  return (
    error instanceof RequestError && [401, 403, 404].includes(error.status)
  );
}
