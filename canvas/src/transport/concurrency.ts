export class ConcurrencyGate {
  private active = 0;

  constructor(readonly maxConcurrent = 4) {
    if (!Number.isSafeInteger(maxConcurrent) || maxConcurrent < 1) {
      throw new Error("INVALID_MAX_CONCURRENT");
    }
  }

  get inFlight(): number {
    return this.active;
  }

  tryAcquire(): (() => void) | null {
    if (this.active >= this.maxConcurrent) return null;
    this.active += 1;
    let released = false;
    return () => {
      if (released) return;
      released = true;
      this.active -= 1;
    };
  }
}

export interface ClassifiedRuntimeError {
  readonly code: string;
  readonly definitive: boolean;
}

export function classifyRuntimeError(error: unknown): ClassifiedRuntimeError {
  const failure = error && typeof error === "object" ? error : {};
  const definitive = "definitive" in failure && failure.definitive === true;
  const code =
    "code" in failure && typeof failure.code === "string"
      ? failure.code
      : "RUNTIME_UNAVAILABLE";
  return { code, definitive };
}
