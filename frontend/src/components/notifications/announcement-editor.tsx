"use client";

import { useRef, useState } from "react";
import { useRouter } from "next/navigation";
import useSWR from "swr";
import {
  createPlatformNotification,
  savePlatformNotification,
  publishPlatformNotification,
} from "@/api/notifications";
import { platformTeams } from "@/api/identity";
import { RequestError } from "@/lib/request";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Field, FieldGroup, FieldLabel, FieldSet } from "@/components/ui/field";
import { Switch } from "@/components/ui/switch";
import { Alert, AlertTitle } from "@/components/ui/alert";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import {
  Item,
  ItemContent,
  ItemTitle,
  ItemDescription,
} from "@/components/ui/item";
import { AdminHeading } from "@/components/governance/admin-layout";
import { toast } from "@/components/feedback/notification";
import { NotificationSelect } from "./notification-filters";
import {
  requireAnnouncement,
  isSidebarPlacement,
  sidebarPromotionTargets,
  sidebarStudioTargets,
  type Announcement,
  type DisplayAnnouncement,
} from "./contracts";
import { AnnouncementDetail, GlobalAnnouncement } from "./global-announcement";
import { TimedAnnouncementDialog } from "./timed-announcement";
import { useNotificationCenter } from "./notification-provider";
import { beijingDateTime, beijingInstant } from "./notification-time";

type Draft = Required<API.AnnouncementContent>;
function initialDraft(): Draft {
  const now = Date.now();
  return {
    title: "",
    body: "",
    placement: "banner",
    audience: "authenticated",
    endless: false,
    manual_close: false,
    frequency: "once",
    team_id: "",
    starts_at: new Date(now).toISOString(),
    ends_at: new Date(now + 7 * 86400000).toISOString(),
    priority: 0,
    duration_seconds: 5,
    action_path: "",
    inbox: true,
  };
}
const positionOptions = [
  ["banner", "顶部公告"],
  ["modal", "消息弹窗"],
  ["inbox", "仅消息中心"],
  ["sidebar_badge", "侧栏 New 标记"],
  ["sidebar", "侧栏推广"],
] as const;
const audienceOptions = [
  ["public", "公开用户（含游客）"],
  ["authenticated", "全部登录用户"],
  ["team", "指定团队"],
] as const;
export { positionOptions, audienceOptions };

