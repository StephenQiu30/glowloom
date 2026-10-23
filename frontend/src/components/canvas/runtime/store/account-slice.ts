import { createSlice, type PayloadAction } from "@reduxjs/toolkit";

type AccountState = {
  account: CanvasContract.AccountVO | null;
  ready: boolean;
  error?: string;
  signingOut: boolean;
};

const initialState: AccountState = {
  account: null,
  ready: false,
  signingOut: false,
};

export const accountSlice = createSlice({
  name: "account",
  initialState,
  reducers: {
    accepted: (
      state,
      action: PayloadAction<CanvasContract.AccountVO | null>,
    ) => {
      state.account = action.payload;
      state.ready = true;
      state.error = undefined;
    },
    failed: (state, action: PayloadAction<string>) => {
      state.ready = true;
      state.error = action.payload;
    },
    signingOut: (state, action: PayloadAction<boolean>) => {
      state.signingOut = action.payload;
    },
  },
});

export const accountActions = accountSlice.actions;
export function accountScope(account: CanvasContract.AccountVO | null) {
  return `${account?.id ?? "guest"}:${[...(account?.actions ?? [])].sort().join(",")}`;
}
