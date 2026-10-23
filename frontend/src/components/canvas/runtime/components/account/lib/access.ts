type AccountState = {
  account: CanvasContract.AccountVO | null;
  ready: boolean;
  error?: Error;
};

// 创作空间按登录身份开放，管理空间按服务端管理员身份展示。
const policies = {
  public: () => true,
  authenticated: () => true,
  viewDashboard: (account: CanvasContract.AccountVO) =>
    account.userRole === "admin",
  viewLogs: (account: CanvasContract.AccountVO) => account.userRole === "admin",
  manageAssets: (account: CanvasContract.AccountVO) =>
    account.userRole === "admin",
  manageCredits: (account: CanvasContract.AccountVO) =>
    account.userRole === "admin",
  readCreditActivity: (account: CanvasContract.AccountVO) =>
    account.userRole === "admin" &&
    account.actions?.includes("billing.read") === true,
  manageUsers: (account: CanvasContract.AccountVO) =>
    account.userRole === "admin",
  manageAuthority: (account: CanvasContract.AccountVO) =>
    account.userRole === "admin",
  readModels: (account: CanvasContract.AccountVO) =>
    account.userRole === "admin",
  manageModels: (account: CanvasContract.AccountVO) =>
    account.userRole === "admin",
  readTeams: (account: CanvasContract.AccountVO) =>
    account.actions?.includes("team.read") === true,
  createTeams: (account: CanvasContract.AccountVO) =>
    account.actions?.includes("team.create") === true,
  manageTeams: (account: CanvasContract.AccountVO) =>
    account.actions?.includes("team.manage") === true,
  participateTeams: (account: CanvasContract.AccountVO) =>
    account.actions?.includes("team.participate") === true,
  denied: () => false,
} satisfies Record<string, (account: CanvasContract.AccountVO) => boolean>;

export type AccessKey = keyof typeof policies;
export type AccessState =
  "allowed" | "loading" | "error" | "anonymous" | "forbidden";

export function getAccessState(
  state: AccountState,
  access: AccessKey,
): AccessState {
  if (access === "public") return "allowed";
  if (!Object.hasOwn(policies, access) || access === "denied")
    return "forbidden";
  if (!state.ready) return "loading";
  if (state.error) return "error";
  if (!state.account) return "anonymous";
  return policies[access](state.account) ? "allowed" : "forbidden";
}

export function createAccess(state: AccountState): Record<AccessKey, boolean> {
  return {
    public: true,
    authenticated: getAccessState(state, "authenticated") === "allowed",
    viewDashboard: getAccessState(state, "viewDashboard") === "allowed",
    viewLogs: getAccessState(state, "viewLogs") === "allowed",
    manageAssets: getAccessState(state, "manageAssets") === "allowed",
    manageCredits: getAccessState(state, "manageCredits") === "allowed",
    readCreditActivity:
      getAccessState(state, "readCreditActivity") === "allowed",
    manageUsers: getAccessState(state, "manageUsers") === "allowed",
    manageAuthority: getAccessState(state, "manageAuthority") === "allowed",
    readModels: getAccessState(state, "readModels") === "allowed",
    manageModels: getAccessState(state, "manageModels") === "allowed",
    readTeams: getAccessState(state, "readTeams") === "allowed",
    createTeams: getAccessState(state, "createTeams") === "allowed",
    manageTeams: getAccessState(state, "manageTeams") === "allowed",
    participateTeams: getAccessState(state, "participateTeams") === "allowed",
    denied: false,
  };
}
