import { formatCurrencyMicros } from "@/lib/money";

export type BudgetSnapshot = Readonly<{
  limit_micros: number;
  settled_micros: number;
  reserved_micros: number;
  available_micros: number;
  is_overrun: boolean;
}>;

type BudgetCardProps = {
  budget: BudgetSnapshot;
};

const percentage = new Intl.NumberFormat("zh-CN", {
  maximumFractionDigits: 1,
});

export function BudgetCard({ budget }: BudgetCardProps) {
  const usedMicros = budget.settled_micros + budget.reserved_micros;
  const usedPercent =
    budget.limit_micros > 0 ? (usedMicros / budget.limit_micros) * 100 : 0;
  const barPercent = Math.min(100, Math.max(0, Math.round(usedPercent)));
  const isOverrun = budget.is_overrun || budget.available_micros < 0;
  const isLow =
    !isOverrun &&
    budget.limit_micros > 0 &&
    budget.available_micros < budget.limit_micros / 5;

  return (
    <section
      aria-label="项目预算"
      className="space-y-6 rounded-2xl bg-muted/65 p-6 sm:p-8"
    >
      <div>
        <p className="font-mono text-xs font-medium tracking-[0.18em] text-muted-foreground">
          PROJECT BUDGET
        </p>
        <h2 className="mt-3 text-2xl font-semibold tracking-tight">项目预算</h2>
        <p className="mt-2 text-sm text-muted-foreground">
          可用额按预算上限扣除已结算和已预留费用计算。
        </p>
      </div>

      {isOverrun ? (
        <p
          role="status"
          className="rounded-xl bg-destructive/10 px-4 py-3 text-sm text-destructive"
        >
          <strong className="font-semibold">已超支</strong>
          <span className="ml-2">
            请核对成本并调整预算；新的付费确认将被拦截。
          </span>
        </p>
      ) : isLow ? (
        <p
          role="status"
          className="rounded-xl bg-amber-500/10 px-4 py-3 text-sm text-amber-900 dark:text-amber-200"
        >
          <strong className="font-semibold">预算余额不足</strong>
          <span className="ml-2">
            可用额低于预算上限的 20%，请留意后续生成费用。
          </span>
        </p>
      ) : budget.limit_micros === 0 ? (
        <p
          role="status"
          className="rounded-xl bg-background/75 px-4 py-3 text-sm text-muted-foreground"
        >
          <strong className="font-medium text-foreground">未设置预算</strong>
          <span className="ml-2">付费报价暂不可确认，请先设置项目预算。</span>
        </p>
      ) : null}

      <dl className="grid grid-cols-2 gap-x-6 gap-y-5 text-sm sm:grid-cols-4">
        <div>
          <dt className="text-muted-foreground">预算上限</dt>
          <dd className="mt-1 font-semibold tabular-nums">
            {formatCurrencyMicros(budget.limit_micros)}
          </dd>
        </div>
        <div>
          <dt className="text-muted-foreground">已结算</dt>
          <dd className="mt-1 font-semibold tabular-nums">
            {formatCurrencyMicros(budget.settled_micros)}
          </dd>
        </div>
        <div>
          <dt className="text-muted-foreground">已预留</dt>
          <dd className="mt-1 font-semibold tabular-nums">
            {formatCurrencyMicros(budget.reserved_micros)}
          </dd>
        </div>
        <div>
          <dt className="text-muted-foreground">可用额</dt>
          <dd
            className={`mt-1 font-semibold tabular-nums ${isOverrun ? "text-destructive" : ""}`}
          >
            {formatCurrencyMicros(budget.available_micros)}
          </dd>
        </div>
      </dl>

      <div className="space-y-2">
        <div className="flex items-center justify-between gap-4 text-sm">
          <span className="text-muted-foreground">预算使用率</span>
          <span className="font-medium tabular-nums">
            {percentage.format(usedPercent)}%
          </span>
        </div>
        <div
          role="progressbar"
          aria-label="预算使用率"
          aria-valuemin={0}
          aria-valuemax={100}
          aria-valuenow={barPercent}
          aria-valuetext={`已占用 ${percentage.format(usedPercent)}%`}
          className="h-2 overflow-hidden rounded-full bg-background/80"
        >
          <div
            className={`h-full rounded-full ${isOverrun ? "bg-destructive" : isLow ? "bg-amber-500" : "bg-foreground"}`}
            style={{ width: `${barPercent}%` }}
          />
        </div>
      </div>
    </section>
  );
}
