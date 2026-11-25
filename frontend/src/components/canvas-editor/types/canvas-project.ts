import type { CanvasBackgroundMode } from "../runtime/lib/theme.ts";
import type {
  CanvasAssistantSession,
  CanvasConnection,
  CanvasNodeData,
  ViewportTransform,
} from "./canvas.ts";

/** Editor draft shape; backend project identity and revision live in the API adapter. */
export type CanvasProject = {
  id: string;
  title: string;
  createdAt: string;
  updatedAt: string;
  nodes: CanvasNodeData[];
  connections: CanvasConnection[];
  chatSessions: CanvasAssistantSession[];
  activeChatId: string | null;
  backgroundMode: CanvasBackgroundMode;
  showImageInfo: boolean;
  viewport: ViewportTransform;
};
