"use client";
import { Save } from "lucide-react";
import { ContentState } from "@/components/layout/content-state";
import { readErrorMessage } from "@/lib/read-error";
import { Switch } from "@/components/ui/switch";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { useEffect, useState } from "react";
import { platformSettings, updatePlatformSettings } from "@/api/identity";
import {
  Confirmation,
  type ConfirmAction,
} from "@/components/governance/confirmation";
import { Button } from "@/components/ui/button";
import { useSession } from "@/components/auth/session";
export function PlatformSettingsPage() {
  const { actor } = useSession();
  if (actor.platform_role !== "platform_admin")
    return (
      <ContentState
        state="error"
        title="暂无访问权限"
        description="此页面仅向平台管理员开放。"
      />
    );
  return <PlatformSettingsAdministration key={actor.id} />;
}
function PlatformSettingsAdministration() {
  const [settings, setSettings] = useState<API.PlatformSettings | null>(null);
  const [registration, setRegistration] = useState(false);
  const [creation, setCreation] = useState(false);
  const [reload, setReload] = useState(0);
  const [error, setError] = useState("");
  const [action, setAction] = useState<ConfirmAction | null>(null);
  useEffect(() => {
    const c = new AbortController();
    platformSettings({ signal: c.signal })
      .then((s) => {
        if (!c.signal.aborted) {
          setSettings(s);
          setRegistration(Boolean(s.registration_enabled));
          setCreation(Boolean(s.team_creation_enabled));
          setError("");
        }
      })
      .catch((e) => {
        if (!c.signal.aborted) setError(readErrorMessage(e));
      });
    return () => c.abort();
  }, [reload]);
  return (
    <div className="admin-page">
      <h1 className="sr-only">平台设置</h1>
      {error ? (
        <ContentState
          state="error"
          title="平台设置暂时无法读取"
          description={error}
          actions={
            <Button
              onClick={() => {
                setError("");
                setSettings(null);
                setReload((n) => n + 1);
              }}
            >
              重试
            </Button>
          }
        />
      ) : null}
      {settings && !error ? (
        <form
          aria-label="平台规则"
          className="flex w-full max-w-xl flex-col items-start gap-4"
          onSubmit={(e) => {
            e.preventDefault();
            setAction({
              title: "保存平台设置",
              effect:
                "修改公开注册与团队创建资格。注册仍须满足邮箱验证和生效条款条件。",
              reason: true,
              run: (reason, signal) =>
                updatePlatformSettings(
                  {
                    expected_revision: settings.revision,
                    reason,
                    registration_enabled: registration,
                    team_creation_enabled: creation,
                  },
                  { signal },
                ),
            });
          }}
        >
          <FieldGroup className="gap-0 divide-y border-y">
            <Field orientation="horizontal" className="justify-between py-4">
              <FieldLabel htmlFor="platform-registration">
                允许公开注册
              </FieldLabel>
              <Switch
                id="platform-registration"
                checked={registration}
                onCheckedChange={setRegistration}
              />
            </Field>
            <Field orientation="horizontal" className="justify-between py-4">
              <FieldLabel htmlFor="platform-team-creation">
                允许创建团队
              </FieldLabel>
              <Switch
                id="platform-team-creation"
                checked={creation}
                onCheckedChange={setCreation}
              />
            </Field>
          </FieldGroup>
          <Button
            type="submit"
            disabled={
              registration === Boolean(settings.registration_enabled) &&
              creation === Boolean(settings.team_creation_enabled)
            }
          >
            <Save aria-hidden="true" data-icon="inline-start" />
            保存平台设置
          </Button>
        </form>
      ) : !error ? (
        <ContentState
          state="loading"
          loadingLayout="form"
          title="正在读取平台设置"
        />
      ) : null}
      <Confirmation
        key={action?.title ?? "closed"}
        action={action}
        onClose={() => setAction(null)}
        onDone={() => setReload((n) => n + 1)}
      />
    </div>
  );
}
