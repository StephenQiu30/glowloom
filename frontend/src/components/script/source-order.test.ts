import { expect, it } from "vitest";
import { moveSourcePosition, reviewSourceOrder } from "./source-order";
it("移动位置保留完整唯一ID集合、原顺序及其余位置，拒越界/重复集合", () => {
  const ids = [
    "11111111-1111-4111-8111-111111111111",
    "22222222-2222-4222-8222-222222222222",
    "33333333-3333-4333-8333-333333333333",
  ];
  expect(moveSourcePosition(ids, ids[0], 2)).toEqual([ids[1], ids[2], ids[0]]);
  expect(moveSourcePosition(ids, ids[2], 0)).toEqual([ids[2], ids[0], ids[1]]);
  expect(ids[0]).toBe("11111111-1111-4111-8111-111111111111");
  for (const position of [-1, 3, 0.5, Infinity])
    expect(() => moveSourcePosition(ids, ids[0], position)).toThrow();
  expect(() => moveSourcePosition([ids[0], ids[0]], ids[0], 0)).toThrow();
});
it("409审阅保留仍有效的草稿顺序并明确最新新增/移除集合", () => {
  const a = "11111111-1111-4111-8111-111111111111";
  const b = "22222222-2222-4222-8222-222222222222";
  const c = "33333333-3333-4333-8333-333333333333";
  const d = "44444444-4444-4444-8444-444444444444";
  expect(reviewSourceOrder([c, a, b], [a, c, d])).toEqual({
    order: [c, a, d],
    added: [d],
    removed: [b],
  });
  expect(() => reviewSourceOrder([a, a], [a])).toThrow();
  expect(() => reviewSourceOrder([a], [a, a])).toThrow();
});
