"use client";
import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { useOptionalSession } from "@/components/auth/session";
import { useSpace } from "@/components/spaces/space-context";
import type { Project } from "@/components/projects/project";
import {
  emptyPromptDraft,
  type PromptDraft,
} from "@/components/generation/prompt-document";
import { draftText } from "./conversation-state";
export interface ConversationSkillPin {
  skill_id: string;
  version: string;
  resource_hash: string;
}
export interface ConversationSettings {
  modelKey?: string;
  budget: string;
  skills: ConversationSkillPin[];
}
interface ConversationContextValue {
  ownerKey: string;
  project: Project | null;
  selectProject: (project: Project) => void;
  listRevision: number;
  changed: () => void;
  readDraft: (key: string) => PromptDraft | undefined;
  saveDraft: (key: string, draft: PromptDraft) => void;
  readSettings: (projectID: string) => ConversationSettings | undefined;
  saveSettings: (projectID: string, settings: ConversationSettings) => void;
}
const Context = createContext<ConversationContextValue | null>(null);
function guestText(draft: PromptDraft) {
  const result = emptyPromptDraft();
  result.document.content = draftText(draft)
    .split("\n")
    .map((text) => ({
      type: "paragraph",
      content: text ? [{ type: "text", text }] : [],
    }));
  return result;
}
/** Page-lifetime memory only. No browser database or private cross-owner cache. */
export function ConversationProvider({ children }: { children: ReactNode }) {
  const session = useOptionalSession(),
    space = useSpace();
  const ownerKey = `${session?.actor.id ?? "guest"}/${space.selected?.id ?? "none"}`;
  const [selected, setSelected] = useState<{
    owner: string;
    project: Project;
  } | null>(null);
  const [revision, setRevision] = useState(0);
  const memory = useRef({
    owner: ownerKey,
    drafts: new Map<string, PromptDraft>(),
    settings: new Map<string, ConversationSettings>(),
  });
  useEffect(() => {
    if (memory.current.owner === ownerKey) return;
    const drafts = memory.current.owner.startsWith("guest/")
      ? new Map(
          [...memory.current.drafts].map(([key, draft]) => [
            key,
            guestText(draft),
          ]),
        )
      : new Map<string, PromptDraft>();
    memory.current = { owner: ownerKey, drafts, settings: new Map() };
  }, [ownerKey]);
  const project = selected?.owner === ownerKey ? selected.project : null;
  const value = useMemo<ConversationContextValue>(
    () => ({
      ownerKey,
      project,
      listRevision: revision,
      selectProject: (project) => setSelected({ owner: ownerKey, project }),
      changed: () => setRevision((n) => n + 1),
      readDraft: (key) => {
        const draft = memory.current.drafts.get(key);
        if (!draft) return;
        if (memory.current.owner === ownerKey) return structuredClone(draft);
        if (memory.current.owner.startsWith("guest/")) return guestText(draft);
      },
      saveDraft: (key, draft) => {
        if (memory.current.owner === ownerKey)
          memory.current.drafts.set(key, structuredClone(draft));
      },
      readSettings: (key) =>
        memory.current.owner === ownerKey
          ? memory.current.settings.get(key)
          : undefined,
      saveSettings: (key, settings) => {
        if (memory.current.owner === ownerKey)
          memory.current.settings.set(key, structuredClone(settings));
      },
    }),
    [ownerKey, project, revision],
  );
  return <Context value={value}>{children}</Context>;
}
export function useConversationContext() {
  return useContext(Context);
}
