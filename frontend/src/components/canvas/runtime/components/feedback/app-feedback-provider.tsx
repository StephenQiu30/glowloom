"use client";

import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { toast } from "sonner";
import { Toaster } from "@/components/ui/sonner";
import { notifyError } from "./error-notification";

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { useThemeStore } from "@/components/canvas/runtime/store/use-theme-store";

export type ConfirmOptions = {
  title: ReactNode;
  content?: ReactNode;
  okText?: string;
  cancelText?: string;
  okType?: string;
  okButtonProps?: { danger?: boolean };
  onOk?: () => unknown;
  onCancel?: () => void;
  afterClose?: () => void;
};

type PendingConfirm = { id: number; options: ConfirmOptions };
type FeedbackContextValue = {
  confirm: (options: ConfirmOptions) => { destroy: () => void };
};

const FeedbackContext = createContext<FeedbackContextValue | null>(null);
let nextConfirmId = 0;

export function AppFeedbackProvider({ children }: { children: ReactNode }) {
  const theme = useThemeStore((state) => state.theme);
  const [pending, setPending] = useState<PendingConfirm | null>(null);
  const [accepting, setAccepting] = useState(false);
  const pendingRef = useRef<PendingConfirm | null>(null);

  const closeConfirm = useCallback((id: number) => {
    if (pendingRef.current?.id !== id) return;
    const current = pendingRef.current;
    pendingRef.current = null;
    setPending(null);
    current.options.afterClose?.();
  }, []);

  const confirm = useCallback(
    (options: ConfirmOptions) => {
      const next = { id: ++nextConfirmId, options };
      pendingRef.current = next;
      setPending(next);
      return { destroy: () => closeConfirm(next.id) };
    },
    [closeConfirm],
  );

  const contextValue = useMemo(() => ({ confirm }), [confirm]);
  const options = pending?.options;
  const danger = options?.okButtonProps?.danger || options?.okType === "danger";

  const accept = async () => {
    if (!pending || accepting) return;
    setAccepting(true);
    try {
      await options?.onOk?.();
      closeConfirm(pending.id);
    } catch {
      // Keep the confirmation open when the requested action fails.
    } finally {
      setAccepting(false);
    }
  };

  return (
    <FeedbackContext.Provider value={contextValue}>
      {children}
      <Toaster
        position="top-right"
        offset={{ top: "var(--spacing-toast-top)" }}
        mobileOffset={{ top: "var(--spacing-toast-mobile-top)" }}
        theme={theme}
      />
      <AlertDialog
        open={Boolean(pending)}
        onOpenChange={(open) =>
          !open && !accepting && pending && closeConfirm(pending.id)
        }
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{options?.title}</AlertDialogTitle>
            {options?.content ? (
              <AlertDialogDescription>{options.content}</AlertDialogDescription>
            ) : null}
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel asChild>
              <Button
                variant="outline"
                disabled={accepting}
                onClick={() => options?.onCancel?.()}
              >
                {options?.cancelText || "取消"}
              </Button>
            </AlertDialogCancel>
            <AlertDialogAction asChild>
              <Button
                variant={danger ? "destructive" : "default"}
                disabled={accepting}
                onClick={(event) => {
                  event.preventDefault();
                  void accept();
                }}
              >
                {accepting && <Spinner data-icon="inline-start" />}
                {options?.okText || "确定"}
              </Button>
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </FeedbackContext.Provider>
  );
}

export function useAppFeedback() {
  const context = useContext(FeedbackContext);
  if (!context)
    throw new Error("useAppFeedback must be used inside AppFeedbackProvider");

  return {
    message: {
      success: toast.success,
      error: notifyError,
      warning: toast.warning,
      info: toast.info,
      loading: (message: string) => {
        const id = toast.loading(message);
        return () => toast.dismiss(id);
      },
    },
    modal: { confirm: context.confirm },
  };
}
