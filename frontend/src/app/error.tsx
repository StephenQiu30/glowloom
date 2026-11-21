"use client";
import { PageFailure } from "@/components/layout/application-state";

export default function PageError({ retry }: { retry: () => void }) {
  return <PageFailure retry={retry} />;
}
