"use client";

import { createApi, fakeBaseQuery } from "@reduxjs/toolkit/query/react";

import { listProjects as listProjectsRequest } from "@/api/listProjects";
import { renderEpisode as renderEpisodeRequest } from "@/api/renderEpisode";

type GeneratedResult<T> = { data: T } | { error: unknown };

async function generated<T>(request: () => Promise<T>): Promise<GeneratedResult<T>> {
  try {
    return { data: await request() };
  } catch (error) {
    return { error };
  }
}

export interface RenderEpisodeArguments {
  episodeId: string;
  idempotencyKey: string;
}

export const backendApi = createApi({
  reducerPath: "backendApi",
  baseQuery: fakeBaseQuery<unknown>(),
  endpoints: (builder) => ({
    listProjects: builder.query<Awaited<ReturnType<typeof listProjectsRequest>>, void>({
      queryFn: () => generated(() => listProjectsRequest()),
    }),
    renderEpisode: builder.mutation<
      Awaited<ReturnType<typeof renderEpisodeRequest>>,
      RenderEpisodeArguments
    >({
      queryFn: ({ episodeId, idempotencyKey }) =>
        generated(() =>
          renderEpisodeRequest(
            { episode_id: episodeId },
            { headers: { "Idempotency-Key": idempotencyKey } },
          ),
        ),
    }),
  }),
});

export const { useListProjectsQuery, useRenderEpisodeMutation } = backendApi;
