"use client";

import { useState, type ReactNode } from "react";
import { ClipboardCheck } from "lucide-react";
import { reviewAiCreditGrant } from "@/api/aiGateway";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { GatewayAction } from "./gateway-action";
import type { RunMutation } from "./gateway-fields";
import { GatewayTable } from "./gateway-table";

const orderStates = {
  pending: "待复核",
  issued: "已入账",
  rejected: "已拒绝",
} satisfies Record<API.CreditGrantOrder["state"], string>;

export function GatewayCreditGrants({
  actorID,
  orders,
  busy,
  run,
  toolbarActions,
}: {
  actorID: string;
  orders: API.CreditGrantOrder[];
  busy: boolean;
  run: RunMutation;
  toolbarActions?: ReactNode;
}) {
  const [selectedID, setSelectedID] = useState<string>();
  const [reviewFailed, setReviewFailed] = useState(false);
  const order = orders.find(({ id }) => id === selectedID);
  const pending = order?.state === "pending" && order.proposer_id !== actorID;

  async function review(approve: boolean) {
    if (!order || !pending) return;
    setReviewFailed(false);
    const ok = await run(
      () => reviewAiCreditGrant({ id: order.id }, { approve }),
      approve ? "已复核入账。" : "已拒绝发放。",
    );
    if (ok) setSelectedID(undefined);
    else setReviewFailed(true);
  }

  return (
    <>
      <GatewayTable
        toolbarActions={toolbarActions}
        label="积分赠送单"
        columns={["受款空间", "积分", "业务凭据", "状态", "操作"]}
        empty="暂无积分赠送单"
        rows={orders.map((row) => ({
          id: row.id,
          label: row.recipient_label,
          terms: [row.scope_id, row.key],
          status: orderStates[row.state],
          cells: [
            row.recipient_label,
            row.credits,
            row.key,
            orderStates[row.state],
            <GatewayAction
              key="inspect"
              icon={ClipboardCheck}
              label={
                row.state === "pending" && row.proposer_id !== actorID
                  ? "复核赠送单"
                  : "查看赠送单"
              }
              disabled={busy}
              onClick={() => {
                setReviewFailed(false);
                setSelectedID(row.id);
              }}
            />,
          ],
        }))}
      />
      <Dialog
        open={Boolean(order)}
        onOpenChange={(open) => {
          if (!open && !busy) setSelectedID(undefined);
        }}
      >
        {order && (
          <DialogContent className="max-h-[90dvh] overflow-y-auto">
            <DialogHeader>
              <DialogTitle>
                {pending ? "复核积分赠送单" : "积分赠送单"}
              </DialogTitle>
              <DialogDescription>
                {orderStates[order.state]} · {order.credits} 通用积分
              </DialogDescription>
            </DialogHeader>
            <dl className="grid gap-3 text-sm [&_dd]:break-all [&_dt]:text-muted-foreground">
              <div>
                <dt>受款空间</dt>
                <dd>
                  {order.recipient_label} ·{" "}
                  {order.recipient_kind === "team" ? "团队" : "个人"}
                </dd>
                <dd>{order.scope_id}</dd>
              </div>
              <div>
                <dt>业务凭据</dt>
                <dd>{order.key}</dd>
              </div>
              <div>
                <dt>赠送原因</dt>
                <dd>{order.reason}</dd>
              </div>
              <div>
                <dt>发放单号</dt>
                <dd>{order.id}</dd>
              </div>
              <div>
                <dt>提交人 ID</dt>
                <dd>{order.proposer_id}</dd>
              </div>
              {order.reviewer_id && (
                <div>
                  <dt>复核人 ID</dt>
                  <dd>{order.reviewer_id}</dd>
                </div>
              )}
              {order.event_id && (
                <div>
                  <dt>钱包流水号</dt>
                  <dd>{order.event_id}</dd>
                </div>
              )}
            </dl>
            {reviewFailed && (
              <Alert variant="destructive" role="alert">
                <AlertDescription>
                  复核未完成。请刷新列表核对原单，再重试。
                </AlertDescription>
              </Alert>
            )}
            {pending ? (
              <DialogFooter>
                <Button
                  variant="outline"
                  disabled={busy}
                  onClick={() => void review(false)}
                >
                  拒绝发放
                </Button>
                <Button disabled={busy} onClick={() => void review(true)}>
                  批准并入账
                </Button>
              </DialogFooter>
            ) : (
              order.state === "pending" && (
                <p className="text-sm text-muted-foreground">
                  此单由你提交，需由另一位管理员复核。
                </p>
              )
            )}
          </DialogContent>
        )}
      </Dialog>
    </>
  );
}
