"use client";
import { useRef, useState, useSyncExternalStore } from "react";
import dynamic from "next/dynamic";
import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { getProject } from "@/components/project/queries";
import { BibleList } from "./bible-list";
import { BibleDetail } from "./bible-detail";
import type { BibleEditFrame } from "./bible-entry-dialog";
import {
  bibleContextKey,
  bibleScopeKey,
  getBibleDetail,
  listBible,
} from "./bible-queries";
import {
  bibleKind,
  bibleUUID,
  kindLabels,
  versionInput,
  type BibleDetail as Detail,
  type BibleIdentity,
  type BibleKind,
  type BibleReceipt,
} from "./bible-model";
import { useBibleWriter } from "./use-bible-writer";
import type { BibleActionFrame } from "./bible-action-dialog";
import type { BibleLookFrame } from "./bible-look-dialog";
const subscribeOrigin = () => () => {};
const BibleWriteDialog = dynamic(
  () =>
    import("./bible-entry-dialog").then((module) => module.BibleWriteDialog),
  { ssr: false },
);
const EntryDialog = dynamic(
  () =>
    import("./bible-entry-dialog").then((module) => module.BibleEntryDialog),
  { ssr: false },
);
const LookDialog = dynamic(
  () => import("./bible-look-dialog").then((module) => module.BibleLookDialog),
  { ssr: false },
);
const VoiceDialog = dynamic(
  () =>
    import("./bible-voice-dialog").then((module) => module.BibleVoiceDialog),
  { ssr: false },
);
const ActionDialog = dynamic(
  () =>
    import("./bible-action-dialog").then((module) => module.BibleActionDialog),
  { ssr: false },
);
const ResultDialog = dynamic(
  () =>
    import("./bible-result-dialog").then((module) => module.BibleResultDialog),
  { ssr: false },
);
const HistoryDialog = dynamic(
  () => import("./bible-history").then((module) => module.BibleHistoryDialog),
  { ssr: false },
);
type Modal =
  | { type: "entry"; frame: BibleEditFrame }
  | { type: "look"; frame: BibleLookFrame }
  | { type: "voice"; detail: Detail }
  | { type: "action"; frame: BibleActionFrame }
  | { type: "result"; kind: BibleKind; id?: string; revision: number }
  | { type: "history"; id: string; kind: BibleKind };
