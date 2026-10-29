import { fireEvent, render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import {
  Bone,
  BufferGeometry,
  Float32BufferAttribute,
  Group,
  Skeleton,
  SkinnedMesh,
} from "three";
import { createDirectorActor } from "@/components/canvas/director/director-assets";
import {
  editDirector,
  emptyDirectorConfig,
  readDirector,
} from "@/components/canvas/director/director-model";
import { directorBodyProfile } from "@/components/canvas/director/director-body-profile";
import { reshapeDirectorQuaterniusActor } from "@/components/canvas/director/director-actor-body";
import { DirectorBodyEditor } from "@/components/canvas/director/director-body-editor";

it("keeps preset values for omitted body parameters and deforms geometry and rest bones together", () => {
  expect(directorBodyProfile("male_athletic", { waist: 0.8 })).toMatchObject({
    leg: 1.08,
    waist: 0.8,
  });
  const root = new Group(),
    bone = new Bone(),
    geometry = new BufferGeometry();
  bone.position.y = 0.95;
  geometry.setAttribute(
    "position",
    new Float32BufferAttribute([0, 0.95, 0.1], 3),
  );
  const mesh = new SkinnedMesh(geometry);
  root.add(bone, mesh);
  mesh.bind(new Skeleton([bone]));
  reshapeDirectorQuaterniusActor(root, "standard", { leg: 0.5 });
  expect(mesh.geometry.getAttribute("position").getY(0)).toBeCloseTo(0.475);
  expect(bone.position.y).toBeCloseTo(0.475);
  expect(geometry.getAttribute("position").getY(0)).toBeCloseTo(0.95);
  mesh.geometry.dispose();
  mesh.skeleton.dispose();
});
it("validates body parameters and permits overrides only for bundled actors", () => {
  const scene = emptyDirectorConfig(),
    object = createDirectorActor("male_child");
  scene.objects = [{ ...object, body_profile: { leg: 0.55, headWidth: 1.27 } }];
  expect(
    readDirector(JSON.parse(JSON.stringify(scene))).objects[0].body_profile,
  ).toEqual(scene.objects[0].body_profile);
  for (const body_profile of [
    { leg: 0 },
    { leg: 1.61 },
    { leg: NaN },
    { unknown: 1 },
    [],
  ])
    expect(() =>
      readDirector({
        ...scene,
        objects: [{ ...object, body_profile } as typeof object],
      }),
    ).toThrow("身体参数");
  expect(() =>
    readDirector({
      ...scene,
      objects: [{ ...scene.objects[0], asset_id: crypto.randomUUID() }],
    }),
  ).toThrow("内置角色");
});
it("clears frozen snapshots on static body edits and reset, while rejecting dynamic edits", () => {
  const scene = emptyDirectorConfig(),
    object = createDirectorActor("female_standard");
  object.frozen_bones = { pelvis: object.transform };
  object.frozen_nodes = { root: { transform: object.transform } };
  scene.objects = [object];
  const edit = {
    type: "entity" as const,
    selection: { kind: "object" as const, id: object.id },
    patch: { body_profile: { waist: 0.9 } },
  };
  const result = editDirector(scene, edit);
  expect(result.objects[0].body_profile).toEqual({ waist: 0.9 });
  expect(result.objects[0].frozen_nodes).toBeUndefined();
  expect(result.objects[0].frozen_bones).toBeUndefined();
  expect(scene.objects[0].frozen_nodes).toBeDefined();
  const reset = editDirector(result, {
    ...edit,
    patch: { body_profile: undefined },
  });
  expect(reset.objects[0].body_profile).toBeUndefined();
  expect(reset.objects[0].builtin_actor).toBe("female_standard");
  expect(() => editDirector({ ...scene, mode: "dynamic" }, edit)).toThrow(
    "静态",
  );
});
it("edits real bounded parameters, resets overrides and respects disabled state", () => {
  const object = createDirectorActor("male_child"),
    onChange = vi.fn();
  const { rerender } = render(
    <DirectorBodyEditor object={object} disabled={false} onChange={onChange} />,
  );
  expect(screen.getAllByRole("spinbutton")).toHaveLength(14);
  const leg = screen.getByRole("spinbutton", { name: "腿长" });
  expect((leg as HTMLInputElement).value).toBe("0.55");
  fireEvent.change(leg, { target: { value: "2" } });
  fireEvent.blur(leg);
  expect(onChange).not.toHaveBeenCalled();
  fireEvent.change(leg, { target: { value: ".7" } });
  fireEvent.blur(leg);
  expect(onChange).toHaveBeenCalledWith({
    body_profile: { leg: 0.7 },
    frozen_nodes: undefined,
    frozen_bones: undefined,
  });
  rerender(
    <DirectorBodyEditor
      object={{ ...object, body_profile: { leg: 0.7 } }}
      disabled={false}
      onChange={onChange}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "重置身体参数" }));
  expect(onChange).toHaveBeenLastCalledWith({
    body_profile: undefined,
    frozen_nodes: undefined,
    frozen_bones: undefined,
  });
  rerender(<DirectorBodyEditor object={object} disabled onChange={onChange} />);
  expect(
    (screen.getByRole("spinbutton", { name: "腿长" }) as HTMLInputElement)
      .disabled,
  ).toBe(true);
});

it("deforms left and right limbs independently without changing the opposite side", () => {
  const root = new Group(),
    left = new Bone(),
    right = new Bone(),
    geometry = new BufferGeometry();
  left.position.set(0.2, 0.2, 0);
  right.position.set(-0.2, 0.2, 0);
  geometry.setAttribute(
    "position",
    new Float32BufferAttribute(
      [0.2, 0.2, 0, -0.2, 0.2, 0, 0.6, 1.4, 0, -0.6, 1.4, 0],
      3,
    ),
  );
  const mesh = new SkinnedMesh(geometry);
  root.add(left, right, mesh);
  mesh.bind(new Skeleton([left, right]));
  reshapeDirectorQuaterniusActor(root, "standard", {
    leftLeg: 1.3,
    leftArm: 1.3,
  });
  expect(left.position.y).toBeCloseTo(0.95 + (0.2 - 0.95) * 1.3);
  expect(right.position.y).toBeCloseTo(0.2);
  expect(mesh.geometry.getAttribute("position").getY(0)).toBeCloseTo(
    left.position.y,
  );
  expect(mesh.geometry.getAttribute("position").getY(1)).toBeCloseTo(
    right.position.y,
  );
  expect(mesh.geometry.getAttribute("position").getX(2)).toBeCloseTo(0.78);
  expect(mesh.geometry.getAttribute("position").getX(3)).toBeCloseTo(-0.6);
  mesh.geometry.dispose();
  mesh.skeleton.dispose();
});
