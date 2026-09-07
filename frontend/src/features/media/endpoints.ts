import {
  initializeUploadApiMediaUploadsPost,
  completeUploadApiMediaUploadsUploadSessionIdCompletePost,
  getMediaApiMediaVersionIdGet,
} from "@/api/media";
import { appApi, runRequest } from "@/lib/server-state";

export const mediaApi = appApi.injectEndpoints({
  endpoints: (builder) => ({
    initializeMediaUpload: builder.mutation<
      API.UploadInitializationResponse,
      API.UploadDeclaration
    >({
      queryFn: (body) =>
        runRequest(() => initializeUploadApiMediaUploadsPost(body)),
    }),
    completeMediaUpload: builder.mutation<
      API.UploadCompletionResponse,
      { uploadSessionId: string; workspaceId: string }
    >({
      queryFn: ({ uploadSessionId }) =>
        runRequest(() =>
          completeUploadApiMediaUploadsUploadSessionIdCompletePost({
            upload_session_id: uploadSessionId,
          }),
        ),
      invalidatesTags: (_result, _error, { workspaceId }) => [
        { type: "Media", id: workspaceId },
      ],
    }),
    mediaVersion: builder.query<API.MediaVersionResponse, string>({
      queryFn: (versionId) =>
        runRequest(() =>
          getMediaApiMediaVersionIdGet({ version_id: versionId }),
        ),
      providesTags: (_result, _error, versionId) => [
        { type: "Media", id: versionId },
      ],
    }),
  }),
});

export const {
  useCompleteMediaUploadMutation,
  useInitializeMediaUploadMutation,
  useLazyMediaVersionQuery,
} = mediaApi;
