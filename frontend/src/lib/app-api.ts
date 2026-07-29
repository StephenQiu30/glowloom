import { createApi, fakeBaseQuery } from "@reduxjs/toolkit/query/react";

import {
  loginApiV1AuthLoginPost,
  registerApiV1AuthRegisterPost,
} from "@/api/identity";
import { ApiClientError } from "@/lib/request";

export type AppApiError = {
  message: string;
  code: string;
  nextAction?: string;
};

async function runRequest<T>(
  request: () => Promise<{ data: T }>,
): Promise<{ data: T } | { error: AppApiError }> {
  try {
    const response = await request();
    return { data: response.data };
  } catch (error: unknown) {
    if (error instanceof ApiClientError) {
      return {
        error: {
          message: error.message,
          code: error.code,
          nextAction: error.nextAction,
        },
      };
    }
    return {
      error: { message: "服务暂时不可用，请稍后重试。", code: "request_failed" },
    };
  }
}

export const appApi = createApi({
  reducerPath: "appApi",
  baseQuery: fakeBaseQuery<AppApiError>(),
  endpoints: (builder) => ({
    login: builder.mutation<API.AuthResponse, API.LoginRequest>({
      queryFn: (body) => runRequest(() => loginApiV1AuthLoginPost(body)),
    }),
    register: builder.mutation<API.AuthResponse, API.RegisterRequest>({
      queryFn: (body) => runRequest(() => registerApiV1AuthRegisterPost(body)),
    }),
  }),
});

export const { useLoginMutation, useRegisterMutation } = appApi;
