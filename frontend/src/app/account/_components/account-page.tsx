"use client";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { ContentState } from "@/components/layout/content-state";
import { readErrorMessage } from "@/lib/read-error";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Field, FieldLabel, FieldGroup } from "@/components/ui/field";
import { useEffect, useRef, useState, useId } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useTheme } from "next-themes";
import { ArrowLeft, ChevronRight, Check, MoreHorizontal } from "lucide-react";
import { SettingsSkeleton } from "./settings-skeleton";
import { DialogTitle } from "@/components/ui/dialog";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
} from "@/components/ui/dropdown-menu";
import { useCanvasBackground } from "@/components/canvas/hooks/use-canvas-background";
import { BoundAvatar } from "@/components/account/avatar-editor";
import { AvatarEditor } from "@/components/account/avatar-editor";
import { getMyAvatar } from "@/api/media";
import { EmailChange } from "./email-change";
import { PageLayout } from "@/components/layout/page-layout";
import {
  useSession,
  useSessionUpdate,
  revokeSession,
  clearPrivateSession,
} from "@/components/auth/session";
import { authErrorMessage, requireAccepted } from "@/components/auth/contracts";
import {
  getMe,
  updateMe,
  getMyPreferences,
  updateMyPreferences,
  getMySessions,
  deleteSession,
  revokeOtherSessions,
  getMySecurityEvents,
  previewAccountDeletion,
  reauthenticate,
  startAccountDeletion,
  confirmAccountDeletion,
} from "@/api/identity";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import {
  Collapsible,
  CollapsibleTrigger,
  CollapsibleContent,
} from "@/components/ui/collapsible";
import { Checkbox } from "@/components/ui/checkbox";

