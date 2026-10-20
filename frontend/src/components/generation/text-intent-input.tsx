"use client";

import { useEffect, useRef, useState, type ComponentProps } from "react";
import { Input } from "@/components/ui/input";
import {
  nativeTextEdit,
  type BeforeTextInput,
  type TextInputEdit,
} from "./text-input-intent";

type Props = Omit<
  ComponentProps<typeof Input>,
  "value" | "onChange" | "ref"
> & {
  value: string;
  onEdit: (value: string, intent: TextInputEdit) => void;
  onHistory?: (direction: "undo" | "redo") => void;
  onCompositionState?: (active: boolean) => void;
};

/** Plain parameter editing reports the browser's selection, including IME's
 * original replacement range. Intermediate composition remains local UI state. */
export function TextIntentInput({
  value,
  onEdit,
  onHistory,
  onCompositionState,
  ...props
}: Props) {
  const [display, setDisplay] = useState(value),
    [error, setError] = useState("");
  const input = useRef<HTMLInputElement>(null),
    lastApplied = useRef(value);
  const before = useRef<BeforeTextInput | undefined>(undefined),
    composition = useRef<BeforeTextInput | undefined>(undefined);
  const compositionOwner = useRef(onCompositionState);
  useEffect(() => {
    compositionOwner.current = onCompositionState;
  }, [onCompositionState]);
  useEffect(
    () => () => {
      if (composition.current) compositionOwner.current?.(false);
      composition.current = undefined;
    },
    [],
  );
  useEffect(() => {
    if (!composition.current) {
      setDisplay(value);
      lastApplied.current = value;
    }
  }, [value]);
  const apply = (next: string, premise: BeforeTextInput | undefined) => {
    if (next === lastApplied.current) {
      setDisplay(next);
      return;
    }
    try {
      if (!premise) throw new Error("missing_beforeinput");
      const edit = nativeTextEdit(premise, next);
      onEdit(next, edit);
      lastApplied.current = next;
      setDisplay(next);
      setError("");
    } catch {
      setDisplay(premise?.value ?? value);
      setError("无法定位本次输入，未应用修改，请重新输入。");
    }
  };
  useEffect(() => {
    const element = input.current;
    if (!element) return;
    const capture = (event: InputEvent) => {
      if (
        event.inputType === "historyUndo" ||
        event.inputType === "historyRedo"
      ) {
        event.preventDefault();
        onHistory?.(event.inputType === "historyUndo" ? "undo" : "redo");
        return;
      }
      before.current = {
        value: element.value,
        from: element.selectionStart ?? 0,
        to: element.selectionEnd ?? 0,
        inputType: event.inputType,
      };
    };
    element.addEventListener("beforeinput", capture);
    return () => element.removeEventListener("beforeinput", capture);
  }, [onHistory]);
  return (
    <>
      <Input
        {...props}
        ref={input}
        value={display}
        aria-invalid={error ? true : props["aria-invalid"]}
        onCompositionStart={(event) => {
          composition.current = {
            value: event.currentTarget.value,
            from: event.currentTarget.selectionStart ?? 0,
            to: event.currentTarget.selectionEnd ?? 0,
            inputType: "insertCompositionText",
          };
          onCompositionState?.(true);
        }}
        onCompositionEnd={(event) => {
          const premise = composition.current;
          composition.current = undefined;
          apply(event.currentTarget.value, premise);
          before.current = undefined;
          onCompositionState?.(false);
        }}
        onChange={(event) => {
          const next = event.currentTarget.value;
          if (composition.current) {
            setDisplay(next);
            return;
          }
          apply(next, before.current);
          before.current = undefined;
        }}
        onKeyDown={(event) => {
          props.onKeyDown?.(event);
          if (event.defaultPrevented || event.nativeEvent.isComposing) return;
          const key = event.key.toLowerCase();
          if (
            (event.metaKey || event.ctrlKey) &&
            ["z", "y"].includes(key) &&
            onHistory
          ) {
            event.preventDefault();
            onHistory(key === "y" || event.shiftKey ? "redo" : "undo");
          }
        }}
      />
      {error ? (
        <span role="alert" className="text-xs text-destructive">
          {error}
        </span>
      ) : null}
    </>
  );
}
