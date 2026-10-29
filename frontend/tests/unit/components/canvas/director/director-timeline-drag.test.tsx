import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { DirectorTimeline } from "@/components/canvas/director/director-timeline";
import { createDirectorActor } from "@/components/canvas/director/director-assets";
import {
  directorTransform,
  emptyDirectorConfig,
} from "@/components/canvas/director/director-model";

beforeEach(() => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
    width: 1000,
    height: 30,
    left: 0,
    top: 0,
    right: 1000,
    bottom: 30,
    x: 0,
    y: 0,
    toJSON() {},
  });
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
function setup() {
  const scene = emptyDirectorConfig();
  scene.mode = "dynamic";
  const actor = createDirectorActor("male_standard");
  actor.name = "角色";
  actor.keyframes = [
    {
      id: crypto.randomUUID(),
      time: 1,
      easing: "linear",
      transform: directorTransform(),
      rotation_keyed: false,
      scale_keyed: false,
    },
  ];
  actor.motion_clips = [
    {
      id: crypto.randomUUID(),
      name: "走路",
      source_animation: "walk",
      start: 1,
      duration: 2,
      playback_rate: 2,
      loop: false,
    },
  ];
  scene.objects = [actor];
  const onChange = vi.fn();
  const props = {
    scene,
    time: 0,
    duration: 10,
    fps: 10,
    disabled: false,
    onTime: vi.fn(),
    onChange,
    onSelect: vi.fn(),
  };
  const result = render(<DirectorTimeline {...props} />);
  return { scene, actor, props, onChange, ...result };
}
const keyButton = () =>
  screen.getByRole("button", { name: /角色 位置 1.00 秒关键帧/ });
function down(node: HTMLElement, x = 100) {
  fireEvent.pointerDown(node, { button: 0, clientX: x, pointerId: 1 });
}
function move(node: HTMLElement, x: number) {
  fireEvent.pointerMove(node, { clientX: x, pointerId: 1 });
}
function up(node: HTMLElement, x: number) {
  fireEvent.pointerUp(node, { clientX: x, pointerId: 1 });
}
it("previews snapped keys locally and commits exactly once on release", () => {
  const { onChange } = setup(),
    key = keyButton();
  down(key);
  move(key, 233);
  expect(onChange).not.toHaveBeenCalled();
  expect(key.style.left).toBe("23%");
  up(key, 233);
  expect(onChange).toHaveBeenCalledTimes(1);
  expect(onChange.mock.calls[0][0].objects[0].keyframes[0].time).toBe(2.3);
});
it.each(["escape", "window-blur", "blur", "pointercancel", "lostcapture"])(
  "cancels a key drag on %s without committing",
  (reason) => {
    const { onChange } = setup(),
      key = keyButton();
    down(key);
    move(key, 250);
    if (reason === "escape") fireEvent.keyDown(window, { key: "Escape" });
    else if (reason === "window-blur") fireEvent(window, new Event("blur"));
    else if (reason === "blur") fireEvent.blur(key);
    else if (reason === "pointercancel")
      fireEvent.pointerCancel(key, { pointerId: 1 });
    else fireEvent.lostPointerCapture(key, { pointerId: 1 });
    up(key, 250);
    expect(onChange).not.toHaveBeenCalled();
    expect(key.style.left).toBe("10%");
  },
);
it("rejects stale scenes, collisions, out of bounds, locked and read-only writes", () => {
  const { scene, actor, props, rerender, onChange } = setup();
  let key = keyButton();
  down(key);
  move(key, 250);
  rerender(
    <DirectorTimeline {...props} scene={{ ...scene, title: "远端修改" }} />,
  );
  rerender(<DirectorTimeline {...props} />);
  up(key, 250);
  expect(onChange).not.toHaveBeenCalled();
  rerender(<DirectorTimeline {...props} />);
  key = keyButton();
  down(key);
  move(key, -100);
  up(key, -100);
  expect(onChange).not.toHaveBeenCalled();
  const collision = structuredClone(scene);
  collision.objects[0].keyframes.push({
    ...actor.keyframes[0],
    id: crypto.randomUUID(),
    time: 2,
  });
  rerender(<DirectorTimeline {...props} scene={collision} />);
  key = keyButton();
  down(key);
  move(key, 200);
  up(key, 200);
  expect(onChange).not.toHaveBeenCalled();
  for (const locked of [false, true]) {
    rerender(
      <DirectorTimeline
        {...props}
        disabled={!locked}
        scene={{ ...scene, objects: [{ ...actor, locked }] }}
      />,
    );
    key = keyButton();
    down(key);
    move(key, 250);
    up(key, 250);
  }
  expect(onChange).not.toHaveBeenCalled();
});
it("moves clips without changing source offset and trims both ends with frame snapping", () => {
  const { props, actor, rerender, onChange } = setup();
  const clip = screen.getByRole("button", { name: "移动走路" });
  down(clip);
  move(clip, 230);
  expect(onChange).not.toHaveBeenCalled();
  up(clip, 230);
  expect(onChange.mock.calls[0][0].objects[0].motion_clips[0]).toMatchObject({
    start: 2.3,
    duration: 2,
  });
  onChange.mockClear();
  rerender(<DirectorTimeline {...props} />);
  const left = screen.getByRole("button", { name: "裁剪走路起点" });
  down(left);
  move(left, 150);
  up(left, 150);
  expect(onChange.mock.calls[0][0].objects[0].motion_clips[0]).toMatchObject({
    start: 1.5,
    duration: 1.5,
    source_offset: 1,
  });
  onChange.mockClear();
  const right = screen.getByRole("button", { name: "裁剪走路终点" });
  down(right);
  move(right, 47);
  up(right, 47);
  expect(onChange.mock.calls[0][0].objects[0].motion_clips[0]).toMatchObject({
    start: 1,
    duration: 1.5,
  });
  expect(actor.motion_clips![0].duration).toBe(2);
});
it("rejects clip overlap, negative source in-point and cancelled resize", () => {
  const { props, scene, actor, rerender, onChange } = setup();
  const next = structuredClone(scene);
  next.objects[0].motion_clips!.push({
    ...actor.motion_clips![0],
    id: crypto.randomUUID(),
    start: 4,
  });
  rerender(<DirectorTimeline {...props} scene={next} />);
  const clip = screen.getAllByRole("button", { name: "移动走路" })[0];
  down(clip);
  move(clip, 350);
  up(clip, 350);
  expect(onChange).not.toHaveBeenCalled();
  const left = screen.getAllByRole("button", { name: "裁剪走路起点" })[0];
  down(left);
  move(left, 50);
  up(left, 50);
  expect(onChange).not.toHaveBeenCalled();
  const right = screen.getAllByRole("button", { name: "裁剪走路终点" })[0];
  down(right);
  move(right, 200);
  fireEvent.keyDown(window, { key: "Escape" });
  up(right, 200);
  expect(onChange).not.toHaveBeenCalled();
});

