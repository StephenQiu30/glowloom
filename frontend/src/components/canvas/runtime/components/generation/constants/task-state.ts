export const taskLabels: Record<
  NonNullable<CanvasContract.GenerationTaskVO["state"]>,
  string
> = {
  PREPARING_FUNDS: "正在准备积分",
  QUEUED: "等待执行",
  SUBMITTING: "正在生成",
  SUBMISSION_UNKNOWN: "提交结果待核对",
  WAITING: "视频正在生成",
  QUERYING: "正在查询原视频",
  OUTPUT_READY: "正在归档",
  ARCHIVE_FAILED: "归档暂未完成",
  DISPATCH_FAILED: "任务未能启动",
  FAILED: "生成失败",
  SUCCEEDED: "已完成",
  CANCELLED: "已取消",
};
