import { describe, expect, it } from "vitest";
import { emptyPromptDraft } from "@/components/generation/prompt-document";
import {
  assertCanvasAudioConnectionCapability,
  canvasAudioCatalogAcceptsSubjects,
  createCanvasAudioConnectionDraft,
  type CanvasAudioModelCatalog,
} from "@/components/canvas/lib/canvas-audio-connection-capability";
const identity = { projectID: "project", scopeID: "scope" };
function model(types = ["image", "audio"], id = "one"): API.ModelResponse {
  return {
    id,
    capability: "audio.generate",
    status: "active",
    provider_status: "active",
    credential_present: true,
    input_roles: types.map((kind) => `reference_${kind}`),
    current_version: {
      id: `${id}-v1`,
      modes: ["omni_audio"],
      param_schema: [
        {
          field: "speed",
          label: "语速",
          type: "number",
          component: "slider",
          default: 1,
          min: 0.5,
          max: 2,
        },
      ],
      limits: {
        roles: Object.fromEntries(
          types.map((kind) => [
            `reference_${kind}`,
            { types: [kind], for_modes: ["omni_audio"] },
          ]),
        ),
      },
    },
  };
}
function catalog(models = [model()]): CanvasAudioModelCatalog {
  return { ...identity, status: "ready", models };
}
const audio = { role: "reference_audio", mediaKinds: ["audio"] as const };
const video = { role: "reference_video", mediaKinds: ["video"] as const };
describe("formal audio connected creation capabilities", () => {
  it("pins the default model version, mode and declared parameter defaults together", () => {
    const selected = createCanvasAudioConnectionDraft(catalog(), identity);
    expect(selected).toMatchObject({
      modelID: "one",
      draft: {
        modelVersion: "one-v1",
        mode: "omni_audio",
        params: { speed: 1, output_count: 1 },
      },
    });
    expect(
      assertCanvasAudioConnectionCapability(
        catalog(),
        identity,
        selected.draft,
        [audio],
      ).id,
    ).toBe("one");
  });
  it.each(["loading", "error"] as const)(
    "fails closed for %s catalog",
    (status) => {
      expect(() =>
        createCanvasAudioConnectionDraft({ ...identity, status }, identity),
      ).toThrow();
    },
  );
  it("rejects a stale project or scope, absent draft and absent registry", () => {
    for (const other of [
      { ...identity, projectID: "other" },
      { ...identity, scopeID: "other" },
    ])
      expect(() =>
        createCanvasAudioConnectionDraft(catalog(), other),
      ).toThrow();
    expect(() =>
      createCanvasAudioConnectionDraft(catalog([]), identity),
    ).toThrow();
    expect(() =>
      assertCanvasAudioConnectionCapability(catalog(), identity, undefined, [
        audio,
      ]),
    ).toThrow();
  });
  it("selects the first eligible omni model without choosing another to accommodate sources", () => {
    const speech = model();
    speech.current_version!.modes = ["text2audio"];
    const choices = catalog([
      { ...model(), status: "disabled" },
      speech,
      model(["audio"], "default"),
      model(["audio", "video"], "later"),
    ]);
    const selected = createCanvasAudioConnectionDraft(choices, identity);
    expect(selected.modelID).toBe("default");
    expect(() =>
      assertCanvasAudioConnectionCapability(choices, identity, selected.draft, [
        video,
      ]),
    ).toThrow();
  });
  it("rejects video only when the formal contract rejects it; future video-capable omni model accepts V+A", () => {
    const current = catalog();
    const draft = createCanvasAudioConnectionDraft(current, identity).draft;
    expect(() =>
      assertCanvasAudioConnectionCapability(current, identity, draft, [video]),
    ).toThrow();
    expect(() =>
      assertCanvasAudioConnectionCapability(current, identity, draft, [
        video,
        audio,
      ]),
    ).toThrow();
    const future = catalog([model(["audio", "video"])]);
    expect(() =>
      assertCanvasAudioConnectionCapability(
        future,
        identity,
        createCanvasAudioConnectionDraft(future, identity).draft,
        [video, audio],
      ),
    ).not.toThrow();
  });
  it.each([
    { status: "disabled" },
    { provider_status: "disabled" },
    { credential_present: false },
    { current_version: undefined },
    { id: undefined },
  ])("fails closed for unavailable model %j", (patch) => {
    expect(() =>
      createCanvasAudioConnectionDraft(
        catalog([{ ...model(), ...patch }]),
        identity,
      ),
    ).toThrow();
  });
  it("rejects absent role contracts and wrong mode restrictions", () => {
    for (const mutate of [
      (m: API.ModelResponse) => {
        m.current_version!.limits = {};
      },
      (m: API.ModelResponse) => {
        m.input_roles = [];
      },
      (m: API.ModelResponse) => {
        m.current_version!.limits!.roles.reference_audio.for_modes = [
          "text2audio",
        ];
      },
    ]) {
      const m = model();
      const draft = createCanvasAudioConnectionDraft(
        catalog([m]),
        identity,
      ).draft;
      mutate(m);
      expect(() =>
        assertCanvasAudioConnectionCapability(catalog([m]), identity, draft, [
          audio,
        ]),
      ).toThrow();
    }
  });
  it("rejects invalid schema and never fabricates a required parameter missing its default", () => {
    const m = model();
    m.current_version!.param_schema = undefined;
    expect(() =>
      createCanvasAudioConnectionDraft(catalog([m]), identity),
    ).toThrow();
    m.current_version!.param_schema = [
      {
        field: "speaker",
        label: "音色",
        type: "string",
        component: "voice",
        required: true,
      },
    ];
    expect(() =>
      createCanvasAudioConnectionDraft(catalog([m]), identity),
    ).toThrow();
  });
  it("revalidates existing exact version/mode, never falling back to the default", () => {
    const c = catalog();
    const draft = createCanvasAudioConnectionDraft(c, identity).draft;
    for (const invalid of [
      { ...draft, modelVersion: "retired" },
      { ...draft, mode: "text2audio" },
      emptyPromptDraft(),
    ])
      expect(() =>
        assertCanvasAudioConnectionCapability(c, identity, invalid, [audio]),
      ).toThrow();
  });
  it("checks existing references and every subject physical type", () => {
    const c = catalog();
    const draft = createCanvasAudioConnectionDraft(c, identity).draft;
    draft.references = [
      {
        id: crypto.randomUUID(),
        role: "reference_video",
        mediaKind: "video",
        order: 0,
        bindingGeneration: 1,
        source: {
          kind: "media",
          sourceId: crypto.randomUUID(),
          sourceVersion: "1",
        },
      },
    ];
    expect(() =>
      assertCanvasAudioConnectionCapability(c, identity, draft, [audio]),
    ).toThrow();
    const m = model();
    m.input_roles!.push("subject");
    m.current_version!.limits!.roles.subject = { types: ["image"] };
    const subjects = catalog([m]);
    const d = createCanvasAudioConnectionDraft(subjects, identity).draft;
    expect(() =>
      assertCanvasAudioConnectionCapability(subjects, identity, d, [
        { role: "subject", mediaKinds: ["image", "video"] },
      ]),
    ).toThrow();
    expect(() =>
      assertCanvasAudioConnectionCapability(subjects, identity, d, [
        { role: "subject", mediaKinds: [] },
      ]),
    ).toThrow();
  });
  it("type preview does not invent duration/count/complete-generation restrictions", () => {
    const c = catalog();
    const draft = createCanvasAudioConnectionDraft(c, identity).draft;
    expect(() =>
      assertCanvasAudioConnectionCapability(c, identity, draft, [
        audio,
        { role: "reference_image", mediaKinds: ["image"] },
      ]),
    ).not.toThrow();
  });
});

it("resolves pinned subject media only when a formal available omni role can accept them", () => {
  const m = model();
  expect(canvasAudioCatalogAcceptsSubjects(catalog([m]), identity)).toBe(false);
  m.input_roles!.push("subject");
  m.current_version!.limits!.roles.subject = { types: ["image", "audio"] };
  expect(canvasAudioCatalogAcceptsSubjects(catalog([m]), identity)).toBe(true);
  expect(
    canvasAudioCatalogAcceptsSubjects(
      { ...identity, status: "loading" },
      identity,
    ),
  ).toBe(false);
  expect(
    canvasAudioCatalogAcceptsSubjects(catalog([m]), {
      ...identity,
      scopeID: "other",
    }),
  ).toBe(false);
});
