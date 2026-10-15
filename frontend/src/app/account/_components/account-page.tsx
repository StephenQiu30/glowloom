"use client";
import { LoadingState } from "@/components/layout/loading-state";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Field, FieldLabel, FieldGroup } from "@/components/ui/field";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import { useEffect, useRef, useState, useId } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useTheme } from "next-themes";
import { AvatarEditor } from "@/components/account/avatar-editor";
import { getMyAvatar } from "@/api/media";
import { EmailChange } from "./email-change";
import { ProductShell } from "@/components/layout/product-shell";
import {
  useSession,
  useSessionUpdate,
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
import { Checkbox } from "@/components/ui/checkbox";

export function Settings() {
  const formFieldId = useId();

  const session = useSession();
  const updateSession = useSessionUpdate();
  const router = useRouter();
  const { setTheme } = useTheme();
  const [tab, setTab] = useState("profile");
  const [profile, setProfile] = useState<API.MeView | null>(null);
  const [preferences, setPreferences] = useState<API.PreferencesView | null>(
    null,
  );
  const [name, setName] = useState("");
  const [theme, changeTheme] = useState("system");
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
              [block]: authErrorMessage(reason),
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
    });
    load(
      "preferences",
      getMyPreferences({ signal: controller.signal }),
      (prefs) => {
        setPreferences(prefs);
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
  }, [session.actor.id, reload]);
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
      { expected_revision: profile.revision, display_name: name.trim() },
      { signal },
    );
    const readBack = await getMe({ signal });
    if (signal.aborted) return;
    if (updated.revision !== readBack.revision) {
      setError("资料保存后又发生变化，请重新加载。");
      return;
    }
    setProfile(readBack);
    setPreferences((current) =>
      current ? { ...current, revision: readBack.revision } : current,
    );
    setImpact(null);
    setOperation("");
    setName(readBack.display_name);
    updateSession({ ...session, actor: readBack });
    setNotice("个人资料已保存并读回。");
  }
  async function savePreferences(signal: AbortSignal) {
    if (!preferences) return;
    await updateMyPreferences(
      { expected_revision: preferences.revision, theme, prompt_suffix: suffix },
      { signal },
    );
    const [readBack, p] = await Promise.all([
      getMyPreferences({ signal }),
      getMe({ signal }),
    ]);
    if (signal.aborted) return;
    setPreferences(readBack);
    changeTheme(readBack.theme);
    setSuffix(readBack.prompt_suffix);
    setProfile(p);
    setTheme(readBack.theme);
    updateSession({ ...session, actor: p });
    setNotice("创作偏好已保存并读回。提示词后缀仅应用于新草稿。");
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
  return (
    <>
      <h1 className="text-2xl">账号设置</h1>
      <div className="settings-layout">
        <nav className="settings-nav" aria-label="账号设置">
          {[
            ["profile", "个人资料"],
            ["security", "密码与会话"],
            ["preferences", "创作偏好"],
            ["deletion", "注销账号"],
          ].map(([id, label]) => (
            <Button
              key={id}
              variant="ghost"
              aria-current={tab === id ? "page" : undefined}
              onClick={() => setTab(id)}
            >
              {label}
            </Button>
          ))}
        </nav>
        <section className="settings-card">
          {error ? (
            <Alert variant="destructive">
              <AlertDescription>
                <p>{error}</p>
                <Button variant="outline" onClick={refresh} disabled={pending}>
                  重新加载
                </Button>
              </AlertDescription>
            </Alert>
          ) : null}
          {notice ? <p role="status">{notice}</p> : null}
          {tab === "profile" && blockErrors.profile ? (
            <Alert variant="destructive">
              <AlertDescription>
                个人资料读取失败：{blockErrors.profile}
                <Button variant="outline" onClick={refresh}>
                  重新加载个人资料
                </Button>
              </AlertDescription>
            </Alert>
          ) : null}
          {tab === "preferences" && blockErrors.preferences ? (
            <Alert variant="destructive">
              <AlertDescription>
                创作偏好读取失败：{blockErrors.preferences}
                <Button variant="outline" onClick={refresh}>
                  重新加载创作偏好
                </Button>
              </AlertDescription>
            </Alert>
          ) : null}
          {tab === "profile" && profile ? (
            <EmailChange revision={profile.revision} onConflict={refresh} />
          ) : null}
          {tab === "profile" ? (
            profile ? (
              <>
                <h2>个人资料</h2>
                <AvatarEditor
                  assetID={profile.avatar_asset_id}
                  load={(signal) => getMyAvatar({ signal })}
                  onConflict={async () => {
                    const readBack = await getMe();
                    setProfile(readBack);
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

                    <Button
                      disabled={
                        pending ||
                        Array.from(name.trim()).length < 1 ||
                        Array.from(name.trim()).length > 32
                      }
                    >
                      保存资料
                    </Button>
                  </FieldGroup>
                </form>
              </>
            ) : loadingBlocks.profile ? (
              <LoadingState>正在加载账号资料…</LoadingState>
            ) : null
          ) : tab === "preferences" ? (
            preferences ? (
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
                      <FieldLabel htmlFor={`${formFieldId}-field-3`}>
                        <span>外观</span>
                      </FieldLabel>
                      <NativeSelect
                        id={`${formFieldId}-field-3`}
                        value={theme}
                        onChange={(e) => changeTheme(e.target.value)}
                        disabled={pending}
                      >
                        <NativeSelectOption value="system">
                          跟随系统
                        </NativeSelectOption>
                        <NativeSelectOption value="light">
                          浅色
                        </NativeSelectOption>
                        <NativeSelectOption value="dark">
                          深色
                        </NativeSelectOption>
                      </NativeSelect>
                    </Field>
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
            ) : loadingBlocks.preferences ? (
              <LoadingState>正在加载创作偏好…</LoadingState>
            ) : null
          ) : tab === "security" ? (
            <>
              <h2>密码与会话</h2>
              <Button variant="outline" asChild>
                <Link href="/reset-password">修改密码</Link>
              </Button>
              <p>修改密码保留当前设备并退出其他设备。</p>
              <h3 className="mt-6">登录设备</h3>
              {loadingBlocks.sessions ? (
                <LoadingState>正在读取设备…</LoadingState>
              ) : blockErrors.sessions ? (
                <Alert variant="destructive">
                  <AlertDescription>
                    设备读取失败：{blockErrors.sessions}
                    <Button variant="outline" onClick={refresh}>
                      重试读取设备
                    </Button>
                  </AlertDescription>
                </Alert>
              ) : sessions.length === 0 ? (
                <p>暂无登录设备。</p>
              ) : null}
              {sessions.map((item) => (
                <div className="my-3 rounded-lg border p-4" key={item.id}>
                  <p>
                    {item.current ? "当前设备 · " : ""}
                    {item.device_summary || "设备摘要不可用"}
                  </p>
                  <p>
                    最近活跃：
                    {new Date(item.last_active_at).toLocaleString("zh-CN")}
                  </p>
                  <Button
                    variant="outline"
                    disabled={pending}
                    onClick={() =>
                      void run(async (signal) => {
                        await deleteSession({ id: item.id }, { signal });
                        if (signal.aborted) return;
                        if (item.current) {
                          await clearPrivateSession();
                          router.replace("/login");
                        } else refresh();
                      })
                    }
                  >
                    退出此设备
                  </Button>
                </div>
              ))}
              <Button
                variant="outline"
                disabled={pending}
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
                <LoadingState>正在读取安全活动…</LoadingState>
              ) : blockErrors.events ? (
                <Alert variant="destructive">
                  <AlertDescription>
                    安全活动读取失败：{blockErrors.events}
                    <Button variant="outline" onClick={refresh}>
                      重试安全活动
                    </Button>
                  </AlertDescription>
                </Alert>
              ) : null}
              {events?.items.length === 0 ? (
                <p>暂无安全活动。</p>
              ) : (
                events?.items.map((item) => (
                  <p key={item.id} className="my-2 text-sm">
                    {item.action} · {item.result} ·{" "}
                    {new Date(item.created_at).toLocaleString("zh-CN")}
                  </p>
                ))
              )}
              {events?.next_cursor ? (
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
        </section>
      </div>
    </>
  );
}
export function AccountPage() {
  return (
    <ProductShell screen="account">
      <Settings />
    </ProductShell>
  );
}
