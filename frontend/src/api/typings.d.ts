declare namespace API {
  type DependencyStatus = {
    /** Critical */
    critical: boolean;
    /** Status */
    status: "available" | "degraded" | "unavailable";
    /** Reason */
    reason: string | null | null;
  };

  type HealthResponse = {
    /** Status */
    status: string | null;
  };

  type ReadinessResponse = {
    /** Status */
    status: "ready" | "degraded" | "unavailable";
    /** Dependencies */
    dependencies: Record<string, any>;
  };
}
