"use client";

import {
  createContext,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { useAccount } from "@/components/canvas/runtime/components/account/account-provider";
import { accountScope } from "@/components/canvas/runtime/store/account-slice";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { notifyError } from "@/components/canvas/runtime/components/feedback/error-notification";
import { creditFormat, creditTime } from "./lib/credit-format";
import { validateModelQuote } from "./lib/model-quote";

type Approval = (quote: CanvasContract.CreditQuoteVO) => Promise<boolean>;
type Pending = {
  quote: CanvasContract.CreditQuoteVO;
  identity: string;
  focus: HTMLElement | null;
  resolve: (approved: boolean) => void;
};
const Context = createContext<Approval | null>(null);

/** A single cost confirmation is shared by standalone generation and canvas nodes. */
export function ModelBillingProvider({ children }: { children: ReactNode }) {
  const { account } = useAccount();
  const identity = accountScope(account);
  const pending = useRef<Pending | null>(null);
  const active = useRef({ identity, mounted: false });
  const restoreFocus = useRef<HTMLElement | null>(null);
  const [dialog, setDialog] = useState<Pending | null>(null);
  useEffect(() => {
    active.current = { identity, mounted: true };
    return () => {
      active.current.mounted = false;
      pending.current?.resolve(false);
      pending.current = null;
    };
  }, [identity]);
  const finish = (approve: boolean) => {
    const current = pending.current;
    if (!current) return;
    let accepted = approve && current.identity === identity;
    if (accepted) {
      try {
        validateModelQuote(current.quote);
      } catch (error) {
        accepted = false;
        notifyError(error, "报价已失效");
      }
    }
    pending.current = null;
    setDialog(null);
    current.resolve(accepted);
  };
  const confirm: Approval = async (value) => {
    if (!active.current.mounted || active.current.identity !== identity)
      return false;
    const quote = validateModelQuote(value);
    if (pending.current) throw new Error("请先完成当前费用确认");
    return new Promise<boolean>((resolve) => {
      const next = {
        quote,
        identity,
        resolve,
        focus:
          document.activeElement instanceof HTMLElement
            ? document.activeElement
            : null,
      };
      pending.current = next;
      restoreFocus.current = next.focus;
      setDialog(next);
    });
  };
  return (
    <Context.Provider value={confirm}>
      {children}
      <Dialog
        open={!!dialog && dialog.identity === identity}
        onOpenChange={(open) => {
          if (!open) finish(false);
        }}
      >
        <DialogContent
          className="sm:max-w-[480px]"
          onCloseAutoFocus={(event) => {
            event.preventDefault();
            if (restoreFocus.current?.isConnected) restoreFocus.current.focus();
          }}
        >
          <DialogHeader>
            <DialogTitle>确认生成费用</DialogTitle>
            <DialogDescription>
              按已发布价目和实际用量结算，多余占用退回可用余额。
            </DialogDescription>
          </DialogHeader>
          {dialog && (
            <div className="flex flex-col gap-4 text-sm">
              <p className="font-medium tabular-nums">
                预计 {creditFormat(dialog.quote.estimatedPoints)} 积分 ·
                最多占用 {creditFormat(dialog.quote.maximumPoints)} 积分
              </p>
              <p className="break-words whitespace-pre-wrap text-muted-foreground">
                {dialog.quote.disclosure}
              </p>
              <p className="text-muted-foreground">
                有效至 {creditTime(dialog.quote.expiresAt)}
                。结果未知时保留占用待核对。
              </p>
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => finish(false)}>
              取消
            </Button>
            <Button onClick={() => finish(true)}>确认并生成</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Context.Provider>
  );
}

export function useModelBillingApproval() {
  const context = useContext(Context);
  if (!context) throw new Error("生成费用确认不可用");
  return context;
}