export function ProjectBibleWorkspace({
  projectId: input,
}: {
  projectId: string;
}) {
  const parsed = bibleUUID.safeParse(input),
    projectId = parsed.success ? parsed.data.toLowerCase() : input;
  const origin = useSyncExternalStore(
    subscribeOrigin,
    () => window.location.origin,
    () => null,
  );
  const project = useQuery({
    queryKey: ["project", projectId],
    queryFn: ({ signal }) => getProject(projectId, signal),
    enabled: parsed.success,
    staleTime: 0,
    retry: false,
  });
  const context = useQuery({
    queryKey: bibleContextKey(origin ?? "", projectId),
    queryFn: ({ signal }) =>
      listBible(projectId, "character", { limit: 1 }, signal),
    enabled: parsed.success && Boolean(origin),
    staleTime: 0,
    gcTime: 0,
    retry: false,
  });
  if (!parsed.success) return <p role="alert">项目UUID无效。</p>;
  const proven =
    Boolean(origin) &&
    project.isSuccess &&
    context.isSuccess &&
    context.isFetchedAfterMount;
  return (
    <section className="mx-auto flex w-full max-w-7xl flex-col gap-5">
      <h1 className="text-xl font-semibold wrap-anywhere">
        {project.data?.name ?? "项目"} · 设定集
      </h1>
      <p className="text-sm text-muted-foreground">
        管理角色、场景和道具的完整定义、正式造型与不可变历史。
      </p>
      {(project.isError || context.isError) && (
        <div>
          <p role="alert">
            当前项目或设定授权无法读取，私有正文与恢复意图尚未打开。
          </p>
          <Button
            variant="outline"
            onClick={() => {
              void project.refetch();
              void context.refetch();
            }}
          >
            重新核验项目与当前身份
          </Button>
        </div>
      )}
      {!proven && !project.isError && !context.isError && (
        <p role="status">正在核验当前项目与设定范围…</p>
      )}
      {proven && origin && context.data && project.data && (
        <ScopedBibleWorkspace
          key={`${origin}:${context.data.current_actor_id}:${context.data.current_org_id}:${projectId}`}
          identity={{
            origin,
            actorId: context.data.current_actor_id,
            orgId: context.data.current_org_id,
            projectId,
          }}
          readOnly={project.data.status !== "active" || project.data.is_delete}
        />
      )}
    </section>
  );
}
function ScopedBibleWorkspace({
  identity,
  readOnly,
}: {
  identity: BibleIdentity;
  readOnly: boolean;
}) {
  const search = useSearchParams(),
    pathname = usePathname(),
    router = useRouter(),
    client = useQueryClient();
  const kindParsed = bibleKind.safeParse(search.get("kind") ?? "character"),
    kind = kindParsed.success ? kindParsed.data : "character";
  const idParsed = bibleUUID.safeParse(search.get("entry_id")),
    selected = idParsed.success ? idParsed.data : undefined,
    versionParsed = bibleUUID.safeParse(search.get("version_id")),
    version = versionParsed.success ? versionParsed.data : undefined;
  const [modal, setModal] = useState<Modal>(),
    [notice, setNotice] = useState<string>();
  const surface = useRef<HTMLElement>(null),
    trigger = useRef<HTMLElement | null>(null);
  function url(change: { kind?: BibleKind; entry?: string; version?: string }) {
    const params = new URLSearchParams(search.toString());
    if (change.kind) params.set("kind", change.kind);
    if ("entry" in change) {
      if (change.entry) params.set("entry_id", change.entry);
      else params.delete("entry_id");
    }
    if ("version" in change) {
      if (change.version) params.set("version_id", change.version);
      else params.delete("version_id");
    }
    router.replace(`${pathname}${params.size ? `?${params}` : ""}`, {
      scroll: false,
    });
  }
  async function accepted(receipt: BibleReceipt) {
    setModal(undefined);
    setNotice(`原修改已确认，回执版本 ${receipt.revision}。正在读取当前事实。`);
    url({
      kind: receipt.kind,
      entry: receipt.created_entry_id ?? receipt.entry_id,
      version: undefined,
    });
    await Promise.all([
      client.invalidateQueries({ queryKey: bibleScopeKey(identity) }),
      client.invalidateQueries({
        queryKey: bibleContextKey(identity.origin, identity.projectId),
      }),
      client.invalidateQueries({ queryKey: ["project", identity.projectId] }),
    ]);
  }
  const writer = useBibleWriter(identity, accepted),
    automaticHistory =
      !modal && writer.ready && !writer.locked && Boolean(version && selected),
    blocked = writer.locked || Boolean(modal) || automaticHistory,
    locked = readOnly || blocked,
    detail = useQuery({
      queryKey: [...bibleScopeKey(identity), "detail", kind, selected],
      queryFn: ({ signal }) =>
        getBibleDetail(identity, kind, selected!, signal),
      enabled: Boolean(selected),
      staleTime: 0,
      gcTime: 0,
      retry: false,
      refetchOnWindowFocus: false,
    });
  const proven = detail.isSuccess && detail.isFetchedAfterMount;
  function restoreFocus(event: Event) {
    event.preventDefault();
    if (document.querySelector("[data-bible-dialog]")) return;
    const element = trigger.current;
    if (element?.isConnected && !element.matches(":disabled")) element.focus();
    else surface.current?.focus();
  }
  function close() {
    setModal(undefined);
  }
  const recovery = !modal && (writer.intent || writer.storageError);
  const original = writer.intent?.command;
  const recoveryEntry =
    original &&
    (original.action === "create" ||
      original.action === "update" ||
      original.action === "split");
  return (
    <section
      ref={surface}
      tabIndex={-1}
      aria-label="当前项目设定集"
      className="space-y-5 outline-none"
      onClickCapture={(event) => {
        if (locked || recovery) return;
        const target = event.target;
        if (target instanceof Element) {
          const node = target.closest<HTMLElement>("button,a");
          if (node && surface.current?.contains(node)) trigger.current = node;
        }
      }}
    >
      {readOnly && <p role="status">当前项目只可阅读设定与历史。</p>}
      {notice && <p role="status">{notice}</p>}
      {!blocked ? (
        <nav aria-label="当前项目创作入口" className="flex flex-wrap gap-2">
          <Link href={`/projects/${identity.projectId}/canvas`}>
            打开项目画布
          </Link>
          <Link href={`/projects/${identity.projectId}/script`}>
            打开正式剧本
          </Link>
          <Link href={`/assets?scope=project&project_id=${identity.projectId}`}>
            打开项目素材库
          </Link>
        </nav>
      ) : null}
      <Tabs
        value={kind}
        onValueChange={(value) => {
          const next = bibleKind.safeParse(value);
          if (next.success)
            url({ kind: next.data, entry: undefined, version: undefined });
        }}
      >
        <TabsList aria-label="设定类型">
          {(["character", "location", "prop"] as const).map((value) => (
            <TabsTrigger key={value} value={value} disabled={blocked}>
              {kindLabels[value]}
            </TabsTrigger>
          ))}
        </TabsList>
        <TabsContent value={kind} className="space-y-4">
          <div className="flex flex-wrap gap-2">
            <Button
              disabled={locked}
              onClick={() =>
                setModal({
                  type: "entry",
                  frame: { action: "create", kind, revision: 0 },
                })
              }
            >
              新建{kindLabels[kind]}
            </Button>
            <Button
              variant="outline"
              disabled={locked}
              onClick={() => setModal({ type: "result", kind, revision: 0 })}
            >
              从正式结果建立{kindLabels[kind]}
            </Button>
            <Button
              variant="ghost"
              disabled={locked}
              onClick={() => {
                void client.invalidateQueries({
                  queryKey: bibleScopeKey(identity),
                });
              }}
            >
              重新读取设定
            </Button>
          </div>
          <div className="grid gap-6 lg:grid-cols-[minmax(15rem,1fr)_minmax(0,3fr)]">
            <BibleList
              identity={identity}
              kind={kind}
              selected={selected}
              locked={writer.locked || Boolean(modal)}
              onSelect={(id) => url({ entry: id, version: undefined })}
            />
            <div className="min-w-0 space-y-4">
              {!selected && <p>选择正式设定后按需读取完整正文。</p>}
              {selected && detail.isPending && (
                <p role="status">正在核验身份并读取完整设定…</p>
              )}
              {detail.isError && (
                <div>
                  <p role="alert">该原设定身份不存在或当前无权读取。</p>
                  <Button
                    variant="outline"
                    disabled={locked}
                    onClick={() => void detail.refetch()}
                  >
                    重新读取原设定
                  </Button>
                </div>
              )}
              {proven && (
                <BibleDetail
                  identity={identity}
                  detail={detail.data}
                  locked={blocked}
                  readOnly={readOnly}
                  onEdit={() =>
                    setModal({
                      type: "entry",
                      frame: {
                        action: "update",
                        kind,
                        id: detail.data.head.id,
                        revision: detail.data.head.revision,
                        initial: versionInput(detail.data),
                      },
                    })
                  }
                  onAction={(frame) => setModal({ type: "action", frame })}
                  onLook={(frame) => setModal({ type: "look", frame })}
                  onVoice={() =>
                    setModal({ type: "voice", detail: detail.data })
                  }
                  onHistory={() =>
                    setModal({ type: "history", id: detail.data.head.id, kind })
                  }
                  onResult={() =>
                    setModal({
                      type: "result",
                      kind,
                      id: detail.data.head.id,
                      revision: detail.data.head.revision,
                    })
                  }
                  onResolved={(id) => url({ entry: id, version: undefined })}
                />
              )}
              {proven &&
                detail.data.current.kind === "character" &&
                !detail.data.head.deleted &&
                !detail.data.head.redirect_id && (
                  <Button
                    variant="outline"
                    disabled={locked}
                    onClick={() =>
                      setModal({
                        type: "entry",
                        frame: {
                          action: "split",
                          kind: "character",
                          id: detail.data.head.id,
                          revision: detail.data.head.revision,
                          initial: versionInput(detail.data),
                        },
                      })
                    }
                  >
                    拆分为独立新角色
                  </Button>
                )}
            </div>
          </div>
        </TabsContent>
      </Tabs>
      {modal?.type === "entry" && (
        <EntryDialog
          frame={modal.frame}
          writer={writer}
          onClose={close}
          onCloseAutoFocus={restoreFocus}
        />
      )}
      {modal?.type === "look" && (
        <LookDialog
          identity={identity}
          frame={modal.frame}
          writer={writer}
          onClose={close}
          onCloseAutoFocus={restoreFocus}
        />
      )}
      {modal?.type === "voice" && (
        <VoiceDialog
          identity={identity}
          id={modal.detail.head.id}
          revision={modal.detail.head.revision}
          current={
            modal.detail.current.kind === "character"
              ? modal.detail.current.character.voice
              : undefined
          }
          writer={writer}
          onClose={close}
          onCloseAutoFocus={restoreFocus}
        />
      )}
      {modal?.type === "action" && (
        <ActionDialog
          identity={identity}
          frame={modal.frame}
          writer={writer}
          onClose={close}
          onCloseAutoFocus={restoreFocus}
        />
      )}
      {modal?.type === "result" && (
        <ResultDialog
          identity={identity}
          kind={modal.kind}
          id={modal.id}
          revision={modal.revision}
          writer={writer}
          onClose={close}
          onCloseAutoFocus={restoreFocus}
        />
      )}
      {(modal?.type === "history" || automaticHistory) && selected && (
        <HistoryDialog
          identity={identity}
          kind={kind}
          id={selected}
          version={version}
          onVersion={(version) => url({ version })}
          onClose={() => {
            close();
            url({ version: undefined });
          }}
          onCloseAutoFocus={restoreFocus}
        />
      )}
      {recovery &&
        (recoveryEntry ? (
          <EntryDialog
            writer={writer}
            onClose={close}
            onCloseAutoFocus={restoreFocus}
          />
        ) : (
          <BibleWriteDialog
            title="恢复原设定意图"
            writer={writer}
            dirty={false}
            onClose={close}
            onCloseAutoFocus={restoreFocus}
            onPrepareLatest={async () => {
              const latest = writer.latest?.detail;
              if (!writer.latest || !original) return;
              if (await writer.discardRejected()) {
                if (latest)
                  url({
                    kind: latest.head.kind,
                    entry: latest.head.id,
                    version: undefined,
                  });
                setNotice(
                  "旧意图已明确释放。请审核当前完整内容后，另行准备新的明确动作。",
                );
              }
            }}
          >
            <p>
              跨刷新仅保存原UUID键、完整原输入与冻结版本；当前身份不匹配时不会重放。
            </p>
          </BibleWriteDialog>
        ))}
    </section>
  );
}
