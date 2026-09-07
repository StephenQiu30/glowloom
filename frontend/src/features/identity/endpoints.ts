import {
  loginApiAuthLoginPost,
  registerApiAuthRegisterPost,
  requestRegistrationVerificationApiAuthRegistrationVerificationsPost,
  confirmRegistrationVerificationApiAuthRegistrationVerificationsConfirmPost,
  meApiMeGet,
  logoutApiAuthLogoutPost,
  updateMeApiMePatch,
  changePasswordApiAuthChangePasswordPost,
  deactivateMeApiMeDeactivatePost,
  listWorkspacesApiWorkspacesGet,
  createWorkspaceApiWorkspacesPost,
  updateWorkspaceApiWorkspacesWorkspaceIdPatch,
  archiveWorkspaceApiWorkspacesWorkspaceIdArchivePost,
  restoreWorkspaceApiWorkspacesWorkspaceIdRestorePost,
} from "@/api/identity";
import { appApi, runRequest } from "@/lib/server-state";

export const identityApi = appApi.injectEndpoints({
  endpoints: (builder) => ({
    login: builder.mutation<API.AuthResponse, API.LoginRequest>({
      queryFn: (body) => runRequest(() => loginApiAuthLoginPost(body)),
    }),
    register: builder.mutation<API.AuthResponse, API.RegisterRequest>({
      queryFn: (body) => runRequest(() => registerApiAuthRegisterPost(body)),
    }),
    requestRegistrationVerification: builder.mutation<
      API.RegistrationVerificationAccepted,
      API.RegistrationVerificationRequest
    >({
      queryFn: (body) =>
        runRequest(() =>
          requestRegistrationVerificationApiAuthRegistrationVerificationsPost(body),
        ),
    }),
    confirmRegistrationVerification: builder.mutation<
      API.RegistrationVerificationConfirmed,
      API.RegistrationVerificationConfirmRequest
    >({
      queryFn: (body) =>
        runRequest(() =>
          confirmRegistrationVerificationApiAuthRegistrationVerificationsConfirmPost(
            body,
          ),
        ),
    }),
    me: builder.query<API.MeResponse, void>({
      queryFn: () => runRequest(() => meApiMeGet()),
      providesTags: ["Me"],
    }),
    logout: builder.mutation<API.RevocationResponse, void>({
      queryFn: () => runRequest(() => logoutApiAuthLogoutPost()),
    }),
    updateProfile: builder.mutation<API.MeResponse, API.ProfileUpdateRequest>({
      queryFn: (body) => runRequest(() => updateMeApiMePatch(body)),
      invalidatesTags: ["Me"],
    }),
    changePassword: builder.mutation<API.RevocationResponse, API.ChangePasswordRequest>({
      queryFn: (body) => runRequest(() => changePasswordApiAuthChangePasswordPost(body)),
    }),
    deactivateAccount: builder.mutation<
      API.RevocationResponse,
      API.DeactivateAccountRequest
    >({
      queryFn: (body) => runRequest(() => deactivateMeApiMeDeactivatePost(body)),
    }),
    workspaces: builder.query<API.WorkspaceResponse[], void>({
      queryFn: () =>
        runRequest(() =>
          listWorkspacesApiWorkspacesGet({ include_archived: true }),
        ),
      providesTags: ["Workspaces"],
    }),
    createWorkspace: builder.mutation<
      API.WorkspaceResponse,
      API.WorkspaceCreateRequest
    >({
      queryFn: (body) => runRequest(() => createWorkspaceApiWorkspacesPost(body)),
      invalidatesTags: ["Workspaces"],
    }),
    updateWorkspace: builder.mutation<
      API.WorkspaceResponse,
      { workspaceId: string; body: API.WorkspaceUpdateRequest }
    >({
      queryFn: ({ workspaceId, body }) =>
        runRequest(() =>
          updateWorkspaceApiWorkspacesWorkspaceIdPatch(
            { workspace_id: workspaceId },
            body,
          ),
        ),
      invalidatesTags: ["Me", "Workspaces"],
    }),
    setWorkspaceArchived: builder.mutation<
      API.WorkspaceResponse,
      { workspaceId: string; expectedRevision: number; archived: boolean }
    >({
      queryFn: ({ workspaceId, expectedRevision, archived }) => {
        const params = { workspace_id: workspaceId };
        const body = { expected_revision: expectedRevision };
        return runRequest(() =>
          archived
            ? archiveWorkspaceApiWorkspacesWorkspaceIdArchivePost(params, body)
            : restoreWorkspaceApiWorkspacesWorkspaceIdRestorePost(params, body),
        );
      },
      invalidatesTags: ["Me", "Workspaces", "Projects"],
    }),
  }),
});

export const {
  useChangePasswordMutation,
  useConfirmRegistrationVerificationMutation,
  useCreateWorkspaceMutation,
  useDeactivateAccountMutation,
  useLoginMutation,
  useLogoutMutation,
  useMeQuery,
  useRegisterMutation,
  useRequestRegistrationVerificationMutation,
  useSetWorkspaceArchivedMutation,
  useUpdateProfileMutation,
  useUpdateWorkspaceMutation,
  useWorkspacesQuery,
} = identityApi;
