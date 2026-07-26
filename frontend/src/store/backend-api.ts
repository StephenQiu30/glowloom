"use client";

import { createApi, fakeBaseQuery } from "@reduxjs/toolkit/query/react";

import { confirmScript } from "@/api/confirmScript";
import { confirmSource } from "@/api/confirmSource";
import { confirmStoryboard } from "@/api/confirmStoryboard";
import { createProject } from "@/api/createProject";
import { createSourceRevision } from "@/api/createSourceRevision";
import { generateScript } from "@/api/generateScript";
import { generateStoryboard } from "@/api/generateStoryboard";
import { listProjects } from "@/api/listProjects";
import { listScriptVersions } from "@/api/listScriptVersions";
import { listSourceRevisions } from "@/api/listSourceRevisions";
import { listStoryboardVersions } from "@/api/listStoryboardVersions";
import { renderEpisode } from "@/api/renderEpisode";

export interface BackendApiError {
  status: number | "FETCH_ERROR";
  data: unknown;
}

type GeneratedResult<T> = { data: T } | { error: BackendApiError };

function apiError(error: unknown): BackendApiError {
  if (
    typeof error === "object" &&
    error !== null &&
    "status" in error &&
    typeof error.status === "number" &&
    "problem" in error
  ) {
    return { status: error.status, data: error.problem };
  }
  return {
    status: "FETCH_ERROR",
    data: {
      code: "NETWORK_ERROR",
      detail: error instanceof Error ? error.message : "Unknown generated API error",
    },
  };
}

async function generated<T>(request: () => Promise<T>): Promise<GeneratedResult<T>> {
  try {
    return { data: await request() };
  } catch (error) {
    return { error: apiError(error) };
  }
}

const intentHeaders = (key: string) => ({ headers: { "Idempotency-Key": key } });
const versionHeaders = (version: number) => ({ headers: { "If-Match": `"${version}"` } });

export interface EpisodeIntent {
  episodeId: string;
  idempotencyKey: string;
}

export interface VersionIntent {
  versionId: string;
  resourceVersion: number;
}

export interface CreateProjectIntent {
  title: string;
  idempotencyKey: string;
}

export interface CreateSourceIntent extends EpisodeIntent {
  content: string;
  rightsBasis: "original" | "licensed";
  parentId: string | null;
}

export const backendApi = createApi({
  reducerPath: "backendApi",
  baseQuery: fakeBaseQuery<BackendApiError>(),
  tagTypes: ["Projects", "Story"],
  endpoints: (builder) => ({
    listProjects: builder.query<Awaited<ReturnType<typeof listProjects>>, void>({
      queryFn: () => generated(() => listProjects()),
      providesTags: ["Projects"],
    }),
    createProject: builder.mutation<Awaited<ReturnType<typeof createProject>>, CreateProjectIntent>({
      queryFn: ({ title, idempotencyKey }) =>
        generated(() => createProject({ title }, intentHeaders(idempotencyKey))),
      invalidatesTags: ["Projects"],
    }),
    listSourceRevisions: builder.query<
      Awaited<ReturnType<typeof listSourceRevisions>>,
      string
    >({
      queryFn: (episodeId) => generated(() => listSourceRevisions({ episode_id: episodeId })),
      providesTags: ["Story"],
    }),
    createSourceRevision: builder.mutation<
      Awaited<ReturnType<typeof createSourceRevision>>,
      CreateSourceIntent
    >({
      queryFn: ({ episodeId, idempotencyKey, content, rightsBasis, parentId }) =>
        generated(() =>
          createSourceRevision(
            { episode_id: episodeId },
            { content, rights_basis: rightsBasis, parent_id: parentId },
            intentHeaders(idempotencyKey),
          ),
        ),
      invalidatesTags: ["Projects", "Story"],
    }),
    confirmSource: builder.mutation<Awaited<ReturnType<typeof confirmSource>>, VersionIntent>({
      queryFn: ({ versionId, resourceVersion }) =>
        generated(() => confirmSource({ version_id: versionId }, versionHeaders(resourceVersion))),
      invalidatesTags: ["Projects", "Story"],
    }),
    listScriptVersions: builder.query<Awaited<ReturnType<typeof listScriptVersions>>, string>({
      queryFn: (episodeId) => generated(() => listScriptVersions({ episode_id: episodeId })),
      providesTags: ["Story"],
    }),
    generateScript: builder.mutation<Awaited<ReturnType<typeof generateScript>>, EpisodeIntent>({
      queryFn: ({ episodeId, idempotencyKey }) =>
        generated(() => generateScript({ episode_id: episodeId }, intentHeaders(idempotencyKey))),
      invalidatesTags: ["Story"],
    }),
    confirmScript: builder.mutation<Awaited<ReturnType<typeof confirmScript>>, VersionIntent>({
      queryFn: ({ versionId, resourceVersion }) =>
        generated(() => confirmScript({ version_id: versionId }, versionHeaders(resourceVersion))),
      invalidatesTags: ["Story"],
    }),
    listStoryboardVersions: builder.query<
      Awaited<ReturnType<typeof listStoryboardVersions>>,
      string
    >({
      queryFn: (episodeId) => generated(() => listStoryboardVersions({ episode_id: episodeId })),
      providesTags: ["Story"],
    }),
    generateStoryboard: builder.mutation<
      Awaited<ReturnType<typeof generateStoryboard>>,
      EpisodeIntent
    >({
      queryFn: ({ episodeId, idempotencyKey }) =>
        generated(() =>
          generateStoryboard({ episode_id: episodeId }, intentHeaders(idempotencyKey)),
        ),
      invalidatesTags: ["Story"],
    }),
    confirmStoryboard: builder.mutation<
      Awaited<ReturnType<typeof confirmStoryboard>>,
      VersionIntent
    >({
      queryFn: ({ versionId, resourceVersion }) =>
        generated(() =>
          confirmStoryboard({ version_id: versionId }, versionHeaders(resourceVersion)),
        ),
      invalidatesTags: ["Story"],
    }),
    renderEpisode: builder.mutation<Awaited<ReturnType<typeof renderEpisode>>, EpisodeIntent>({
      queryFn: ({ episodeId, idempotencyKey }) =>
        generated(() => renderEpisode({ episode_id: episodeId }, intentHeaders(idempotencyKey))),
    }),
  }),
});

export const {
  useConfirmScriptMutation,
  useConfirmSourceMutation,
  useConfirmStoryboardMutation,
  useCreateProjectMutation,
  useCreateSourceRevisionMutation,
  useGenerateScriptMutation,
  useGenerateStoryboardMutation,
  useListProjectsQuery,
  useListScriptVersionsQuery,
  useListSourceRevisionsQuery,
  useListStoryboardVersionsQuery,
  useRenderEpisodeMutation,
} = backendApi;
