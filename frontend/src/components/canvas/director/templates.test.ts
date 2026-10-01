import { describe, expect, it } from "vitest";
import { directorSchema } from "./model";
import {
  createDirectorSceneFromTemplate,
  DIRECTOR_TEMPLATES,
} from "./templates";

describe("导演台场景模板", () => {
  it("五种模板形成可保存场景，空场景和产品镜头不附带演员", () => {
    for (const template of DIRECTOR_TEMPLATES) {
      const scene = createDirectorSceneFromTemplate(template.id, template.name);
      expect(directorSchema.safeParse(scene).success).toBe(true);
      expect(scene.lights).toHaveLength(3);
      expect(scene.shots[0].cameraId).toBe(scene.cameras[0].id);
      const actors = scene.objects.filter((object) => object.kind === "actor");
      const counts = {
        empty: 0,
        monologue: 1,
        dialogue: 2,
        blocking: 3,
        product: 0,
      };
      expect(actors).toHaveLength(counts[template.id]);
    }
  });
  it("相同模板每次产生独立身份和可独立编辑的数据", () => {
    const first = createDirectorSceneFromTemplate("dialogue");
    const second = createDirectorSceneFromTemplate("dialogue");
    expect(first.id).not.toBe(second.id);
    expect(first.objects[0].id).not.toBe(second.objects[0].id);
    expect(first.cameras[0].id).not.toBe(second.cameras[0].id);
    first.objects[0].transform.position[0] = 100;
    expect(second.objects[0].transform.position[0]).toBe(-0.8);
    expect(first.cameras[0].fov).toBeCloseTo(48.45549, 4);
  });
});
