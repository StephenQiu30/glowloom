"use client";
import { useState } from "react";
import { useOptionalSession } from "@/components/auth/session";
import { PageLayout } from "@/components/layout/page-layout";
import { ProvidersPage } from "./providers-page";
export function ProvidersWorkspace({
  initialConnection,
  initialModel,
  initialRelease,
  initialMode,
}: {
  initialConnection?: string;
  initialModel?: string;
  initialRelease?: string;
  initialMode?: string;
}) {
  const [presetsOpen, setPresetsOpen] = useState(false);
  const [connections, setConnections] = useState({ draft: "", query: "" });
  const [presets, setPresets] = useState({ draft: "", query: "" });
  const search = presetsOpen ? presets : connections;
  const setSearch = presetsOpen ? setPresets : setConnections;
  const session = useOptionalSession();
  return (
    <PageLayout
      screen="providers"
      search={{
        value: search.draft,
        onValueChange: (draft) => setSearch((state) => ({ ...state, draft })),
        onSearch: (query) => setSearch((state) => ({ ...state, query })),
        label: presetsOpen ? "搜索供应商预设" : "搜索 AI 服务",
        placeholder: presetsOpen
          ? "搜索预设名称或供应商"
          : "搜索连接名称或供应商",
        disabled: session?.actor.platform_role !== "platform_admin",
      }}
    >
      <ProvidersPage
        initialConnection={initialConnection}
        initialModel={initialModel}
        initialRelease={initialRelease}
        initialMode={initialMode}
        search={search.query}
        onPresetViewChange={setPresetsOpen}
      />
    </PageLayout>
  );
}
