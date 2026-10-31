import { Editor } from "@tiptap/core";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import {
  ComposerSession,
  composerEditorOptions,
} from "@/components/generation/composer-session";
import { emptyPromptDraft } from "@/components/generation/prompt-document";
import { ReferenceControls } from "@/components/generation/reference-controls";
import type { UnifiedSubject } from "@/components/subjects/unified-contracts";
const api = vi.hoisted(() => ({ read: vi.fn() }));
vi.mock("@/api/subjects", () => ({
  getSubjectVersion: api.read,
  listSubjects: vi.fn(),
}));
vi.mock("@/components/auth/session", () => ({
  useOptionalSession: () => ({ actor: { id: "actor" } }),
}));
vi.mock("@/components/spaces/space-context", () => ({
  useSpace: () => ({
    selected: { id: "33333333-3333-4333-8333-333333333333" },
  }),
}));
const subject: UnifiedSubject = {
  id: "11111111-1111-4111-8111-111111111111",
  version_id: "22222222-2222-4222-8222-222222222222",
  owner_scope_id: "33333333-3333-4333-8333-333333333333",
  revision: 9,
  name: "固定主体",
  description: "第一版内容",
  primary: {
    binding_id: "44444444-4444-4444-8444-444444444444",
    asset_id: "55555555-5555-4555-8555-555555555555",
    asset_revision: 2,
    kind: "image",
    mime_type: "image/png",
    file_name: "main.png",
  },
  references: [
    {
      binding_id: "66666666-6666-4666-8666-666666666666",
      asset_id: "55555555-5555-4555-8555-555555555555",
      asset_revision: 2,
      kind: "image",
      mime_type: "image/png",
      file_name: "reference.png",
    },
  ],
  voice: {
    name: "声音",
    media: {
      binding_id: "77777777-7777-4777-8777-777777777777",
      asset_id: "88888888-8888-4888-8888-888888888888",
      asset_revision: 3,
      kind: "audio",
      mime_type: "audio/mp4",
      file_name: "voice.m4a",
    },
  },
  favorite: false,
  created_at: "2026-10-28T00:00:00Z",
  updated_at: "2026-10-29T00:00:00Z",
};

const editors: Editor[] = [];
afterEach(() => {
  cleanup();
  editors.splice(0).forEach((e) => e.destroy());
  vi.clearAllMocks();
});
function setup() {
  const editor = new Editor(
    composerEditorOptions({
      contextKey: "canvas/subject",
      replicaId: "test",
      draft: emptyPromptDraft(),
    }),
  );
  editors.push(editor);
  const session = new ComposerSession(editor);
  const source = {
    source: "subject" as const,
    nodeID: "subject-node",
    title: "画布固定主体",
    subjectID: subject.id,
    versionID: subject.version_id,
  };
  const view = (sources = [source]) => (
    <ReferenceControls
      session={session}
      draft={session.snapshot().draft}
      projectID="project"
      compact
      canvasSources={[]}
      canvasSubjectSources={sources}
    />
  );
  const rendered = render(view());
  return { session, rendered, view, source };
}
async function choose() {
  fireEvent.click(screen.getByRole("button", { name: "添加参考" }));
  fireEvent.click(await screen.findByRole("option", { name: "从画布选择" }));
  fireEvent.click(await screen.findByRole("option", { name: "画布固定主体" }));
}
it("imports the pinned subject with primary, auxiliaries and voice instead of using its current head", async () => {
  api.read.mockResolvedValue(subject);
  const { session } = setup();
  await choose();
  await waitFor(() =>
    expect(session.snapshot().draft.references).toHaveLength(1),
  );
  expect(api.read).toHaveBeenCalledWith(
    { id: subject.id, version: subject.version_id },
    expect.objectContaining({ signal: expect.any(AbortSignal) }),
  );
  expect(session.snapshot().draft.references[0].source).toMatchObject({
    kind: "subject",
    sourceId: subject.id,
    sourceVersion: subject.version_id,
  });
  const source = session.snapshot().draft.references[0].source;
  expect(source.kind).toBe("subject");
  if (source.kind === "subject") expect(source.media).toHaveLength(3);
});
it("does not insert a late response after the canvas subject pin has changed", async () => {
  let resolve!: (value: unknown) => void;
  api.read.mockReturnValue(
    new Promise((done) => {
      resolve = done;
    }),
  );
  const { session, rendered, view, source } = setup();
  await choose();
  rendered.rerender(view([{ ...source, versionID: subject.id }]));
  await act(async () => {
    resolve(subject);
  });
  expect(session.snapshot().draft.references).toHaveLength(0);
});
it("rejects an unauthorized scope or substituted fixed version", async () => {
  api.read.mockResolvedValue({ ...subject, owner_scope_id: subject.id });
  const { session } = setup();
  await choose();
  await waitFor(() => expect(api.read).toHaveBeenCalledOnce());
  await act(async () => {});
  expect(session.snapshot().draft.references).toHaveLength(0);
});
