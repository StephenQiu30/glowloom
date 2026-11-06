"use client";

import Link from "next/link";
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
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { Spinner } from "@/components/ui/spinner";
import {
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

function missingModes(release: API.OnboardingReleaseReview) {
  return (release.required_modes ?? []).filter(
    (mode) => !release.validated_modes?.includes(mode),
  );
}

export function ConnectionReleaseReview({
  id,
  titleId,
  onClose,
  onSaved,
}: {
  id: string;
  titleId: string;
  onClose: () => void;
  onSaved: () => void;
}) {
  const { actor } = useSession();
  const { data, error, isLoading, mutate } = useSWR(
    ["provider-release-review", actor.id, id],
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
  const canActivate = ready && data?.status !== "active";
  const needsReview = reviews.some(
    (release) =>
      !release.required_modes?.length || missingModes(release).length > 0,
  );

  return (
    <>
      <DialogHeader className="gap-1">
        <DialogTitle id={titleId} tabIndex={-1} size="lg" className="text-xl">
          发布复核
        </DialogTitle>
        <DialogDescription id={`${titleId}-description`}>
          必验模式全部通过后，才可启用对应发布。
        </DialogDescription>
      </DialogHeader>
      <div className="flex min-h-0 flex-col gap-4 overflow-y-auto p-px">
        {isLoading && (
          <p
            role="status"
            className="flex items-center gap-2 text-sm text-muted-foreground"
          >
            <Spinner aria-hidden="true" />
            正在读取发布证据…
          </p>
        )}
        {error && (
          <p role="alert">
            {catalogMessage(error)}{" "}
            <Button variant="link" onClick={() => void mutate()}>
              重新读取
            </Button>
          </p>
        )}
        {data && reviews.length === 0 && (
          <p className="text-sm text-muted-foreground">
            暂无批量接入发布需要复核。
          </p>
        )}
        {reviews.map((release) => {
          const missing = missingModes(release);
          const modes = release.required_modes ?? [];
          return (
            <article
              key={release.id}
              className="flex flex-col gap-2.5 rounded-lg border p-3 text-sm"
            >
              <div className="flex flex-wrap items-center justify-between gap-2">
                <h4 className="min-w-0 font-semibold break-words">
                  {release.display_name}
                </h4>
                {modes.length > 0 && (
                  <Badge variant="outline">
                    {modes.length - missing.length} / {modes.length} 已验证
                  </Badge>
                )}
              </div>
              <p className="text-xs text-muted-foreground">
                {release.onboarding_check_id ? "历史批量接入" : "待启用发布"} ·{" "}
                {release.status === "active"
                  ? "当前发布"
                  : release.status === "retired"
                    ? "已退役"
                    : "草稿"}
              </p>
              {modes.length > 0 ? (
                <>
                  <Separator />
                  <ul className="flex flex-col gap-2" aria-label="必验模式">
                    {modes.map((mode) => (
                      <li
                        key={mode}
                        className="flex items-center justify-between gap-2"
                      >
                        <span className="min-w-0 text-sm break-all">
                          {mode}
                        </span>
                        <Badge
                          variant={
                            missing.includes(mode) ? "outline" : "secondary"
                          }
                        >
                          {missing.includes(mode) ? "待复核" : "已验证"}
                        </Badge>
                      </li>
                    ))}
                  </ul>
                </>
              ) : (
                <p>缺少模式合同。</p>
              )}
              <Button
                asChild
                type="button"
                variant="ghost"
                className="self-start max-md:min-h-11"
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
        {needsReview && (
          <Alert>
            <AlertTitle>尚有模式待复核</AlertTitle>
            <AlertDescription>
              本次查看保留现有发布及已冻结任务。
            </AlertDescription>
          </Alert>
        )}
        {(activation.pending || activation.message) && (
          <MutationStatus mutation={activation} inline />
        )}
      </div>
      <Separator />
      <DialogFooter className="shrink-0 pt-0 max-md:[&_button]:min-h-11">
        <Button type="button" variant="ghost" onClick={onClose}>
          关闭
        </Button>
        {reviews.length > 0 && (
          <Button
            type="button"
            disabled={!canActivate || activation.busy || !!activation.pending}
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
            {canActivate ? `启用 ${drafts.length} 个已复核模型` : "批量启用"}
          </Button>
        )}
      </DialogFooter>
    </>
  );
}
