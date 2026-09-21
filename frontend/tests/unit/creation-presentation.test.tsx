import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { ProposalContent } from "@/components/creation/proposal-content";
import { ProposalNavigation } from "@/components/creation/proposal-navigation";

const evidence = { block: 0, quote: "阿宁披着红衣，手臂受伤。", occurrence: null };
const base = { run_id: "run", source_revision_id: "source", source_hash: "hash", revision: 1 };
function proposal(
  stage: API.CreationProposal["stage"],
  candidate: API.CreationProposal["candidate"],
): API.CreationProposal {
  return { ...base, id: stage, stage, candidate } as API.CreationProposal;
}
const map = proposal("map_manuscript", {
  mode: "preserve",
  episodes: [
    { key: "ep", number: 1, title: "雨夜", first_block: 0, last_block: 0, rationale: "保留原稿" },
  ],
  excluded: [],
  issues: [],
});
const analysis = proposal("analyze_episode", {
  episode_key: "ep",
  summary: "来信",
  conflict: "犹豫",
  turning_point: "敲门",
  ending_hook: "谁来了",
  excluded: [],
  issues: [],
  scenes: [
    {
      key: "door",
      title: "旧宅门前",
      first_block: 0,
      last_block: 0,
      summary: "阿宁受伤归来",
      time_label: "夜",
      time_branch: "main",
      presentation: "present",
      beats: [],
      dialogues: [],
      mentions: [
        {
          key: "person",
          name: "阿宁",
          kind: "cast",
          presence: "onscreen",
          evidence,
          visual_details: [evidence],
        },
      ],
      issues: [{ code: "time", scope: "door", severity: "warning", summary: "此场时间尚待核对" }],
    },
  ],
});
const world = proposal("build_world", {
  entities: [
    {
      key: "aning",
      label: "阿宁",
      kind: "cast",
      identity_basis: "explicit",
      uncertainty: null,
      evidence: [evidence],
      mentions: [{ episode_key: "ep", scene_key: "door", mention_key: "person" }],
    },
  ],
  unresolved_mentions: [],
  relations: [],
  asset_needs: [{ entity_key: "aning", description: "红衣受伤造型参考", evidence: [evidence] }],
  state_events: [
    {
      entity_key: "aning",
      episode_key: "ep",
      scene_key: "door",
      time_branch: "main",
      story_time: "归来时",
      property: "手臂",
      before: "完好",
      after: "受伤",
      knowledge: "known",
      basis: "narration",
      evidence: [evidence],
    },
  ],
  issues: [],
});

describe("创作内容完整展示", () => {
  it("用剧集和场景名称查找并切换内容，空搜索可恢复", async () => {
    const user = userEvent.setup();
    function Directory() {
      const [selected, select] = useState(map.id);
      return (
        <ProposalNavigation
          proposals={[map, analysis, world]}
          selectedId={selected}
          onSelect={select}
        />
      );
    }
    render(<Directory />);
    const search = screen.getByRole("textbox", { name: "查找剧集或场景" });
    await user.type(search, "雨夜");
    const episode = screen.getByRole("button", { name: /第 1 集 · 雨夜/ });
    await user.click(episode);
    expect(episode).toHaveAttribute("aria-current", "true");
    expect(screen.queryByRole("button", { name: /人物、场景与道具/ })).not.toBeInTheDocument();
    await user.clear(search);
    expect(screen.getByRole("button", { name: /人物、场景与道具/ })).toBeInTheDocument();
    await user.type(search, "不存在");
    expect(screen.getByRole("status")).toHaveTextContent("没有找到匹配的剧集或场景");
  });

  it("同一角色资料汇集出现位置、外观、状态与参考需求", () => {
    render(<ProposalContent proposal={world} proposals={[map, analysis, world]} />);
    const card = screen.getByRole("region", { name: "角色资料：阿宁" });
    expect(within(card).getAllByText(/第 1 集 · 雨夜.*旧宅门前/)).toHaveLength(2);
    expect(within(card).getByRole("heading", { name: "外观与形象依据" })).toBeInTheDocument();
    expect(within(card).getByText(/完好.*受伤/)).toBeInTheDocument();
    expect(within(card).getByText("红衣受伤造型参考")).toBeInTheDocument();
    expect(within(card).getByText(/尚未生成角色图或三视图/)).toBeInTheDocument();
  });

  it("剧集分析显示场景级待确认问题", () => {
    render(<ProposalContent proposal={analysis} proposals={[map, analysis]} />);
    expect(screen.getByText("此场时间尚待核对")).toBeInTheDocument();
  });

  it("同一人物不同状态仍只有一份身份资料，不合并为一个当前状态", () => {
    const candidate = world.candidate as API.CreationTextWorldBook;
    const later = {
      ...candidate.state_events[0],
      story_time: "次日",
      before: "受伤",
      after: "包扎",
    };
    const changed = {
      ...world,
      candidate: { ...candidate, state_events: [...candidate.state_events, later] },
    };
    render(<ProposalContent proposal={changed} proposals={[map, analysis, changed]} />);
    const cards = screen.getAllByRole("region", { name: "角色资料：阿宁" });
    expect(cards).toHaveLength(1);
    expect(within(cards[0]).getByText(/完好.*受伤/)).toBeInTheDocument();
    expect(within(cards[0]).getByText(/受伤.*包扎/)).toBeInTheDocument();
  });

  it("不使用其他运行的同名内部标识解析角色来源", () => {
    render(
      <ProposalContent
        proposal={world}
        proposals={[{ ...analysis, run_id: "another-run" }, world]}
      />,
    );
    expect(screen.queryByText(/旧宅门前/)).not.toBeInTheDocument();
    expect(screen.getByText(/来源内容尚未读取/)).toBeInTheDocument();
  });
});
