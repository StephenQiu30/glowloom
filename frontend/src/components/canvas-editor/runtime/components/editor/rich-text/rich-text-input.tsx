"use client";

import dynamic from "next/dynamic";
import { Skeleton } from "@/components/ui/skeleton";
import type { RichTextInputProps } from "../types/rich-text.ts";

const Editor = dynamic(
  () =>
    import("./rich-text-editor.tsx").then((module) => module.RichTextEditor),
  { ssr: false, loading: () => <Skeleton className="rich-text-loading" /> },
);

export function RichTextInput(props: RichTextInputProps) {
  return <Editor {...props} />;
}
