import { getCanvasDocument } from "@/components/canvas/runtime/api/canvasDocumentBffController";
import { ApiError, unwrap } from "@/components/canvas/runtime/lib/request";

type CanvasCommandVersion = {
  canvasId?: number | string;
  id?: number | string;
  expectedRevision: number | string;
  expectedProjectBindingRevision: number | string;
  expectedEpoch?: string;
  expectedCommitSeq?: string;
};
const scopeValid = (epoch: unknown, sequence: unknown) =>
  typeof epoch === "string" &&
  /^[a-f0-9]{8}(?:-[a-f0-9]{4}){3}-[a-f0-9]{12}$/.test(epoch) &&
  typeof sequence === "string" &&
  /^(0|[1-9][0-9]{0,18})$/.test(sequence);

/** Qualifies a fresh resource intent once. An uncertain original always retains the already frozen scope. */
export async function qualifyCanvasCommand<T extends CanvasCommandVersion>(
  body: T,
  mayWrite: () => boolean,
): Promise<T & { expectedEpoch: string; expectedCommitSeq: string }> {
  const requireCurrentActor = () => {
    if (!mayWrite())
      throw new ApiError("账户或画布权限已暂停，请重新验证", 40100);
  };
  requireCurrentActor();
  if (body.expectedEpoch != null || body.expectedCommitSeq != null) {
    if (!scopeValid(body.expectedEpoch, body.expectedCommitSeq))
      throw new ApiError("原命令的画布确认范围无效，请保留记录核对", 40901);
    return body as T & { expectedEpoch: string; expectedCommitSeq: string };
  }
  const canvasId = String(body.canvasId ?? body.id ?? "");
  if (!/^[1-9][0-9]*$/.test(canvasId))
    throw new ApiError("画布身份无效", 40901);
  const head = unwrap(await getCanvasDocument({ canvasId }));
  requireCurrentActor();
  if (
    String(head.canvasId) !== canvasId ||
    String(head.revision) !== String(body.expectedRevision) ||
    String(head.projectBindingRevision) !==
      String(body.expectedProjectBindingRevision)
  )
    throw new ApiError("画布内容或归属版本已变化，请重新确认", 40901);
  if (!scopeValid(head.epoch, head.commitSeq))
    throw new ApiError("画布同步范围未就绪", 50300);
  return Object.freeze({
    ...body,
    expectedEpoch: head.epoch,
    expectedCommitSeq: head.commitSeq,
  });
}
