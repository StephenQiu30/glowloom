export type WireFrame = {
  type: "YJS_CHUNK";
  id: string;
  index: number;
  count: number;
  total: number;
  hash: string;
  data: string;
};
export function wireFrames(
  message: unknown,
  maxBytes?: number,
): Promise<WireFrame[]>;
export class WireAssembler {
  constructor(options?: { maxBytes?: number; now?: () => number });
  readonly pendingBytes: number;
  clear(): void;
  accept(frame: unknown): Promise<unknown | null>;
}
