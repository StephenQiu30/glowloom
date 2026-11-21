"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import useSWR from "swr";
import {
  getPlatformNotification,
  getPlatformNotificationAudit,
  setPlatformNotificationEnabled,
  publishPlatformNotification,
  deletePlatformNotification,
} from "@/api/notifications";
import { useSession } from "@/components/auth/session";
import { AdminHeading } from "@/components/governance/admin-layout";
import { ContentState } from "@/components/layout/content-state";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
import { Field, FieldLabel } from "@/components/ui/field";
import { Separator } from "@/components/ui/separator";
import { Alert, AlertTitle } from "@/components/ui/alert";
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogCancel,
  AlertDialogAction,
} from "@/components/ui/alert-dialog";
import {
  Collapsible,
  CollapsibleTrigger,
  CollapsibleContent,
} from "@/components/ui/collapsible";
import { toast } from "@/components/feedback/notification";
import { RequestError } from "@/lib/request";
import { requireAnnouncement, notificationDate } from "./contracts";
import { positionOptions, audienceOptions } from "./announcement-editor";
import { announcementStatus, optionLabel } from "./announcement-list";
import { useNotificationCenter } from "./notification-provider";

export function AnnouncementAdministration({ id }: { id: string }) {
  const router = useRouter();
  const { actor } = useSession();
  const center = useNotificationCenter();
  const detail = useSWR(
    ["announcement-detail", actor.id, id],
    async () => requireAnnouncement(await getPlatformNotification({ id })),
    { shouldRetryOnError: false },
  );
  const [auditOpen, setAuditOpen] = useState(false);
  const [pending, setPending] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [error, setError] = useState("");
  const [conflict, setConflict] = useState(false);
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 30000);
    return () => clearInterval(timer);
  }, []);
  const locked = useRef(false);
  const commands = useRef(new Map<string, string>());
  const audit = useSWR(
    auditOpen
      ? ["announcement-audit", actor.id, id, detail.data?.version]
      : null,
    () => getPlatformNotificationAudit({ id }),
    { shouldRetryOnError: false },
  );
  const value = detail.data;
  if (detail.error)
    return (
      <ContentState
        state="error"
        title="通知暂时无法读取"
        actions={
          <Button variant="outline" onClick={() => void detail.mutate()}>
            重新读取
          </Button>
        }
      />
    );
  if (!value) return <ContentState state="loading" title="正在读取通知" />;
  const content = value.published ?? value.draft;
  const expired =
    !content.endless && content.ends_at
      ? Date.parse(content.ends_at) <= now
      : false;
  const unpublished =
    !value.published ||
    JSON.stringify(value.draft) !== JSON.stringify(value.published);
  async function command(
    action: "publish" | "enabled" | "delete",
    enabled?: boolean,
  ) {
    if (!value || locked.current || conflict) return;
    locked.current = true;
    setPending(true);
    setError("");
    const key = `${value.version}:${action}:${enabled}`;
    if (!commands.current.has(key))
      commands.current.set(key, crypto.randomUUID());
    const body = {
      command_id: commands.current.get(key)!,
      expected_version: value.version,
    };
    try {
      const next =
        action === "delete"
          ? await deletePlatformNotification({ id }, body)
          : action === "publish"
            ? await publishPlatformNotification({ id }, body)
            : await setPlatformNotificationEnabled(
                { id },
                { ...body, enabled },
              );
      if (action === "delete") {
        setDeleteOpen(false);
        toast.success("草稿已删除");
        router.push("/platform/notifications");
        return;
      }
      await detail.mutate(requireAnnouncement(next), false);
      void center?.refresh().catch(() => {});
      toast.success(
        action === "publish"
          ? "通知已发布"
          : enabled
            ? "通知已启用"
            : "通知已停用",
      );
    } catch (e) {
      if (e instanceof RequestError && e.status === 409) {
        setConflict(true);
        setError("内容已被更新，请重新读取");
      } else setError("操作未完成，请重试");
    } finally {
      locked.current = false;
      setPending(false);
    }
  }
  return (
    <div className="notification-surface flex min-w-0 flex-col gap-6">
      <AdminHeading title={content.title ?? "通知详情"} detail>
        <Button variant="outline" asChild>
          <Link href={`/platform/notifications/${id}/edit`}>编辑</Link>
        </Button>
        {unpublished ? (
          <Button
            disabled={pending || conflict}
            onClick={() => void command("publish")}
          >
            发布草稿
          </Button>
        ) : null}
        <Button variant="ghost" asChild>
          <Link href="/platform/notifications">返回列表</Link>
        </Button>
        {!value.published ? (
          <Button
            variant="ghost"
            disabled={pending || conflict}
            onClick={() => setDeleteOpen(true)}
          >
            删除草稿
          </Button>
        ) : null}
      </AdminHeading>
      {error ? (
        <Alert variant="destructive">
          <AlertTitle>{error}</AlertTitle>
          {conflict ? (
            <Button
              variant="outline"
              onClick={() => {
                void detail.mutate().then(() => {
                  setConflict(false);
                  setError("");
                });
              }}
            >
              重新读取
            </Button>
          ) : null}
        </Alert>
      ) : null}
      <div className="flex items-center justify-between gap-4">
        <Badge variant="secondary">{announcementStatus(value.status)}</Badge>
        {value.published ? (
          <Field orientation="horizontal" className="w-auto">
            <FieldLabel htmlFor="announcement-enabled">启用</FieldLabel>
            <Switch
              id="announcement-enabled"
              checked={value.enabled}
              disabled={pending || conflict || expired}
              onCheckedChange={(enabled) => void command("enabled", enabled)}
            />
          </Field>
        ) : null}
      </div>
      <Separator />
      <p className="break-words whitespace-pre-wrap">{content.body}</p>
      <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-8 gap-y-4 text-sm">
        <dt className="text-muted-foreground">展示位置</dt>
        <dd>
          {optionLabel(positionOptions, content.placement)}
          {content.inbox && content.placement !== "inbox" ? "、消息中心" : ""}
        </dd>
        <dt className="text-muted-foreground">接收范围</dt>
        <dd>{optionLabel(audienceOptions, content.audience)}</dd>
        <dt className="text-muted-foreground">开始时间（北京时间）</dt>
        <dd>{content.starts_at ? notificationDate(content.starts_at) : "—"}</dd>
        <dt className="text-muted-foreground">结束展示（北京时间）</dt>
        <dd>
          {content.endless
            ? "长期有效"
            : content.ends_at
              ? notificationDate(content.ends_at)
              : "—"}
        </dd>
        <dt className="text-muted-foreground">自动关闭</dt>
        <dd>
          {content.manual_close ? "手动关闭" : `${content.duration_seconds} 秒`}
        </dd>
        {content.placement === "modal" ? (
          <>
            <dt className="text-muted-foreground">重复频次</dt>
            <dd>
              {
                (
                  {
                    once: "每个账号一次",
                    daily: "每天一次",
                    login: "每次登录一次",
                  } as Record<string, string>
                )[content.frequency ?? "once"]
              }
            </dd>
          </>
        ) : null}
      </dl>
      {value.published && unpublished ? (
        <Alert>
          <AlertTitle>有未发布的修改</AlertTitle>
        </Alert>
      ) : null}
      <Separator />
      <Collapsible open={auditOpen} onOpenChange={setAuditOpen}>
        <CollapsibleTrigger asChild>
          <Button variant="ghost">查看操作记录</Button>
        </CollapsibleTrigger>
        <CollapsibleContent className="pt-4">
          {audit.error ? (
            <ContentState
              state="error"
              title="操作记录读取失败"
              actions={
                <Button variant="outline" onClick={() => void audit.mutate()}>
                  重试
                </Button>
              }
            />
          ) : audit.isLoading ? (
            <ContentState state="loading" title="正在读取操作记录" />
          ) : (
            <ul className="flex flex-col gap-3">
              {audit.data?.items?.map((item) => (
                <li
                  key={item.id}
                  className="flex justify-between gap-4 text-sm"
                >
                  <span>
                    {(
                      {
                        created: "创建草稿",
                        saved: "保存草稿",
                        published: "发布通知",
                        enabled: "启用通知",
                        disabled: "停用通知",
                      } as Record<string, string>
                    )[item.action ?? ""] ?? "更新通知"}
                  </span>
                  <time className="text-muted-foreground">
                    {item.created_at ? notificationDate(item.created_at) : "—"}
                  </time>
                </li>
              ))}
            </ul>
          )}
        </CollapsibleContent>
      </Collapsible>
      <AlertDialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>删除草稿？</AlertDialogTitle>
            <AlertDialogDescription>
              删除后将无法继续编辑此草稿。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={pending}>取消</AlertDialogCancel>
            <AlertDialogAction
              disabled={pending || conflict}
              onClick={(event) => {
                event.preventDefault();
                void command("delete");
              }}
            >
              {pending ? "删除中…" : "删除草稿"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
