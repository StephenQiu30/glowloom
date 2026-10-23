import type { CanvasBackgroundMode } from "@/components/canvas/runtime/lib/theme";
import type {
  CanvasAssistantSession,
  CanvasConnection,
  CanvasNodeData,
  ViewportTransform,
} from "@/components/canvas/types/canvas";

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
