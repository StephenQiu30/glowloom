import Link from "next/link";
import { PageHeader } from "@/components/layout/page-header";
import { PageLayout } from "@/components/layout/page-layout";
import { ContentState } from "@/components/layout/content-state";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";

import { billingSections, type BillingSection } from "./billing-sections";

import { payerLabel, presentMovement } from "./wallet-presentation";

function TransactionRow({ entry }: { entry: API.WalletEntry }) {
  const { label, purpose, amount, time } = presentMovement(entry);
  return (
    <li className="grid grid-cols-[minmax(0,1fr)_minmax(0,max-content)] items-start gap-x-3 gap-y-1 px-4 py-3 text-sm">
      <span>{label}</span>
      <span className="text-right wrap-anywhere tabular-nums">{amount}</span>
      <span className="truncate text-muted-foreground" title={purpose}>
        {purpose}
      </span>
      {time && (
        <time
          dateTime={entry.created_at}
          className="text-right whitespace-nowrap text-muted-foreground tabular-nums"
        >
          {time}
        </time>
      )}
      {entry.source === "promotional" && entry.id && (
        <span className="col-span-2 text-xs wrap-anywhere text-muted-foreground">
          流水号 {entry.id}
        </span>
      )}
    </li>
  );
}

export function BillingView({
  section = "overview",
  wallet,
  walletLoading,
  walletError,
  onNext,
  onPrevious,
  sizeControl,
  onRetry,
}: {
  section?: BillingSection;
  wallet?: API.WalletView;
  walletLoading?: boolean;
  walletError?: boolean;
  onNext?: () => void;
  sizeControl?: React.ReactNode;
  onPrevious?: () => void;
  onRetry?: () => void;
}) {
  const activeWallet = wallet?.enabled ? wallet : undefined;
  const secondaryBalances = [
    { label: "预占积分", value: activeWallet?.held_credits },
    { label: "今日消耗", value: activeWallet?.redeemed_today },
  ];
  const balanceState = walletLoading
    ? "正在读取积分…"
    : walletError
      ? "积分暂不可读取"
      : "积分服务暂未开放";
  const retry = walletError && !walletLoading && onRetry && (
    <Button size="workspace" variant="outline" onClick={onRetry}>
      重新读取
    </Button>
  );
  return (
    <PageLayout screen="billing" fullBleed>
      <section className="billing-page flex flex-col gap-(--layout-gap)">
        <PageHeader title={billingSections[section].title} />
        {section === "overview" ? (
          <Card className="max-w-2xl">
            <CardHeader>
              <CardTitle>可用积分</CardTitle>
              <CardDescription role="status">
                {activeWallet ? payerLabel(activeWallet.payer) : balanceState}
              </CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col gap-4">
              {walletLoading ? (
                <Skeleton aria-hidden="true" className="h-10 w-28" />
              ) : (
                <p
                  className="text-4xl break-all tabular-nums"
                  aria-label={activeWallet ? undefined : "可用积分暂不可查询"}
                >
                  {activeWallet?.general_credits ?? "—"}
                </p>
              )}
              <dl className="grid grid-cols-2 gap-4">
                {secondaryBalances.map(({ label, value }) => (
                  <div key={label} className="flex min-w-0 flex-col gap-1">
                    <dt className="text-muted-foreground">{label}</dt>
                    <dd
                      aria-label={
                        value === undefined && !walletLoading
                          ? `${label}暂不可查询`
                          : undefined
                      }
                      className="break-all tabular-nums"
                    >
                      {walletLoading ? (
                        <Skeleton aria-hidden="true" className="h-5 w-16" />
                      ) : (
                        (value ?? "—")
                      )}
                    </dd>
                  </div>
                ))}
              </dl>
            </CardContent>
            {activeWallet?.restricted_credits &&
            activeWallet.restricted_credits !== "0" ? (
              <CardFooter className="break-all tabular-nums">
                限模型积分 {activeWallet.restricted_credits}
              </CardFooter>
            ) : retry ? (
              <CardFooter>{retry}</CardFooter>
            ) : null}
          </Card>
        ) : section === "transactions" ? (
          activeWallet?.history_available ? (
            <div className="flex max-w-2xl flex-col gap-4">
              {activeWallet.entries?.length ? (
                <Card className="rounded-lg py-0">
                  <CardContent className="px-0">
                    <ul className="divide-y" aria-label="积分流水">
                      {activeWallet.entries.map((entry) => (
                        <TransactionRow key={entry.id} entry={entry} />
                      ))}
                    </ul>
                  </CardContent>
                </Card>
              ) : (
                <ContentState title="暂无积分流水" />
              )}
              {(sizeControl || onPrevious || onNext) && (
                <div className="flex gap-2">
                  {sizeControl}
                  {onPrevious && (
                    <Button
                      size="workspace"
                      variant="outline"
                      onClick={onPrevious}
                    >
                      上一页
                    </Button>
                  )}
                  {onNext && (
                    <Button size="workspace" variant="outline" onClick={onNext}>
                      下一页
                    </Button>
                  )}
                </div>
              )}
            </div>
          ) : (
            <ContentState
              className="max-w-2xl"
              title={
                activeWallet ? "积分明细由团队付款管理员查看" : balanceState
              }
              state={
                walletLoading ? "loading" : walletError ? "error" : "empty"
              }
              loadingLayout="list"
              actions={retry}
            />
          )
        ) : (
          <ContentState
            className="max-w-2xl"
            title={
              section === "membership" ? "会员计划即将上线" : "充值即将开放"
            }
            actions={
              <Button size="workspace" variant="secondary" asChild>
                <Link href={billingSections.overview.href}>返回积分总览</Link>
              </Button>
            }
          />
        )}
      </section>
    </PageLayout>
  );
}
