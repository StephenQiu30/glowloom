"use client";

import { useAccount } from "../account-provider";
import { createAccess } from "../lib/access";

export function useAccess() {
  return createAccess(useAccount());
}
