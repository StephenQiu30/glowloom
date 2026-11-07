import { beforeEach, expect, it, vi } from "vitest";
import { RequestError } from "@/lib/request";
import {
  createStyle,
  deleteStyle,
  freezeStyle,
  listStyles,
  readStyle,
  updateStyle,
} from "@/components/styles/style-api";

const network = vi.hoisted(() => ({ request: vi.fn() }));
vi.mock("@/lib/request", async (original) => ({
  ...(await original<typeof import("@/lib/request")>()),
  request: network.request,
}));
const id = "11111111-1111-4111-8111-111111111111";
const asset = "22222222-2222-4222-8222-222222222222";
const key = "33333333-3333-4333-8333-333333333333";
const detail = {
  id,
  name: "水墨",
  category: "2d",
  revision: 1,
  cover_asset_id: asset,
  reference_asset_ids: [asset],
  owned: true,
  can_edit: true,
  prompt_fragment: "疏朗的水墨构图",
  negative_prompt: "避免霓虹色",
  cover_item_id: asset,
  references_available: true,
  reference_items: [
    {
      id: asset,
      revision: 1,
      asset_id: asset,
      asset_revision: 1,
      title: "参考图",
    },
  ],
};
const input = {
  name: "水墨",
  category: "2d" as const,
  prompt_fragment: "疏朗的水墨构图",
  reference_asset_ids: [asset],
};
beforeEach(() => vi.clearAllMocks());

it("reads the actual detail before creating an immutable independent selection", async () => {
  network.request.mockResolvedValue(detail);
  const value = freezeStyle(await readStyle(id));
  detail.reference_asset_ids.push("44444444-4444-4444-8444-444444444444");
  expect(value.reference_asset_ids).toEqual([asset]);
  expect(Object.isFrozen(value)).toBe(true);
  expect(Object.isFrozen(value.reference_asset_ids)).toBe(true);
  expect(value.negative_prompt).toBe("避免霓虹色");
  detail.reference_asset_ids = [asset];
});

it("rejects a list whose cover is not its first reference", async () => {
  network.request.mockResolvedValue({
    items: [{ ...detail, cover_asset_id: null }],
    next_cursor: null,
  });
  await expect(
    listStyles({ mine: false, category: "2d", limit: 24 }),
  ).rejects.toMatchObject({ code: "invalid_receipt" });
});

it("preserves official versus personal query and opaque pagination cursor", async () => {
  network.request.mockResolvedValue({
    items: [detail],
    next_cursor: "opaque-next",
  });
  expect(
    (await listStyles({ mine: false, q: "水墨", cursor: "opaque", limit: 24 }))
      .next_cursor,
  ).toBe("opaque-next");
  expect(network.request).toHaveBeenCalledWith(
    "/api/style-library",
    expect.objectContaining({
      method: "GET",
      params: { mine: false, q: "水墨", cursor: "opaque", limit: 24 },
    }),
  );
});

it("sends the same idempotency key and full revision contract for retries", async () => {
  network.request.mockResolvedValue({
    request_id: key,
    action: "patch",
    preset: detail,
    deleted: false,
  });
  await updateStyle(id, { ...input, expected_revision: 1 }, key);
  expect(network.request).toHaveBeenCalledWith(
    `/api/style-library/${id}`,
    expect.objectContaining({
      method: "PATCH",
      data: { ...input, expected_revision: 1 },
      headers: { "Idempotency-Key": key },
    }),
  );
});

it("does not accept a write receipt belonging to another idempotency request", async () => {
  network.request.mockResolvedValue({
    request_id: id,
    action: "create",
    preset: detail,
    deleted: false,
  });
  await expect(createStyle(input, key)).rejects.toMatchObject({
    code: "invalid_receipt",
  });
});

it("retains the server conflict when a bound style cannot be deleted", async () => {
  network.request.mockRejectedValue(
    new RequestError("style_preset_in_use", 409),
  );
  await expect(deleteStyle(id, 1, key)).rejects.toMatchObject({
    code: "style_preset_in_use",
    status: 409,
  });
  expect(network.request).toHaveBeenCalledWith(
    `/api/style-library/${id}`,
    expect.objectContaining({
      method: "DELETE",
      data: { expected_revision: 1 },
      headers: { "Idempotency-Key": key },
    }),
  );
});

it("lets an owner read unavailable references for repair but refuses selecting that snapshot", async () => {
  network.request.mockResolvedValue({
    ...detail,
    cover_item_id: undefined,
    reference_items: [],
    references_available: false,
  });
  const editable = await readStyle(id);
  expect(editable.prompt_fragment).toBe(detail.prompt_fragment);
  expect(() => freezeStyle(editable)).toThrowError(
    "style_reference_unavailable",
  );
});

it("拒绝不符合素材目录起始版本的风格参考回执", async () => {
  network.request.mockResolvedValue({
    ...detail,
    reference_items: [{ ...detail.reference_items[0], revision: 0 }],
  });
  await expect(readStyle(id)).rejects.toMatchObject({
    code: "invalid_receipt",
  });
});
