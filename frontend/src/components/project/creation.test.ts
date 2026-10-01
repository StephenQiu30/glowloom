import { describe, expect, it } from "vitest";
import {
  creationBody,
  initialProjectDraft,
  updateProjectDraft,
} from "./creation";

describe("项目创建规格", () => {
  it("按 Unicode 字符校验名称，保留描述且不发送固定默认策略", () => {
    const draft = {
      ...initialProjectDraft,
      name: "  🎬".repeat(1) + "🎬".repeat(49) + "  ",
      description: "故事\n设定",
    };
    const result = creationBody(draft, []);
    expect(result.errors).toEqual({});
    expect(result.body).toEqual({
      name: "🎬".repeat(50),
      description: "故事\n设定",
      aspect_ratio: "16:9",
      style_type: "realistic",
    });
    expect(
      creationBody({ ...draft, name: "🎬".repeat(51) }, []).errors.name,
    ).toBeTruthy();
    expect(
      creationBody({ ...draft, name: " \n\t" }, []).errors.name,
    ).toBeTruthy();
  });

  it("风格化必须选正式子风格，写实不能夹带旧子风格", () => {
    const draft = {
      ...initialProjectDraft,
      name: "逆光",
      style_type: "stylized" as const,
    };
    expect(creationBody(draft, []).errors.style_subtype).toBeTruthy();
    for (const style_subtype of [
      "anime_jp",
      "guofeng_xianxia",
      "cartoon_3d",
      "manhwa",
    ] as const) {
      expect(
        creationBody({ ...draft, style_subtype }, []).body?.style_subtype,
      ).toBe(style_subtype);
    }
    const realistic = updateProjectDraft(
      { ...draft, style_subtype: "anime_jp", style_preset_id: "a" },
      { style_type: "realistic" },
    );
    expect(realistic.style_subtype).toBe("");
    expect(realistic.style_preset_id).toBe("");
  });

  it("只提交授权列表中与当前类型和子风格匹配的预设，切换子风格清预设", () => {
    const presets = [
      {
        id: "4c7fb626-1e42-4f02-a21f-36e8c60a0567",
        name: "国风",
        style_type: "stylized" as const,
        style_subtype: "guofeng_xianxia" as const,
      },
    ];
    const draft = {
      ...initialProjectDraft,
      name: "逆光",
      style_type: "stylized" as const,
      style_subtype: "guofeng_xianxia" as const,
      style_preset_id: presets[0].id,
    };
    expect(creationBody(draft, presets).body?.style_preset_id).toBe(
      presets[0].id,
    );
    expect(creationBody(draft, []).errors.style_preset_id).toBeTruthy();
    expect(
      updateProjectDraft(draft, { style_subtype: "manhwa" }).style_preset_id,
    ).toBe("");
    expect(
      creationBody({ ...draft, style_subtype: "anime_jp" }, presets).errors
        .style_preset_id,
    ).toBeTruthy();
  });
});
