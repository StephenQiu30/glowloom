"use client";

import { useState } from "react";
import useSWR from "swr";
import { getProviderCheck, testAdminCredential } from "@/api/settings";
import { useSession } from "@/components/auth/session";
import { useCatalogMutation } from "./mutation";

export function useProviderCatalogProbe(provider: string, scope: string) {
  const { actor } = useSession();
  const [checkID, setCheckID] = useState("");
  const mutation = useCatalogMutation(
    (result) => setCheckID((result as API.CredentialTestAccepted).check_id!),
    scope,
    (value, target) => {
      const result = value as API.CredentialTestAccepted | undefined;
      return (
        result?.accepted === true &&
        result.provider_id === target &&
        !!result.check_id
      );
    },
  );
  const { data, error } = useSWR(
    checkID ? ["provider-catalog-probe", actor.id, checkID] : null,
    () => getProviderCheck({ id: checkID }),
    {
      errorRetryCount: 2,
      refreshInterval: (check) =>
        !check?.status ||
        check.status === "running" ||
        check.status === "pending"
          ? 1200
          : 0,
    },
  );
  const running =
    !!checkID &&
    !error &&
    (!data || data.status === "running" || data.status === "pending");
  function run(credentialID: string) {
    setCheckID("");
    return mutation.run("credential.test", provider, (requestID) =>
      testAdminCredential(
        { id: provider, credential_id: credentialID },
        { headers: { "Idempotency-Key": requestID } },
      ),
    );
  }
  return { data, error, running, mutation, run };
}
