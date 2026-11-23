import { describe, expect, it } from "vitest";
import {
  shotReference,
  storyboardMove,
  storyboardEdit,
} from "@/components/canvas/storyboard/canvas-storyboard-state";

const first = {
  id: "first",
  title: "渡轮",
  action: "渡轮驶过水面",
  duration_ms: 7000,
  revision: 1,
};
const second = {
  id: "second",
  title: "船舷",
  action: "乘客的脚步",
  duration_ms: 5000,
  revision: 3,
};
const third = { ...first, id: "third", title: "靠岸" };

describe("普通故事板", () => {
  it("按稳定镜头 ID 和锚点调整顺序", () => {
    expect(storyboardMove([first, second, third], second.id, -1)).toEqual({
      shot_id: second.id,
      before_id: first.id,
    });
    expect(storyboardMove([first, second, third], second.id, 1)).toEqual({
      shot_id: second.id,
    });
    expect(storyboardMove([first, second, third], first.id, -1)).toBeNull();
  });

  it("可在其他字段的改动后按原 ID 和当前修订保存，目标字段冲突时保留草稿", () => {
    const draft = { base: second.action, value: "手握住栏杆" };
    expect(
      storyboardEdit(
        { ...second, title: "扶栏", revision: 4 },
        "action",
        draft,
      ),
    ).toEqual({
      shot_id: second.id,
      expected_revision: 4,
      field: "action",
      value: "手握住栏杆",
    });
    expect(
      storyboardEdit(
        { ...second, action: "松开栏杆", revision: 5 },
        "action",
        draft,
      ),
    ).toBeNull();
  });

  it("加入对话时保留当时的镜头身份、顺序和修订", () => {
    expect(shotReference("board", second, 2)).toContain(
      "board_id=board, shot_id=second, shot_revision=3",
    );
    expect(shotReference("board", second, 2)).toContain("第 2 镜");
  });
});
