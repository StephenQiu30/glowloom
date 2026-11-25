export type CanvasPendingForms = Record<
  string,
  { values: Record<string, string> }
>;
export type CanvasLocalPersistence =
  "CHECKING" | "MEMORY" | "PERSISTING" | "PERSISTED" | "UNAVAILABLE";
