declare namespace AuthAPI {
  type Actor = {
    credential_revision: number;
    display_name: string;
    id: string;
    login_name: string;
    must_change_password: boolean;
    role: "creator" | "admin";
  };

  type AvailabilityView = {
    available: boolean;
  };

  type deleteSessionParams = {
    /** Current session UUID */
    session_id: string;
  };

  type getLoginAvailabilityParams = {
    /** ASCII login name */
    login_name: string;
  };

  type LoginRequest = {
    login_name: string;
    password: string;
    persistent: boolean;
  };

  type PasswordChange = {
    confirm_password: string;
    current_password: string;
    expected_credential_revision: number;
    new_password: string;
  };

  type Problem = {
    code?: string;
    detail?: string;
    meta?: Record<string, any>;
    request_id?: string;
    status?: number;
    title?: string;
    type?: string;
  };

  type RegisterRequest = {
    confirm_password: string;
    display_name: string;
    login_name: string;
    password: string;
  };

  type SessionView = {
    absolute_expires_at: string;
    actor: Actor;
    last_active_at: string;
    persistent: boolean;
    session_id: string;
  };
}
