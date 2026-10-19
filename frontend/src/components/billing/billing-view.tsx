import Link from "next/link";
import { PageLayout } from "@/components/layout/page-layout";
import { ProductFooter } from "@/components/layout/product-footer";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
  CardFooter,
} from "@/components/ui/card";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import {
  Empty,
  EmptyHeader,
  EmptyTitle,
  EmptyDescription,
} from "@/components/ui/empty";

const sections = {
  overview: {
    title: "积分总览",
    href: "/billing",
    description: "查看余额、会员状态与最近消费。",
  },
  membership: {
    title: "会员中心",
    href: "/billing/membership",
    description: "管理你的会员状态。",
  },
  transactions: {
    title: "积分明细",
    href: "/billing/transactions",
    description: "预占、结算与返还分别记录。",
  },
  recharge: {
    title: "充值积分",
    href: "/billing/recharge",
    description: "查看可购买的积分套餐。",
  },
} as const;
type Section = keyof typeof sections;

function BillingAvailability({
  section,
}: {
  section: "membership" | "recharge";
}) {
  const member = section === "membership";
  return (
    <Card>
      <CardHeader>
        <CardTitle>{member ? "会员计划即将上线" : "充值即将开放"}</CardTitle>
        <CardDescription>
          {member
            ? "套餐价格、赠送积分与权益将在开放时公布。"
            : "充值套餐尚未发布。开放后可在这里查看积分数量、价格与有效期。"}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <p>暂未开放</p>
      </CardContent>
      <CardFooter className="flex-wrap gap-3">
        <Button disabled>暂未开放</Button>
        <Button variant="outline" asChild>
          <Link href="/billing">返回积分总览</Link>
        </Button>
      </CardFooter>
    </Card>
  );
}

/** Only the published availability is rendered; prototype totals never become account data. */
export function BillingView({ section = "overview" }: { section?: Section }) {
  const current = sections[section];
  return (
    <PageLayout screen="billing" fullBleed>
      <section className="billing-page">
        <Tabs value={section} activationMode="manual" className="gap-6">
          <div className="overflow-x-auto">
            <TabsList variant="navigation" aria-label="账务导航">
              {(["overview", "membership", "transactions"] as const).map(
                (key) => (
                  <TabsTrigger key={key} value={key} asChild>
                    <Link href={sections[key].href}>{sections[key].title}</Link>
                  </TabsTrigger>
                ),
              )}
            </TabsList>
          </div>
          <TabsContent value={section} className="flex flex-col gap-6">
            <div className="billing-heading">
              <h1>{current.title}</h1>
              <p>{current.description}</p>
            </div>
            {section === "overview" ? (
              <>
                <div className="billing-metrics">
                  {["可用积分", "预占积分", "今日消耗"].map((label) => (
                    <Card key={label}>
                      <CardHeader>
                        <CardTitle>{label}</CardTitle>
                        <CardDescription>积分服务暂未开放</CardDescription>
                      </CardHeader>
                      <CardContent>
                        <p
                          className="billing-metric"
                          aria-label={`${label}暂不可查询`}
                        >
                          —
                        </p>
                      </CardContent>
                    </Card>
                  ))}
                </div>
                <div className="flex flex-wrap gap-3">
                  <Button asChild>
                    <Link href="/billing/recharge">充值积分</Link>
                  </Button>
                  <Button asChild variant="outline">
                    <Link href="/billing/transactions">查看明细</Link>
                  </Button>
                </div>
                <Card>
                  <CardHeader>
                    <CardTitle>会员计划正在准备中</CardTitle>
                    <CardDescription>
                      价格和权益将在开放时公布。
                    </CardDescription>
                  </CardHeader>
                  <CardContent>
                    <p>暂未开放</p>
                  </CardContent>
                  <CardFooter>
                    <Button asChild variant="outline">
                      <Link href="/billing/membership">查看会员中心</Link>
                    </Button>
                  </CardFooter>
                </Card>
                <Card>
                  <CardHeader>
                    <CardTitle>积分如何计算</CardTitle>
                  </CardHeader>
                  <CardContent className="flex flex-col gap-3">
                    <p>人民币报价 × 10，向上取整为积分。</p>
                    <p className="text-sm text-muted-foreground">
                      例如：¥0.26 → 3
                      积分。每次按整笔任务报价取整，提交前确认。积分计费开放后按此规则展示。
                    </p>
                  </CardContent>
                </Card>
              </>
            ) : section === "transactions" ? (
              <Empty className="min-h-80">
                <EmptyHeader>
                  <EmptyTitle>积分明细暂未开放</EmptyTitle>
                  <EmptyDescription>
                    开放后可在这里查询积分预占、结算与返还记录。
                  </EmptyDescription>
                </EmptyHeader>
              </Empty>
            ) : (
              <BillingAvailability section={section} />
            )}
            {section === "membership" || section === "recharge" ? (
              <ProductFooter />
            ) : null}
          </TabsContent>
        </Tabs>
      </section>
    </PageLayout>
  );
}