it("keeps numeric key editing usable after a cancelled drag", () => {
  const { onChange } = setup();
  const key = keyButton();
  down(key);
  move(key, 250);
  fireEvent.keyDown(window, { key: "Escape" });
  up(key, 250);
  fireEvent.click(key); // consume the click generated by the cancelled gesture
  fireEvent.click(key);
  const input = screen.getByRole("spinbutton", { name: "关键帧时间" });
  fireEvent.change(input, { target: { value: "3.14" } });
  fireEvent.blur(input);
  expect(onChange).toHaveBeenCalledTimes(1);
  expect(onChange.mock.calls[0][0].objects[0].keyframes[0].time).toBe(3.1);
});

it.each([
  ["移动走路", 1100],
  ["裁剪走路终点", -100],
  ["裁剪走路起点", 900],
  ["裁剪走路终点", 850],
] as const)("rejects an invalid clip boundary from %s", (name, x) => {
  const { onChange } = setup();
  const handle = screen.getByRole("button", { name });
  down(handle);
  move(handle, x);
  up(handle, x);
  expect(onChange).not.toHaveBeenCalled();
  expect(screen.getByRole("alert").textContent).toMatch(/范围/);
});
it("preserves a nonzero source in-point when moving a clip", () => {
  const { scene, props, rerender, onChange } = setup();
  const changed = structuredClone(scene);
  changed.objects[0].motion_clips![0].source_offset = 0.75;
  rerender(<DirectorTimeline {...props} scene={changed} />);
  const clip = screen.getByRole("button", { name: "移动走路" });
  down(clip);
  move(clip, 200);
  up(clip, 200);
  expect(onChange.mock.calls[0][0].objects[0].motion_clips[0]).toMatchObject({
    source_offset: 0.75,
    start: 2,
    duration: 2,
  });
});
