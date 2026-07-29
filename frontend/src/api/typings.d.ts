declare namespace API {
  type ApiResponseAuthResponse_ = {
    data: AuthResponse;
  };

  type ApiResponseMeResponse_ = {
    data: MeResponse;
  };

  type ApiResponseRevocationResponse_ = {
    data: RevocationResponse;
  };

  type AuthResponse = {
    user: UserResponse;
    workspace: WorkspaceResponse;
    /** Access Token */
    access_token: string;
    /** Token Type */
    token_type: string | null;
    /** Expires In */
    expires_in: number;
  };

  type ChangePasswordRequest = {
    /** Current Password */
    current_password: string;
    /** New Password */
    new_password: string;
  };

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

  type HTTPValidationError = {
    /** Detail */
    detail: ValidationError[] | null;
  };

  type LoginRequest = {
    /** Email */
    email: string;
    /** Password */
    password: string;
  };

  type MeResponse = {
    user: UserResponse;
    workspace: WorkspaceResponse;
  };

  type ReadinessResponse = {
    /** Status */
    status: "ready" | "degraded" | "unavailable";
    /** Dependencies */
    dependencies: Record<string, any>;
  };

  type RegisterRequest = {
    /** Email */
    email: string;
    /** Password */
    password: string;
    /** Display Name */
    display_name: string;
  };

  type RevocationResponse = {
    /** Revoked */
    revoked: boolean | null;
  };

  type UserResponse = {
    /** Id */
    id: string;
    /** Email */
    email: string;
    /** Display Name */
    display_name: string;
    /** Avatar Url */
    avatar_url: string | null;
  };

  type ValidationError = {
    /** Location */
    loc: (string | number)[];
    /** Message */
    msg: string;
    /** Error Type */
    type: string;
    /** Input */
    input: any | null;
    /** Context */
    ctx: Record<string, any> | null;
  };

  type WorkspaceResponse = {
    /** Id */
    id: string;
    /** Name */
    name: string;
    /** Status */
    status: "active" | "archived";
    /** Role */
    role: "owner" | "editor" | "viewer";
  };
}
