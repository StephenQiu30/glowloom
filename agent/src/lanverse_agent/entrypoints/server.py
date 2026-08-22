from __future__ import annotations

import hashlib
import os
from datetime import datetime, timezone
from threading import Lock
from uuid import UUID, uuid4

from fastapi import Depends, FastAPI, Header, HTTPException, status
from pydantic import BaseModel, ConfigDict, Field


class RunRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    skill: str = Field(pattern="^script_analysis$")
    stage: str = Field(pattern="^(manifest|narrative|knowledge)$")
    request_hash: str = Field(pattern="^[a-f0-9]{64}$")
    snapshot_ref: str = Field(min_length=1)


class ProposalItem(BaseModel):
    item_id: str
    kind: str
    value: object
    evidence: list[dict[str, int]]


class AgentRun(BaseModel):
    run_id: UUID
    skill: str
    stage: str
    status: str
    request_hash: str
    items: list[ProposalItem] = Field(default_factory=list)
    created_at: datetime
    error: str | None = None


class Harness:
    """Private deterministic harness; backend remains the only fact writer."""

    def __init__(self) -> None:
        self._runs: dict[tuple[str, str], AgentRun] = {}
        self._lock = Lock()

    def start(self, request: RunRequest) -> AgentRun:
        key = (request.stage, request.request_hash)
        with self._lock:
            existing = self._runs.get(key)
            if existing is not None:
                return existing
            run = AgentRun(
                run_id=uuid4(),
                skill=request.skill,
                stage=request.stage,
                status="succeeded",
                request_hash=request.request_hash,
                items=[],
                created_at=datetime.now(timezone.utc),
            )
            self._runs[key] = run
            return run

    def get(self, run_id: UUID) -> AgentRun | None:
        with self._lock:
            return next((run for run in self._runs.values() if run.run_id == run_id), None)

    def cancel(self, run_id: UUID) -> AgentRun | None:
        with self._lock:
            run = next((item for item in self._runs.values() if item.run_id == run_id), None)
            if run is None:
                return None
            run.status = "cancelled"
            return run


def build_app(harness: Harness | None = None) -> FastAPI:
    app = FastAPI(title="Lanverse Private Agent", version="current", docs_url=None, redoc_url=None, openapi_url=None)
    runtime = harness or Harness()
    expected_token = os.getenv("LANVERSE_AGENT_TOKEN", "lanverse-agent-local")

    def authorize(token: str | None = Header(default=None, alias="X-Lanverse-Agent-Token")) -> None:
        if token != expected_token:
            raise HTTPException(status_code=status.HTTP_403_FORBIDDEN, detail="private agent identity rejected")

    @app.get("/readyz")
    def ready() -> dict[str, str]:
        return {"status": "ready"}

    @app.post("/internal/agent-runs", response_model=AgentRun, dependencies=[Depends(authorize)])
    def start_run(request: RunRequest) -> AgentRun:
        return runtime.start(request)

    @app.get("/internal/agent-runs/{run_id}", response_model=AgentRun, dependencies=[Depends(authorize)])
    def get_run(run_id: UUID) -> AgentRun:
        run = runtime.get(run_id)
        if run is None:
            raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="agent run not found")
        return run

    @app.post("/internal/agent-runs/{run_id}/cancel", response_model=AgentRun, dependencies=[Depends(authorize)])
    def cancel_run(run_id: UUID) -> AgentRun:
        run = runtime.cancel(run_id)
        if run is None:
            raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="agent run not found")
        return run

    return app


app = build_app()


def canonical_request_hash(value: str) -> str:
    return hashlib.sha256(value.encode("utf-8")).hexdigest()


if __name__ == "__main__":
    import uvicorn

    uvicorn.run(app, host="127.0.0.1", port=int(os.getenv("AGENT_PORT", "8790")))
