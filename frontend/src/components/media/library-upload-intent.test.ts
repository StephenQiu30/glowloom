import { expect, it } from "vitest";
import {
  loadLibraryUploads,
  saveLibraryUploads,
  uploadFingerprint,
  requireOriginalUpload,
  validateLibraryFiles,
} from "./library-upload-intent";
const identity = {
  origin: window.location.origin,
  actorId: "11111111-1111-4111-8111-111111111111",
  orgId: "22222222-2222-4222-8222-222222222222",
  libraryId: "33333333-3333-5333-8333-333333333333",
  scope: { kind: "personal" as const },
};
it("按origin/currentActor/org/library隔离，仅原key/file fingerprint持久，损坏与quota阻断", async () => {
  sessionStorage.clear();
  const file = new File(["real"], "source.txt", { type: "text/plain" });
  const intent = {
    key: crypto.randomUUID(),
    ...(await uploadFingerprint(file)),
    local_review_confirmed: true as const,
  };
  saveLibraryUploads(sessionStorage, identity, [intent]);
  expect(loadLibraryUploads(sessionStorage, identity)).toEqual([intent]);
  expect(
    loadLibraryUploads(sessionStorage, {
      ...identity,
      actorId: identity.orgId,
    }),
  ).toEqual([]);
  expect(
    [...Object.keys(sessionStorage)].every(
      (key) => !sessionStorage.getItem(key)?.includes("blob:"),
    ),
  ).toBe(true);
  expect(() =>
    saveLibraryUploads(
      {
        getItem: () => null,
        setItem: () => {
          throw Error("quota");
        },
        removeItem: () => {},
      },
      identity,
      [intent],
    ),
  ).toThrow();
  await expect(
    requireOriginalUpload(
      new File(["edit"], file.name, { type: file.type }),
      intent,
    ),
  ).rejects.toThrow("SHA");
  await requireOriginalUpload(file, intent);
});
it("真实25文件/700MiB与4类加TXT-DOCX容量限制，路径和不支持格式不送DML", () => {
  expect(validateLibraryFiles([new File(["x"], "source.txt")]).valid).toBe(
    true,
  );
  expect(validateLibraryFiles([new File(["x"], "../source.txt")]).valid).toBe(
    false,
  );
  expect(
    validateLibraryFiles([new File(["x"], "raw.gif", { type: "image/gif" })])
      .valid,
  ).toBe(false);
  expect(
    validateLibraryFiles(
      Array.from({ length: 26 }, () => new File(["x"], "a.png")),
    ).valid,
  ).toBe(false);
});
