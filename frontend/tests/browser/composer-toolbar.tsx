/** Native production components, with explicit in-memory model fixtures and no API. */
import { useState } from "react";
import { createRoot } from "react-dom/client";
import { ArrowUp } from "lucide-react";
import { CreationEntryPrompt } from "@/components/generation/creation-entry-prompt";
import { ReferenceStack } from "@/components/generation/reference-stack";
import { ModelPicker } from "@/components/generation/model-picker";
import { GenerationParameters } from "@/components/generation/generation-parameters";
import { GenerationToolbarParameters } from "@/components/generation/generation-toolbar-parameters";
import {
  ComposerToolbar,
  ComposerToolbarActions,
  ComposerToolbarButton,
} from "@/components/generation/composer-toolbar";
import {
  emptyPromptDraft,
  type PromptDraft,
} from "@/components/generation/prompt-document";
import { CreationTypeSelect } from "@/components/generation/creation-type-select";
import type { ParamField } from "@/components/generation/contracts";
import "../../src/app/globals.css";

document.documentElement.classList.add("dark");
const fields: ParamField[] = [
  {
    field: "aspect_ratio",
    label: "比例",
    type: "string",
    component: "select",
    enum: ["21:9", "16:9", "4:3", "1:1", "3:4", "9:16"],
  },
  {
    field: "resolution",
    label: "清晰度",
    type: "string",
    component: "select",
    enum: ["720P"],
  },
];
const duration: ParamField = {
  field: "duration_ms",
  label: "时长",
  type: "integer",
  component: "input",
  min: 4000,
  max: 15000,
  step: 1000,
};
const models: API.ModelResponse[] = [
  "样本视频模型",
  "用于验证截断和手机宽度的很长很长的视频模型名称",
].map((name, i) => ({
  id: `model-${i}`,
  key: `model-${i}`,
  display_name: name,
  provider_name: "隔离界面测试",
  status: "active",
  provider_status: "active",
  credential_present: true,
  available: true,
  mode_availability: [{ mode: "text2video", available: true }],
  current_version: { id: `version-${i}` },
}));
function Fixture() {
  const [draft, setDraft] = useState<PromptDraft>(() => ({
    ...emptyPromptDraft(),
    params: {
      aspect_ratio: "16:9",
      resolution: "720P",
      output_count: 1,
      duration_ms: 5000,
    },
  }));
  const [model, setModel] = useState("version-0");
  return (
    <main className="mx-auto flex max-w-5xl flex-col gap-6 p-4">
      <p>隔离组件验证：模型与参数为内存夹具，没有上传、保存或生成请求。</p>
      <form
        className="creation-input-surface flex min-w-0 flex-col gap-3 p-4"
        onSubmit={(event) => event.preventDefault()}
      >
        <div className="flex min-w-0 items-start gap-3">
          <ReferenceStack
            entries={[]}
            onAdd={() => {}}
            onSelect={() => {}}
            onRemove={() => {}}
          />
          <CreationEntryPrompt
            contextKey="toolbar-fixture"
            draft={draft}
            onChange={setDraft}
            onSubmit={() => {}}
            placeholder="使用 @ 快速调用参考内容，输入中文与长文本验证共享正文。"
          />
        </div>
        <ComposerToolbar aria-label="生成工具">
          <CreationTypeSelect value="video" onChange={() => {}} />
          <ModelPicker
            models={models}
            mode="text2video"
            value={model}
            disabled={false}
            onSelect={(m) => setModel(m.current_version!.id!)}
          />
          <GenerationToolbarParameters
            enabled
            disabled={false}
            params={draft.params}
            maxOutputs={2}
            duration={duration}
            onChange={(params) => setDraft({ ...draft, params })}
          >
            <GenerationParameters
              fields={fields}
              params={draft.params}
              maxOutputs={2}
              disabled={false}
              compact
              includeOutputCount={false}
              onChange={(params) => setDraft({ ...draft, params })}
            />
          </GenerationToolbarParameters>
          <ComposerToolbarActions>
            <span>费用待确认</span>
            <ComposerToolbarButton
              size="toolbar-icon"
              disabled
              variant="default"
              aria-label="生成"
            >
              <ArrowUp />
            </ComposerToolbarButton>
          </ComposerToolbarActions>
        </ComposerToolbar>
      </form>
      <output aria-label="参数快照" className="text-xs break-all">
        {JSON.stringify({ model, ...draft.params })}
      </output>
    </main>
  );
}
createRoot(document.getElementById("root")!).render(<Fixture />);
