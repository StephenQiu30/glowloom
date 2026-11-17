/** All management responses are synthetic; this fixture never contacts suppliers. */
import React from "react";
import { createRoot } from "react-dom/client";
import axios from "axios";
import { GatewayAdmin } from "../../src/components/gateway/gateway-admin";
import "../../src/app/globals.css";
axios.defaults.adapter = async (config) => {
  if (!config.url?.startsWith("/api/ai/admin/"))
    throw new Error("Unexpected fixture endpoint");
  return {
    config,
    status: 200,
    statusText: "OK",
    headers: {},
    data: { models: [], routes: [], credentials: [], quotas: [], unknown: [] },
  };
};
createRoot(document.getElementById("root")!).render(<GatewayAdmin />);
