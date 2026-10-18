"use client";
import { useState } from "react";
import { useOptionalSession } from "@/components/auth/session";
import { BasicLayout } from "@/components/layout/basic-layout";
import { ProvidersPage } from "./providers-page";
export function ProvidersWorkspace({
  initialConnection,
}: {
  initialConnection?: string;
}) {
  const [search, setSearch] = useState("");
  const [query, setQuery] = useState("");
  const session = useOptionalSession();
  return (
    <BasicLayout
      screen="providers"
      fullBleed
      search={{
        value: search,
        onValueChange: setSearch,
        onSearch: setQuery,
        label: "搜索 AI 服务",
        placeholder: "搜索连接或预设",
        disabled: session?.actor.platform_role !== "platform_admin",
      }}
    >
      <ProvidersPage initialConnection={initialConnection} search={query} />
    </BasicLayout>
  );
}
