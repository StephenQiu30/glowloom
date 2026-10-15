import { describe, expect, it } from "vitest";
import { modelConfiguration, priceRule } from "./model-setup";
function form(values: Record<string, string | string[]>) {
  const f = new FormData();
  for (const [key, value] of Object.entries(values))
    for (const v of Array.isArray(value) ? value : [value]) f.append(key, v);
  return f;
}
describe("basic model setup", () => {
  it("builds declared frame and reference modes with millisecond duration", () => {
    const result = modelConfiguration(
      form({
        max_outputs: "1",
        durations: "5, 10",
        modes: ["image2video", "omni_reference"],
        max_references: "3",
        aspect_ratios: "16:9, 9:16",
      }),
      "video.generate",
    );
    expect(result.limits).toMatchObject({
      durations_ms: [5000, 10000],
      roles: {
        first_frame: { max_count: 1, for_modes: ["image2video"] },
        reference_image: { max_count: 3, for_modes: ["omni_reference"] },
      },
    });
    expect(result.param_schema).toContainEqual(
      expect.objectContaining({ field: "duration_ms", default: 5000 }),
    );
  });
  it("rejects missing reference limits and invalid duration before saving", () => {
    expect(() =>
      modelConfiguration(
        form({ max_outputs: "1", durations: "5", modes: "omni_reference" }),
        "video.generate",
      ),
    ).toThrow();
    expect(() =>
      modelConfiguration(
        form({ max_outputs: "1", durations: "NaN" }),
        "video.generate",
      ),
    ).toThrow();
  });
  it("preserves advanced rules and converts visible currency amounts", () => {
    const advanced = {
      roles: { subject: { max_count: 4, types: ["image"] } },
      max_outputs: 2,
    };
    expect(
      modelConfiguration(
        form({
          setup_mode: "advanced",
          limits: JSON.stringify(advanced),
          parameters: "[]",
        }),
        "image.generate",
      ).limits,
    ).toEqual(advanced);
    expect(
      priceRule(
        form({
          unit: "per_1k_tokens",
          input_rate: "0.001",
          output_rate: "0.002",
        }),
      ),
    ).toEqual({ input_micros_per_1k: 1000, output_micros_per_1k: 2000 });
    expect(() =>
      priceRule(form({ unit: "per_request", base_rate: "0.0000001" })),
    ).toThrow();
  });
});