export function AnnouncementEditor({
  announcement,
}: {
  announcement?: Announcement;
}) {
  const router = useRouter();
  const center = useNotificationCenter();
  const [draft, setDraft] = useState<Draft>(() => ({
    ...initialDraft(),
    ...announcement?.draft,
    team_id: announcement?.draft.team_id ?? "",
  }));
  const [startMode, setStartMode] = useState(() =>
    announcement?.draft.starts_at &&
    Date.parse(announcement.draft.starts_at) > Date.now()
      ? "scheduled"
      : "immediate",
  );
  const [busy, setBusy] = useState(false);
  const sidebar = isSidebarPlacement(draft.placement);
  const sidebarBadge = draft.placement === "sidebar_badge";
  const targets = sidebarBadge ? sidebarStudioTargets : sidebarPromotionTargets;
  const [error, setError] = useState("");
  const [conflict, setConflict] = useState(false);
  const [uncertain, setUncertain] = useState(false);
  const originalIntent = useRef(false);
  const [preview, setPreview] = useState(false);
  const [teamSearch, setTeamSearch] = useState("");
  const [teamQuery, setTeamQuery] = useState("");
  const locked = useRef(false);
  const attempt = useRef<
    | {
        payload: string;
        saveID: string;
        publishID: string;
        saved?: Announcement;
        base?: Announcement;
      }
    | undefined
  >(undefined);
  const teams = useSWR(
    draft.audience === "team" ? ["notification-teams", teamQuery] : null,
    () =>
      platformTeams({
        search: teamQuery || undefined,
        limit: 100,
        status: "active",
      }),
    { shouldRetryOnError: false },
  );
  function change<K extends keyof Draft>(field: K, value: Draft[K]) {
    setDraft((current) => ({ ...current, [field]: value }));
  }
  async function submit(publish: boolean) {
    if (locked.current || conflict) return;
    if (sidebar && !targets.some(([path]) => path === draft.action_path)) {
      setError(sidebarBadge ? "请选择工作室" : "请选择落地页面");
      return;
    }
    if (
      !draft.starts_at ||
      (!draft.endless &&
        (!draft.ends_at ||
          new Date(draft.ends_at) <= new Date(draft.starts_at)))
    ) {
      setError("结束展示时间必须晚于开始时间");
      return;
    }
    locked.current = true;
    setBusy(true);
    setError("");
    originalIntent.current = publish;
    const content: API.AnnouncementContent = {
      ...draft,
      title: sidebarBadge ? "New" : draft.title.trim(),
      body: sidebar ? "" : draft.body.trim(),
      inbox: sidebar ? false : draft.inbox,
      ends_at: draft.endless ? undefined : draft.ends_at,
      team_id: draft.audience === "team" ? draft.team_id : undefined,
    };
    const payload = JSON.stringify(content);
    if (!attempt.current || attempt.current.payload !== payload)
      attempt.current = {
        payload,
        saveID: crypto.randomUUID(),
        publishID: crypto.randomUUID(),
        base: attempt.current?.saved ?? announcement,
      };
    const current = attempt.current;
    try {
      if (!current.saved) {
        const body = {
          command_id: current.saveID,
          expected_version: current.base?.version ?? "0",
          content,
        };
        current.saved = requireAnnouncement(
          current.base
            ? await savePlatformNotification({ id: current.base.id }, body)
            : await createPlatformNotification(body),
        );
      }
      const saved = current.saved;
      if (publish)
        await publishPlatformNotification(
          { id: saved.id },
          { command_id: current.publishID, expected_version: saved.version },
        );
      toast.success(publish ? "通知已发布" : "草稿已保存");
      void center?.refresh().catch(() => {});
      router.push(`/platform/notifications/${saved.id}`);
    } catch (e) {
      if (e instanceof RequestError && e.status === 409) {
        setConflict(true);
        setError("内容已被更新，请重新读取后核对");
      } else {
        const unknown =
          !(e instanceof RequestError) || e.status === 0 || e.status >= 500;
        setUncertain(unknown && !current.saved);
        setError(
          e instanceof RequestError && e.status === 422
            ? "请检查通知内容、有效时间和站内地址"
            : publish
              ? "发布未完成，请重试"
              : "保存未完成，请重试",
        );
      }
    } finally {
      locked.current = false;
      setBusy(false);
    }
  }
  const sample: DisplayAnnouncement = {
    id: announcement?.id ?? "preview",
    revision: announcement?.version ?? "0",
    notification_id: "",
    title: draft.title || "通知标题",
    body: draft.body || "通知正文",
    placement: draft.placement,
    duration_seconds: draft.duration_seconds,
    action_path: draft.action_path,
    starts_at: draft.starts_at,
    ends_at: draft.ends_at,
    endless: draft.endless,
    manual_close: draft.manual_close,
    frequency: draft.frequency,
  };
  return (
    <div className="admin-page">
      <AdminHeading title={announcement ? "编辑通知" : "新建通知"} />
      {error ? (
        <Alert variant={conflict ? "default" : "destructive"}>
          <AlertTitle>{error}</AlertTitle>
          {conflict ? (
            <Button
              variant="outline"
              onClick={() =>
                router.push(
                  announcement
                    ? `/platform/notifications/${announcement.id}`
                    : "/platform/notifications",
                )
              }
            >
              重新读取
            </Button>
          ) : null}
        </Alert>
      ) : null}
      <div className="grid min-w-0 items-start gap-8 lg:grid-cols-2">
        <form
          aria-label="通知内容"
          onSubmit={(event) => {
            event.preventDefault();
            const submitter = (event.nativeEvent as SubmitEvent)
              .submitter as HTMLButtonElement | null;
            void submit(submitter?.value === "publish");
          }}
        >
          <FieldGroup>
            <FieldSet className="contents" disabled={busy || uncertain}>
              <FieldGroup className="grid gap-4 sm:grid-cols-2">
                <NotificationSelect
                  label="展示位置"
                  value={draft.placement}
                  onChange={(value) =>
                    setDraft((current) => ({
                      ...current,
                      placement: value,
                      action_path:
                        isSidebarPlacement(value) !==
                          isSidebarPlacement(current.placement) ||
                        (value === "sidebar_badge" &&
                          !sidebarStudioTargets.some(
                            ([path]) => path === current.action_path,
                          ))
                          ? ""
                          : current.action_path,
                      audience:
                        value === "modal" && current.audience === "public"
                          ? "authenticated"
                          : current.audience,
                    }))
                  }
                  options={positionOptions}
                />
                <NotificationSelect
                  label="接收范围"
                  value={draft.audience}
                  onChange={(value) => change("audience", value)}
                  options={
                    draft.placement === "modal"
                      ? audienceOptions.filter(([key]) => key !== "public")
                      : audienceOptions
                  }
                />
              </FieldGroup>
              {!sidebarBadge ? (
                <Field>
                  <FieldLabel htmlFor="announcement-title">标题</FieldLabel>
                  <Input
                    id="announcement-title"
                    required
                    maxLength={sidebar ? 24 : 120}
                    value={draft.title}
                    onChange={(event) => change("title", event.target.value)}
                  />
                </Field>
              ) : null}
              {!sidebar ? (
                <Field>
                  <FieldLabel htmlFor="announcement-body">正文</FieldLabel>
                  <Textarea
                    id="announcement-body"
                    required
                    maxLength={10000}
                    rows={5}
                    value={draft.body}
                    onChange={(event) => change("body", event.target.value)}
                  />
                </Field>
              ) : (
                <NotificationSelect
                  label={sidebarBadge ? "工作室" : "落地页面"}
                  placeholder="请选择"
                  value={draft.action_path}
                  onChange={(value) => change("action_path", value)}
                  options={targets}
                />
              )}
              {draft.audience === "team" ? (
                <FieldGroup>
                  <Field>
                    <FieldLabel htmlFor="announcement-team-search">
                      查找团队
                    </FieldLabel>
                    <div className="flex gap-2">
                      <Input
                        id="announcement-team-search"
                        value={teamSearch}
                        onChange={(event) => setTeamSearch(event.target.value)}
                      />
                      <Button
                        type="button"
                        variant="outline"
                        onClick={() => setTeamQuery(teamSearch.trim())}
                      >
                        查找
                      </Button>
                    </div>
                  </Field>
                  {teams.error ? (
                    <Alert>
                      <AlertTitle>团队读取失败</AlertTitle>
                      <Button
                        type="button"
                        variant="outline"
                        onClick={() => void teams.mutate()}
                      >
                        重试
                      </Button>
                    </Alert>
                  ) : (
                    <NotificationSelect
                      label="团队"
                      value={draft.team_id || "none"}
                      onChange={(value) =>
                        change("team_id", value === "none" ? "" : value)
                      }
                      options={[
                        ["none", teams.isLoading ? "正在读取团队" : "选择团队"],
                        ...(teams.data?.items ?? []).map(
                          (team) => [team.id!, team.name!] as const,
                        ),
                      ]}
                    />
                  )}
                </FieldGroup>
              ) : null}
              <FieldGroup className="grid gap-4 sm:grid-cols-2">
                <NotificationSelect
                  label="开始方式"
                  value={startMode}
                  options={[
                    ["immediate", "立即发布"],
                    ["scheduled", "定时发布"],
                  ]}
                  onChange={(mode) => {
                    setStartMode(mode);
                    if (mode === "immediate")
                      change("starts_at", new Date().toISOString());
                  }}
                />
                {startMode === "scheduled" ? (
                  <Field>
                    <FieldLabel htmlFor="announcement-start">
                      开始时间 · 北京时间
                    </FieldLabel>
                    <Input
                      id="announcement-start"
                      type="datetime-local"
                      required
                      value={beijingDateTime(draft.starts_at)}
                      onChange={(event) =>
                        change("starts_at", beijingInstant(event.target.value))
                      }
                    />
                  </Field>
                ) : null}
              </FieldGroup>
              <FieldGroup className="grid gap-4 sm:grid-cols-2">
                <NotificationSelect
                  label="结束展示"
                  value={draft.endless ? "endless" : "scheduled"}
                  options={[
                    ["scheduled", "指定时间"],
                    ["endless", "长期有效"],
                  ]}
                  onChange={(mode) => change("endless", mode === "endless")}
                />
                {!draft.endless ? (
                  <Field
                    data-invalid={error.includes("结束展示时间") || undefined}
                  >
                    <FieldLabel htmlFor="announcement-end">
                      截止时间 · 北京时间
                    </FieldLabel>
                    <Input
                      id="announcement-end"
                      type="datetime-local"
                      required
                      aria-invalid={error.includes("结束展示时间") || undefined}
                      value={beijingDateTime(draft.ends_at)}
                      onChange={(event) =>
                        change("ends_at", beijingInstant(event.target.value))
                      }
                    />
                  </Field>
                ) : null}
              </FieldGroup>
              {draft.placement === "modal" ? (
                <FieldGroup>
                  <FieldGroup className="grid gap-4 sm:grid-cols-2">
                    <NotificationSelect
                      label="弹窗关闭方式"
                      value={draft.manual_close ? "manual" : "timed"}
                      options={[
                        ["timed", "定时关闭"],
                        ["manual", "手动关闭"],
                      ]}
                      onChange={(value) =>
                        change("manual_close", value === "manual")
                      }
                    />
                    {!draft.manual_close ? (
                      <Field>
                        <FieldLabel htmlFor="announcement-duration">
                          停留时长（秒）
                        </FieldLabel>
                        <Input
                          id="announcement-duration"
                          type="number"
                          required
                          min={5}
                          max={60}
                          value={draft.duration_seconds}
                          onChange={(event) =>
                            change(
                              "duration_seconds",
                              Number(event.target.value),
                            )
                          }
                        />
                      </Field>
                    ) : null}
                  </FieldGroup>
                  <NotificationSelect
                    label="重复频次"
                    value={draft.frequency}
                    options={[
                      ["once", "每个账号一次"],
                      ["daily", "每天一次"],
                      ["login", "每次登录一次"],
                    ]}
                    onChange={(value) => {
                      if (
                        value === "once" ||
                        value === "daily" ||
                        value === "login"
                      )
                        change("frequency", value);
                    }}
                  />
                </FieldGroup>
              ) : null}
              {!sidebar ? (
                <Field orientation="horizontal">
                  <FieldLabel htmlFor="announcement-inbox">
                    同时保存到消息中心
                  </FieldLabel>
                  <Switch
                    id="announcement-inbox"
                    checked={draft.inbox || draft.placement === "inbox"}
                    disabled={draft.placement === "inbox"}
                    onCheckedChange={(value) => change("inbox", value)}
                  />
                </Field>
              ) : null}
              <Collapsible>
                <CollapsibleTrigger asChild>
                  <Button type="button" variant="ghost">
                    高级设置
                  </Button>
                </CollapsibleTrigger>
                <CollapsibleContent>
                  <FieldGroup className="pt-4">
                    <Field>
                      <FieldLabel htmlFor="announcement-priority">
                        展示优先级
                      </FieldLabel>
                      <Input
                        id="announcement-priority"
                        type="number"
                        min={0}
                        max={100}
                        value={draft.priority}
                        onChange={(event) =>
                          change("priority", Number(event.target.value))
                        }
                      />
                    </Field>
                    {!sidebar ? (
                      <Field>
                        <FieldLabel htmlFor="announcement-action">
                          站内跳转地址
                        </FieldLabel>
                        <Input
                          id="announcement-action"
                          value={draft.action_path}
                          placeholder="/assets"
                          onChange={(event) =>
                            change("action_path", event.target.value)
                          }
                        />
                      </Field>
                    ) : null}
                  </FieldGroup>
                </CollapsibleContent>
              </Collapsible>
            </FieldSet>
            <div className="flex flex-wrap gap-2">
              <Button
                type="button"
                variant="outline"
                disabled={busy}
                onClick={() => router.push("/platform/notifications")}
              >
                取消
              </Button>
              <Button
                variant="outline"
                value="draft"
                disabled={busy || conflict || uncertain}
              >
                保存草稿
              </Button>
              <Button value="publish" disabled={busy || conflict || uncertain}>
                {busy ? "保存中…" : "发布"}
              </Button>
            </div>
            {uncertain ? (
              <Button
                type="button"
                disabled={busy}
                onClick={() => void submit(originalIntent.current)}
              >
                重试原操作
              </Button>
            ) : null}
          </FieldGroup>
        </form>
        <aside className="flex min-w-0 flex-col gap-4" aria-label="通知预览">
          <h2 className="text-base font-medium">预览</h2>
          {sidebar ? (
            <Item variant="muted">
              <ItemContent>
                <ItemTitle>
                  {sidebarBadge
                    ? (targets.find(
                        ([path]) => path === draft.action_path,
                      )?.[1] ?? "工作室")
                    : draft.title || "推广标题"}
                </ItemTitle>
              </ItemContent>
              {sidebarBadge ? <Badge variant="secondary">New</Badge> : null}
            </Item>
          ) : (
            <>
              <p className="text-xs text-muted-foreground">
                {positionOptions.find(([key]) => key === draft.placement)?.[1]}{" "}
                ·{" "}
                {draft.placement === "modal"
                  ? draft.manual_close
                    ? "手动关闭"
                    : `${draft.duration_seconds} 秒`
                  : "北京时间"}
              </p>
              {draft.placement === "banner" ? (
                <div className="overflow-hidden rounded-lg border [--global-announcement-height:40px] max-sm:[--global-announcement-height:56px]">
                  <GlobalAnnouncement
                    announcement={sample}
                    onDismiss={() => {}}
                    onExpire={() => {}}
                    onView={() => setPreview(true)}
                  />
                </div>
              ) : (
                <Item variant="muted">
                  <ItemContent>
                    <ItemTitle>{sample.title}</ItemTitle>
                    <ItemDescription>{sample.body}</ItemDescription>
                  </ItemContent>
                </Item>
              )}
              <Button
                className="self-start"
                variant="outline"
                onClick={() => setPreview(true)}
              >
                预览消息
              </Button>
            </>
          )}
        </aside>
      </div>
      {preview && draft.placement === "modal" ? (
        <TimedAnnouncementDialog
          announcement={sample}
          onClose={() => setPreview(false)}
        />
      ) : preview ? (
        <AnnouncementDetail
          announcement={sample}
          onClose={() => setPreview(false)}
        />
      ) : null}
    </div>
  );
}