export function Settings({
  embedded = false,
  initialView = "home",
  onClose = () => {},
}: {
  embedded?: boolean;
  initialView?: string;
  onClose?: () => void;
}) {
  const formFieldId = useId();

  const session = useSession();
  const updateSession = useSessionUpdate();
  const router = useRouter();
  const { setTheme } = useTheme();
  const [tab, setTab] = useState(initialView);
  const [background, saveBackground, backgroundQuery] = useCanvasBackground(
    session.actor.id,
  );
  const { mutate: publishPreferences } = backgroundQuery;
  const [profile, setProfile] = useState<API.MeView | null>(null);
  const [preferences, setPreferences] = useState<API.PreferencesView | null>(
    null,
  );
  const [bio, setBio] = useState("");
  const [name, setName] = useState("");
  const [theme, changeTheme] = useState("dark");
  const [suffix, setSuffix] = useState("");
  const [sessions, setSessions] = useState<API.SessionItemView[]>([]);
  const [loadingBlocks, setLoadingBlocks] = useState({
    profile: true,
    preferences: true,
    sessions: true,
    events: true,
  });
  const [blockErrors, setBlockErrors] = useState<
    Partial<Record<"profile" | "preferences" | "sessions" | "events", string>>
  >({});
  const [events, setEvents] = useState<API.SecurityEventPageView | null>(null);
  const [impact, setImpact] = useState<API.DeletionImpactView | null>(null);
  const [password, setPassword] = useState("");
  const [code, setCode] = useState("");
  const [operation, setOperation] = useState("");
  const [consent, setConsent] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [reload, setReload] = useState(0);
  const flight = useRef<AbortController | null>(null);
  const locked = useRef(false);
  useEffect(() => {
    const controller = new AbortController();
    flight.current = controller;
    function load<T>(
      block: keyof typeof loadingBlocks,
      result: Promise<T>,
      apply: (value: T) => void,
    ) {
      result
        .then((value) => {
          if (!controller.signal.aborted) apply(value);
        })
        .catch((reason) => {
          if (!controller.signal.aborted)
            setBlockErrors((current) => ({
              ...current,
              [block]: readErrorMessage(reason),
            }));
        })
        .finally(() => {
          if (!controller.signal.aborted)
            setLoadingBlocks((current) => ({ ...current, [block]: false }));
        });
    }
    load("profile", getMe({ signal: controller.signal }), (p) => {
      setProfile(p);
      setImpact(null);
      setOperation("");
      setName(p.display_name);
      setBio(p.bio ?? "");
    });
    load(
      "preferences",
      getMyPreferences({ signal: controller.signal }),
      (prefs) => {
        setPreferences(prefs);
        void publishPreferences(prefs, { revalidate: false });
        changeTheme(prefs.theme);
        setSuffix(prefs.prompt_suffix);
      },
    );
    load("sessions", getMySessions({ signal: controller.signal }), (devices) =>
      setSessions(devices.items),
    );
    load(
      "events",
      getMySecurityEvents({ limit: 20 }, { signal: controller.signal }),
      setEvents,
    );
    return () => {
      controller.abort();
      flight.current?.abort();
    };
  }, [session.actor.id, reload, publishPreferences]);
  async function run(action: (signal: AbortSignal) => Promise<void>) {
    if (locked.current) return;
    locked.current = true;
    setPending(true);
    setError("");
    setNotice("");
    const controller = new AbortController();
    flight.current = controller;
    try {
      await action(controller.signal);
    } catch (reason) {
      if (!controller.signal.aborted) setError(authErrorMessage(reason));
    } finally {
      locked.current = false;
      if (!controller.signal.aborted) setPending(false);
    }
  }
  async function saveProfile(signal: AbortSignal) {
    if (!profile) return;
    const updated = await updateMe(
      {
        expected_revision: profile.revision,
        display_name: name.trim(),
        bio: bio.trim(),
      },
      { signal },
    );
    const readBack = await getMe({ signal });
    if (signal.aborted) return;
    if (updated.revision !== readBack.revision) {
      setError("资料保存后又发生变化，请重新加载。");
      return;
    }
    setProfile(readBack);
    if (preferences)
      void publishPreferences({
        ...preferences,
        revision: readBack.revision,
      });
    setPreferences((current) =>
      current ? { ...current, revision: readBack.revision } : current,
    );
    setImpact(null);
    setOperation("");
    setName(readBack.display_name);
    setBio(readBack.bio ?? "");
    updateSession({ ...session, actor: readBack });
    setNotice("个人资料已保存并读回。");
    if (embedded && initialView === "profile") onClose();
    else setTab("home");
  }
  async function savePreferences(signal: AbortSignal, nextTheme?: string) {
    if (!preferences) return;
    await updateMyPreferences(
      {
        expected_revision: preferences.revision,
        ...(nextTheme ? { theme: nextTheme } : { prompt_suffix: suffix }),
      },
      { signal },
    );
    const [readBack, p] = await Promise.all([
      getMyPreferences({ signal }),
      getMe({ signal }),
    ]);
    if (signal.aborted) return;
    if (
      nextTheme
        ? readBack.theme !== nextTheme
        : readBack.prompt_suffix !== suffix
    ) {
      throw new Error("偏好未保存成功，请重试。");
    }
    setPreferences(readBack);
    void publishPreferences(readBack, { revalidate: false });
    changeTheme(readBack.theme);
    setSuffix(readBack.prompt_suffix);
    setProfile(p);
    setTheme(readBack.theme);
    updateSession({ ...session, actor: p });
    setNotice("偏好已保存。");
  }
  function refresh() {
    setImpact(null);
    setOperation("");
    setCode("");
    setPassword("");
    setConsent(false);
    setError("");
    setNotice("");
    setLoadingBlocks({
      profile: true,
      preferences: true,
      sessions: true,
      events: true,
    });
    setBlockErrors({});
    setReload((n) => n + 1);
  }
  const title =
    (
      {
        home: "账号设置",
        profile: "编辑资料",
        theme: "主题模式",
        background: "画布背景",
        logout: "退出登录",
        security: "密码与会话",
        preferences: "创作偏好",
        deletion: "注销账号",
      } as Record<string, string>
    )[tab] ?? "账号设置";
  const busy = pending;
  const row = (label: string, value: string, view: string) => (
    <Button
      type="button"
      variant="ghost"
      className="h-12 w-full justify-between px-0"
      disabled={busy}
      onClick={() => setTab(view)}
    >
      <span>{label}</span>
      <span className="flex items-center gap-3 text-muted-foreground">
        {value}
        <ChevronRight className="size-4" />
      </span>
    </Button>
  );
  return (
    <section className="account-settings-content" aria-busy={busy}>
      <div className="flex h-8 items-center gap-2 pr-8">
        {tab !== "home" && (
          <Button
            variant="ghost"
            size="icon"
            aria-label="返回账号设置"
            disabled={busy}
            onClick={() => {
              setError("");
              setTab("home");
            }}
          >
            <ArrowLeft />
          </Button>
        )}
        {embedded ? (
          <DialogTitle>{title}</DialogTitle>
        ) : (
          <h1 className="text-base font-medium">{title}</h1>
        )}
      </div>
      <div className="flex flex-col gap-5">
        {error ? (
          <Alert role="alert">
            <AlertDescription>{error}</AlertDescription>
            <Button variant="ghost" disabled={pending} onClick={refresh}>
              重新加载
            </Button>
          </Alert>
        ) : null}
        {notice ? <FeedbackToast message={notice} tone="info" /> : null}
        {tab === "home" ? (
          loadingBlocks.profile || loadingBlocks.preferences ? (
            <SettingsSkeleton />
          ) : blockErrors.profile || blockErrors.preferences ? (
            <ContentState
              state="error"
              title="账号设置暂时无法读取"
              description={blockErrors.profile || blockErrors.preferences}
              actions={<Button onClick={refresh}>重试</Button>}
            />
          ) : profile ? (
            <>
              <div className="flex min-h-14 items-center gap-3">
                <BoundAvatar
                  assetID={profile.avatar_asset_id}
                  label={profile.display_name}
                  load={(signal) => getMyAvatar({ signal })}
                  size="compact"
                />
                <div className="min-w-0 flex-1">
                  <p className="truncate">{profile.display_name}</p>
                  <p className="truncate text-sm text-muted-foreground">
                    {profile.email}
                  </p>
                </div>
                <Button
                  variant="ghost"
                  disabled={busy}
                  onClick={() => {
                    setName(profile.display_name);
                    setBio(profile.bio ?? "");
                    setTab("profile");
                  }}
                >
                  编辑资料
                </Button>
              </div>
              <div>
                {row(
                  "主题模式",
                  (
                    {
                      light: "浅色",
                      dark: "深色",
                      system: "跟随系统",
                    } as Record<string, string>
                  )[theme] ?? "深色",
                  "theme",
                )}
                {row(
                  "画布背景",
                  { dots: "点阵", lines: "网格", blank: "无背景" }[background],
                  "background",
                )}
              </div>
              <div className="flex items-center justify-between gap-2">
                <Button variant="ghost" onClick={() => setTab("logout")}>
                  退出登录
                </Button>
                <div className="flex gap-2">
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button
                        variant="ghost"
                        size="icon"
                        aria-label="更多账号设置"
                      >
                        <MoreHorizontal />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent>
                      <DropdownMenuGroup>
                        {[
                          ["security", "密码与会话"],
                          ["preferences", "创作偏好"],
                          ["deletion", "注销账号"],
                        ].map(([view, label]) => (
                          <DropdownMenuItem
                            key={view}
                            onSelect={() => setTab(view)}
                          >
                            {label}
                          </DropdownMenuItem>
                        ))}
                      </DropdownMenuGroup>
                    </DropdownMenuContent>
                  </DropdownMenu>
                  <Button className="min-w-20" onClick={onClose}>
                    完成
                  </Button>
                </div>
              </div>
            </>
          ) : null
        ) : tab === "theme" || tab === "background" ? (
          <>
            <ToggleGroup
              type="single"
              orientation="vertical"
              className="w-full"
              aria-label={title}
              value={tab === "theme" ? theme : background}
              disabled={busy}
              onValueChange={(value) => {
                if (!value) return;
                if (tab === "theme")
                  void run(async (signal) => {
                    await savePreferences(signal, value);
                    if (!signal.aborted) setTab("home");
                  });
                else if (
                  value === "dots" ||
                  value === "lines" ||
                  value === "blank"
                ) {
                  void run(async (signal) => {
                    const saved = await saveBackground(value);
                    const p = await getMe({ signal });
                    if (signal.aborted) return;
                    setPreferences(saved);
                    setProfile(p);
                    updateSession({ ...session, actor: p });
                    setTab("home");
                  });
                }
              }}
            >
              {(tab === "theme"
                ? [
                    ["light", "浅色"],
                    ["dark", "深色"],
                    ["system", "跟随系统"],
                  ]
                : [
                    ["dots", "点阵"],
                    ["lines", "网格"],
                    ["blank", "无背景"],
                  ]
              ).map(([value, label]) => (
                <ToggleGroupItem
                  className="h-12 w-full justify-between px-3"
                  key={value}
                  value={value}
                >
                  {label}
                  {(tab === "theme" ? theme : background) === value && (
                    <Check className="size-4" />
                  )}
                </ToggleGroupItem>
              ))}
            </ToggleGroup>
            {pending && (
              <p role="status" className="text-sm text-muted-foreground">
                正在保存…
              </p>
            )}
          </>
        ) : tab === "logout" ? (
          <>
            <p className="text-sm text-muted-foreground">
              退出后需要重新登录才能继续创作。
            </p>
            <div className="flex justify-end gap-3">
              <Button
                variant="ghost"
                className="min-w-20"
                disabled={busy}
                onClick={() => setTab("home")}
              >
                取消
              </Button>
              <Button
                className="min-w-20"
                disabled={busy}
                onClick={() =>
                  void run(async () => {
                    await revokeSession(session);
                    onClose();
                    router.replace("/");
                  })
                }
              >
                {pending ? "正在退出…" : "退出登录"}
              </Button>
            </div>
          </>
        ) : tab === "profile" ? (
          loadingBlocks.profile ? (
            <SettingsSkeleton label="正在读取个人资料" />
          ) : blockErrors.profile ? (
            <ContentState
              className="min-h-40"
              state="error"
              title="个人资料暂时无法读取"
              description={blockErrors.profile}
              actions={<Button onClick={refresh}>重试</Button>}
            />
          ) : profile ? (
            <>
              <AvatarEditor
                label={profile.display_name}
                compact
                onBusyChange={setPending}
                assetID={profile.avatar_asset_id}
                load={(signal) => getMyAvatar({ signal })}
                onConflict={async () => {
                  const readBack = await getMe();
                  setProfile(readBack);
                  if (preferences)
                    void publishPreferences({
                      ...preferences,
                      revision: readBack.revision,
                    });
                  setPreferences((p) =>
                    p ? { ...p, revision: readBack.revision } : p,
                  );
                  updateSession({ ...session, actor: readBack });
                }}
                bind={async (asset, signal) => {
                  const body = {
                    expected_revision: profile.revision,
                    display_name: profile.display_name,
                  };
                  await updateMe(body, {
                    signal,
                    data: { ...body, avatar_asset_id: asset },
                  });
                  const readBack = await getMe({ signal });
                  if (signal.aborted) return;
                  setProfile(readBack);
                  if (preferences)
                    void publishPreferences({
                      ...preferences,
                      revision: readBack.revision,
                    });
                  setPreferences((p) =>
                    p ? { ...p, revision: readBack.revision } : p,
                  );
                  updateSession({ ...session, actor: readBack });
                }}
              />
              <form
                onSubmit={(e) => {
                  e.preventDefault();
                  void run(saveProfile);
                }}
              >
                <FieldGroup>
                  <Field data-disabled={pending}>
                    <FieldLabel htmlFor={`${formFieldId}-field-1`}>
                      <span>显示名</span>
                    </FieldLabel>
                    <Input
                      id={`${formFieldId}-field-1`}
                      value={name}
                      onChange={(e) => setName(e.target.value)}
                      disabled={pending}
                    />
                  </Field>
                  <Field data-disabled={pending}>
                    <FieldLabel htmlFor={`${formFieldId}-bio`}>
                      个人简介
                    </FieldLabel>
                    <Textarea
                      id={`${formFieldId}-bio`}
                      value={bio}
                      disabled={pending}
                      onChange={(e) => setBio(e.target.value)}
                      className="min-h-20 resize-y"
                      aria-invalid={Array.from(bio).length > 100}
                      aria-describedby={`${formFieldId}-bio-count`}
                      placeholder="介绍一下自己"
                    />
                    <p
                      id={`${formFieldId}-bio-count`}
                      className="text-right text-xs text-muted-foreground"
                    >
                      {Array.from(bio).length > 100
                        ? "简介不能超过100字 · "
                        : ""}
                      {Array.from(bio).length}/100
                    </p>
                  </Field>
                  <Field>
                    <FieldLabel htmlFor={`${formFieldId}-field-2`}>
                      <span>邮箱</span>
                    </FieldLabel>
                    <Input
                      id={`${formFieldId}-field-2`}
                      value={profile.email}
                      readOnly
                    />
                  </Field>

                  <div className="flex justify-end gap-3">
                    <Button
                      type="button"
                      variant="ghost"
                      className="min-w-20"
                      disabled={pending}
                      onClick={() => {
                        setName(profile.display_name);
                        setBio(profile.bio ?? "");
                        if (embedded && initialView === "profile") onClose();
                        else setTab("home");
                      }}
                    >
                      取消
                    </Button>
                    <Button
                      className="min-w-20"
                      disabled={
                        pending ||
                        Array.from(bio).length > 100 ||
                        Array.from(name.trim()).length < 1 ||
                        Array.from(name.trim()).length > 32
                      }
                    >
                      {pending ? "正在保存…" : "保存资料"}
                    </Button>
                  </div>
                </FieldGroup>
              </form>
              <Collapsible className="mt-6 text-sm">
                <CollapsibleTrigger asChild>
                  <Button variant="ghost">更换邮箱</Button>
                </CollapsibleTrigger>
                <CollapsibleContent>
                  <EmailChange
                    revision={profile.revision}
                    onConflict={refresh}
                  />
                </CollapsibleContent>
              </Collapsible>
            </>
          ) : null
        ) : tab === "preferences" ? (
          loadingBlocks.preferences ? (
            <ContentState
              className="min-h-40"
              state="loading"
              title="正在读取创作偏好"
            />
          ) : blockErrors.preferences ? (
            <ContentState
              className="min-h-40"
              state="error"
              title="创作偏好暂时无法读取"
              description={blockErrors.preferences}
              actions={<Button onClick={refresh}>重试</Button>}
            />
          ) : preferences ? (
            <>
              <h2>创作偏好</h2>
              <form
                onSubmit={(e) => {
                  e.preventDefault();
                  void run(savePreferences);
                }}
              >
                <FieldGroup>
                  <Field data-disabled={pending}>
                    <FieldLabel htmlFor={`${formFieldId}-field-4`}>
                      <span>新草稿提示词后缀</span>
                    </FieldLabel>
                    <Textarea
                      id={`${formFieldId}-field-4`}
                      value={suffix}
                      onChange={(e) => setSuffix(e.target.value)}
                      disabled={pending}
                    />
                  </Field>
                  <p>保存不会覆盖已有草稿。</p>
                  <Button
                    className="w-fit self-end"
                    disabled={
                      pending ||
                      !preferences ||
                      Array.from(suffix).length > 2000
                    }
                  >
                    保存偏好
                  </Button>
                </FieldGroup>
              </form>
            </>
          ) : null
        ) : tab === "security" ? (
          <>
            <Button variant="outline" asChild>
              <Link href="/reset-password">修改密码</Link>
            </Button>
            <p>修改密码保留当前设备并退出其他设备。</p>
            <h3 className="mt-6">登录设备</h3>
            {loadingBlocks.sessions ? (
              <ContentState
                className="min-h-40"
                state="loading"
                title="正在读取登录设备"
              />
            ) : blockErrors.sessions ? (
              <ContentState
                className="min-h-40"
                state="error"
                title="登录设备暂时无法读取"
                description={blockErrors.sessions}
                actions={<Button onClick={refresh}>重试读取设备</Button>}
              />
            ) : sessions.length === 0 ? (
              <ContentState className="min-h-40" title="暂无登录设备" />
            ) : null}
            {!loadingBlocks.sessions &&
              !blockErrors.sessions &&
              sessions.map((item) => (
                <div
                  className="my-3 grid gap-2 rounded-lg bg-muted/50 p-4"
                  key={item.id}
                >
                  <p>
                    {item.current ? "当前设备 · " : ""}
                    {item.device_summary || "设备摘要不可用"}
                  </p>
                  <p>
                    最近活跃：
                    {new Date(item.last_active_at).toLocaleString("zh-CN")}
                  </p>
                  <Button
                    className="w-fit justify-self-end"
                    variant="secondary"
                    disabled={pending}
                    onClick={() =>
                      void run(async (signal) => {
                        if (item.id === session.session_id) {
                          await revokeSession(session);
                          return;
                        }
                        await deleteSession({ id: item.id }, { signal });
                        if (!signal.aborted) refresh();
                      })
                    }
                  >
                    退出此设备
                  </Button>
                </div>
              ))}
            <Button
              variant="outline"
              disabled={
                pending ||
                loadingBlocks.sessions ||
                !!blockErrors.sessions ||
                sessions.length === 0
              }
              onClick={() =>
                void run(async (signal) => {
                  await revokeOtherSessions({ signal });
                  if (!signal.aborted) refresh();
                })
              }
            >
              退出其他设备
            </Button>
            <h3 className="mt-6">本人安全活动</h3>
            {loadingBlocks.events ? (
              <ContentState
                className="min-h-40"
                state="loading"
                title="正在读取安全活动"
              />
            ) : blockErrors.events ? (
              <ContentState
                className="min-h-40"
                state="error"
                title="安全活动暂时无法读取"
                description={blockErrors.events}
                actions={<Button onClick={refresh}>重试安全活动</Button>}
              />
            ) : null}
            {loadingBlocks.events || blockErrors.events ? null : events?.items
                .length === 0 ? (
              <ContentState className="min-h-40" title="暂无安全活动" />
            ) : (
              events?.items.map((item) => (
                <p key={item.id} className="my-2 text-sm">
                  {item.action} · {item.result} ·{" "}
                  {new Date(item.created_at).toLocaleString("zh-CN")}
                </p>
              ))
            )}
            {!loadingBlocks.events &&
            !blockErrors.events &&
            events?.next_cursor ? (
              <Button
                variant="outline"
                disabled={pending}
                onClick={() =>
                  void run(async (signal) => {
                    const next = await getMySecurityEvents(
                      { cursor: events.next_cursor!, limit: 20 },
                      { signal },
                    );
                    if (!signal.aborted)
                      setEvents({
                        ...next,
                        items: [...events.items, ...next.items],
                      });
                  })
                }
              >
                加载更多活动
              </Button>
            ) : null}
          </>
        ) : (
          <>
            <h2>注销账号</h2>
            <p>
              确认后立即无法登录，资料、邮箱占位、作品和作者引用保留。团队成员资格与会话立即撤销，不自动转移资源。
            </p>
            <Button
              variant="outline"
              disabled={pending}
              onClick={() =>
                void run(async (signal) => {
                  const result = await previewAccountDeletion({ signal });
                  if (!signal.aborted) {
                    setImpact(result);
                    setOperation("");
                    setConsent(false);
                  }
                })
              }
            >
              检查注销影响
            </Button>
            {impact ? (
              <>
                {impact.blocking_duties.length ? (
                  <Alert variant="destructive">
                    <AlertDescription>
                      <p>请先交接以下管理员职责：</p>
                      {impact.blocking_duties.map((duty) => (
                        <p key={duty}>{duty}</p>
                      ))}
                    </AlertDescription>
                  </Alert>
                ) : (
                  <>
                    <p>
                      关联团队：{impact.team_ids.length}；策略：
                      {impact.policy_version}
                    </p>
                    <Field data-disabled={pending}>
                      <FieldLabel htmlFor={`${formFieldId}-field-5`}>
                        <span>当前密码</span>
                      </FieldLabel>
                      <Input
                        id={`${formFieldId}-field-5`}
                        type="password"
                        autoComplete="current-password"
                        value={password}
                        onChange={(e) => setPassword(e.target.value)}
                        disabled={pending}
                      />
                    </Field>
                    <Button
                      variant="outline"
                      disabled={pending || !password || Boolean(operation)}
                      onClick={() =>
                        void run(async (signal) => {
                          await reauthenticate({ password }, { signal });
                          const result = requireAccepted(
                            await startAccountDeletion({ signal }),
                          );
                          if (!signal.aborted) {
                            setPassword("");
                            setOperation(result.operation_id);
                            setNotice(
                              "验证码请求已受理，不代表邮件已送达。请在 10 分钟内输入验证码；密码复验有效期为 5 分钟。",
                            );
                          }
                        })
                      }
                    >
                      验证密码并发送验证码
                    </Button>
                    {operation ? (
                      <>
                        <Field data-disabled={pending}>
                          <FieldLabel htmlFor={`${formFieldId}-field-6`}>
                            <span>邮箱验证码</span>
                          </FieldLabel>
                          <Input
                            id={`${formFieldId}-field-6`}
                            inputMode="numeric"
                            autoComplete="one-time-code"
                            maxLength={6}
                            value={code}
                            onChange={(e) => setCode(e.target.value)}
                            disabled={pending}
                          />
                        </Field>
                        <Field
                          className="my-4"
                          data-disabled={pending}
                          orientation="horizontal"
                        >
                          <Checkbox
                            id={`${formFieldId}-field-7`}
                            checked={consent}
                            onCheckedChange={(v) => setConsent(v === true)}
                            disabled={pending}
                          />
                          <FieldLabel htmlFor={`${formFieldId}-field-7`}>
                            <span>我确认立即注销，理解资料与作品保留</span>
                          </FieldLabel>
                        </Field>
                        <Button
                          variant="destructive"
                          disabled={
                            pending || !consent || !/^\d{6}$/.test(code)
                          }
                          onClick={() =>
                            void run(async (signal) => {
                              await confirmAccountDeletion(
                                {
                                  expected_revision: impact.revision,
                                  operation_id: operation,
                                  code,
                                  policy_version: impact.policy_version,
                                },
                                { signal },
                              );
                              if (!signal.aborted) {
                                await clearPrivateSession();
                                router.replace("/login");
                              }
                            })
                          }
                        >
                          确认注销账号
                        </Button>
                      </>
                    ) : null}
                  </>
                )}
              </>
            ) : null}
          </>
        )}
      </div>
    </section>
  );
}
export function AccountPage() {
  return (
    <PageLayout screen="account">
      <Settings />
    </PageLayout>
  );
}
