"use client";

import Link from "next/link";
import { useState } from "react";
import useSWR from "swr";
import {
  activateProviderOnboarding,
  getProviderOnboarding,
} from "@/api/settings";
import { useSession } from "@/components/auth/session";
import {
  catalogMessage,
  MutationStatus,
  useCatalogMutation,
} from "@/components/catalog/mutation";
import { Button } from "@/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";

function missingModes(release: API.OnboardingReleaseReview) {
  return (release.required_modes ?? []).filter(
    (mode) => !release.validated_modes?.includes(mode),
  );
}

export function ConnectionReleaseReview({
  id,
  onSaved,
}: {
  id: string;
  onSaved: () => void;
}) {
  const [open, setOpen] = useState(false);
  const { actor } = useSession();
  const { data, error, isLoading, mutate } = useSWR(
    open ? ["provider-release-review", actor.id, id] : null,
    () => getProviderOnboarding({ id }),
  );
  const activation = useCatalogMutation(
    () => {
      void mutate();
      onSaved();
    },
    `provider.onboarding.activate.${id}`,
    (value, target) => {
      const result = value as API.ProviderOnboardingState | undefined;
      return result?.provider_id === target && result.status === "active";
    },
  );
  const reviews = data?.release_reviews ?? [];
  const drafts = reviews.filter((release) => release.batch_draft);
  const ready =
    drafts.length > 0 &&
    drafts.every(
      (release) =>
        !!release.required_modes?.length && missingModes(release).length === 0,
    );
  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <CollapsibleTrigger asChild>
        <Button variant="outline" type="button">
          发布复核
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent className="flex flex-col gap-3 pt-3">
        {isLoading && <p role="status">正在读取发布证据…</p>}
        {error && (
          <p role="alert">
            {catalogMessage(error)}{" "}
            <Button variant="link" onClick={() => void mutate()}>
              重新读取
            </Button>
          </p>
        )}
        {data && reviews.length === 0 && <p>暂无批量接入发布需要复核。</p>}
        {reviews.map((release) => {
          const missing = missingModes(release);
          return (
            <article
              key={release.id}
              className="flex flex-col gap-1 rounded-md border p-3 text-sm"
            >
              <h4 className="font-medium">{release.display_name}</h4>
              <p>
                {release.onboarding_check_id ? "历史批量接入" : "待启用发布"} ·{" "}
                {release.status === "active"
                  ? "当前发布"
                  : release.status === "retired"
                    ? "已退役"
                    : "草稿"}
              </p>
              <ul>
                {(release.required_modes ?? []).map((mode) => (
                  <li key={mode}>
                    {mode} · {missing.includes(mode) ? "待复核" : "已验证"}
                  </li>
                ))}
              </ul>
              {!release.required_modes?.length && <p>缺少模式合同。</p>}
              <Button
                asChild
                type="button"
                variant="link"
                className="self-start px-0"
              >
                <Link
                  href={`/models?provider=${encodeURIComponent(id)}&model=${encodeURIComponent(release.model_id ?? "")}`}
                >
                  查看模型发布
                </Link>
              </Button>
            </article>
          );
        })}
        {drafts.length > 0 && !ready && (
          <p>每个模型的所有必验模式通过后才可批量启用。</p>
        )}
        {ready && data?.status !== "active" && (
          <Button
            type="button"
            disabled={activation.busy || !!activation.pending}
            className="self-start"
            onClick={() =>
              void activation.run(
                "provider.onboarding.activate",
                id,
                (requestID) =>
                  activateProviderOnboarding(
                    { id },
                    {
                      request_id: requestID,
                      expected_revision: data?.revision,
                      release_ids: drafts.flatMap((release) =>
                        release.id ? [release.id] : [],
                      ),
                    },
                  ),
              )
            }
          >
            启用 {drafts.length} 个已复核模型
          </Button>
        )}
        {(activation.pending || activation.message) && (
          <MutationStatus mutation={activation} inline />
        )}
      </CollapsibleContent>
    </Collapsible>
  );
}
