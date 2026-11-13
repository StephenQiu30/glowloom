import React from "react";
import { createRoot } from "react-dom/client";
import { ModelRoutePool } from "../../src/app/models/_components/model-route-pool";
import { SalePricePreview } from "../../src/app/models/_components/sale-price-preview";
import { setCSRFToken } from "../../src/lib/request";
import "../../src/app/globals.css";

setCSRFToken("isolated-fixture");
createRoot(document.getElementById("root")!).render(
  <main>
    <ModelRoutePool
      releaseID={new URLSearchParams(location.search).get("id")!}
    />
    <form>
      <label>
        采购单价
        <input name="base_rate" defaultValue="7.56" />
      </label>
      <label>
        倍率
        <input name="markup_multiplier" defaultValue="1.5" />
      </label>
      <label>
        折扣
        <input name="discount_tenths" defaultValue="8" />
      </label>
      <SalePricePreview unit="per_request" currency="CNY" />
    </form>
  </main>,
);
