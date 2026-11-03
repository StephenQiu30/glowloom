"use client";

import { useId, useState } from "react";
import useSWR from "swr";
import dynamic from "next/dynamic";
import Link from "next/link";
import {
  getAdminModel,
  listModelReleases,
  listProviderTestModels,
  getProviderOnboarding,
} from "@/api/settings";
import { useSession } from "@/components/auth/session";
import { catalogMessage } from "@/components/catalog/mutation";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import { ConnectionOnboarding } from "./connection-onboarding";
import { checkRunning } from "./testing-status";
const ProviderTrial = dynamic(() =>
  import("@/components/catalog/provider-trial").then((m) => m.ProviderTrial),
);

export function ConnectionModels({
  id,
  revision,
  onSaved,
  initialModel,
  initialRelease,
  initialMode,
  adapter,
  checks,
  onChecked,
}: {
  id: string;
  revision: number;
  onSaved: () => void;
  initialModel?: string;
  initialRelease?: string;
  initialMode?: string;
  adapter?: string;
  checks?: API.ProviderCheck[];
  onChecked?: () => void;
}) {
  const { actor } = useSession();
  const fieldID = useId();
  const [selected, setSelected] = useState(initialModel ?? "");
  const { data, error, isLoading, mutate } = useSWR(
    ["provider-test-models", actor.id, id, revision],
    () => listProviderTestModels({ id }),
  );
  const { data: onboarding } = useSWR(
    ["provider-onboarding", actor.id, id, revision],
    () => getProviderOnboarding({ id }),
  );
  const target =
    data?.find((m) => m.id === selected) ??
    data?.find((m) => m.capability === "text.generate") ??
    data?.[0];
  const selectorLocked =
    checkRunning(onboarding?.check) ||
    (onboarding?.check?.model_result === "unknown" &&
      !onboarding.check.tracking_ended_at);
  const selector = (locked = false) => (
    <Select
      value={target?.id ?? ""}
      disabled={selectorLocked || locked}
      onValueChange={(value) => {
        setSelected(value);
        const url = new URL(window.location.href);
        url.searchParams.set("model", value);
        url.searchParams.delete("release");
        url.searchParams.delete("mode");
        window.history.replaceState(null, "", url);
      }}
    >
      <SelectTrigger
        id={fieldID}
        aria-label="模型验证"
        className="min-w-0 flex-1 max-md:min-h-11"
      >
        <SelectValue placeholder="选择模型" />
      </SelectTrigger>
      <SelectContent>
        {data?.map((m) =>
          m.id ? (
            <SelectItem key={m.id} value={m.id}>
              {m.display_name}
            </SelectItem>
          ) : null,
        )}
      </SelectContent>
    </Select>
  );
  if (isLoading)
    return (
      <p role="status" className="text-sm">
        正在读取模型…
      </p>
    );
  if (error)
    return (
      <div className="space-y-2 text-sm">
        <p>{catalogMessage(error)}</p>
        <Button variant="outline" onClick={() => void mutate()}>
          重试读取模型
        </Button>
      </div>
    );
  if (!target)
    return (
      <div className="space-y-2 text-sm">
        <h3 className="font-medium">模型验证</h3>
        {onboarding?.models?.map((model) => (
          <p key={model.id} className="text-sm">
            {model.display_name}
            <span className="ml-2 text-muted-foreground">
              价格待审核，尚不可创作。请在模型管理中发布已审核价格并完成验证。
            </span>
          </p>
        ))}
        <p className="text-xs text-muted-foreground">
          当前连接没有可测试的发布，请先在模型管理中完成配置。
        </p>
        <Button variant="outline" asChild>
          <Link href={`/models?provider=${encodeURIComponent(id)}`}>
            模型管理
          </Link>
        </Button>
      </div>
    );
  const pendingCandidates = onboarding?.models?.filter(
    (model) =>
      !model.release_id ||
      model.release_id === "00000000-0000-0000-0000-000000000000",
  );
  const pendingPrices = pendingCandidates?.length ? (
    <div className="flex flex-col gap-2 text-sm">
      {pendingCandidates.map((model) => (
        <p key={model.id}>{model.display_name}</p>
      ))}
      <p className="text-xs text-muted-foreground">
        价格待审核，尚不可创作。请在模型管理中发布已审核价格并完成验证。
      </p>
      <Button variant="outline" asChild>
        <Link href={`/models?provider=${encodeURIComponent(id)}`}>
          模型管理
        </Link>
      </Button>
    </div>
  ) : null;
  if (!target.requires_trial)
    return (
      <>
        {pendingPrices}
        <ConnectionOnboarding
          adapter={adapter}
          id={id}
          revision={revision}
          onSaved={onSaved}
          modelID={target.id}
          selector={selector}
          checks={checks}
          onChecked={onChecked}
        />
      </>
    );
  return (
    <>
      {pendingPrices}
      <FormalModelTest
        key={target.release_id}
        provider={id}
        target={target}
        selector={selector()}
        releaseID={target.id === initialModel ? initialRelease : undefined}
        mode={target.id === initialModel ? initialMode : undefined}
        onSaved={onSaved}
      />
    </>
  );
}

function FormalModelTest({
  provider,
  target,
  selector,
  releaseID,
  mode,
  onSaved,
}: {
  provider: string;
  target: API.ProviderTestModel;
  selector: React.ReactNode;
  releaseID?: string;
  mode?: string;
  onSaved: () => void;
}) {
  const { actor } = useSession();
  const { data, error, isLoading, mutate } = useSWR(
    [
      "provider-formal-test",
      actor.id,
      provider,
      target.id,
      target.version_id,
      releaseID,
    ],
    async () => {
      const [detail, releases] = await Promise.all([
        getAdminModel({ id: target.id! }),
        listModelReleases({ id: target.id! }),
      ]);
      if (detail.model?.provider_id !== provider)
        throw new Error("模型不属于当前连接。");
      const release = releases.items?.find(
        (r) =>
          r.id === (releaseID ?? target.release_id) &&
          r.model_id === target.id &&
          r.model_version_id === target.version_id &&
          ["draft", "active"].includes(r.status ?? ""),
      );
      const version = detail.versions?.find((v) => v.id === target.version_id);
      if (!release || !version)
        throw new Error("发布或模型版本已变化，请重新选择。");
      return { release, version };
    },
  );
  return (
    <div className="flex min-w-0 flex-col gap-3">
      <div className="flex items-center justify-between gap-2">
        <h3 className="text-sm font-medium">模型验证</h3>
        <Badge variant="secondary">逐模式试运行</Badge>
      </div>
      {selector}
      {isLoading ? (
        <p role="status" className="text-xs text-muted-foreground">
          正在读取发布…
        </p>
      ) : error ? (
        <div className="space-y-2 text-sm">
          <p>{catalogMessage(error)}</p>
          <Button variant="outline" onClick={() => void mutate()}>
            刷新发布
          </Button>
        </div>
      ) : data ? (
        <ProviderTrial
          release={data.release}
          version={data.version}
          initialMode={mode}
          embedded
          onVerified={() => {
            onSaved();
            void mutate();
          }}
        />
      ) : null}
    </div>
  );
}
