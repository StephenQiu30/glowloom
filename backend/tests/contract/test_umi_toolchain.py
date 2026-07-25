from __future__ import annotations

import json
import os
import shutil
import subprocess
from pathlib import Path
from uuid import UUID

from fastapi import FastAPI
from pydantic import BaseModel, ConfigDict

from lanverse.shared_kernel.http_contracts import Problem, TaskAccepted, TaskResponse

ROOT = Path(__file__).resolve().parents[3]
FRONTEND = ROOT / "frontend"
CONFIG = FRONTEND / "openapi2ts.config.ts"
REQUEST = FRONTEND / "src" / "lib" / "request.ts"


class CreatedResource(BaseModel):
    model_config = ConfigDict(extra="forbid")
    id: UUID


def representative_openapi() -> dict[str, object]:
    app = FastAPI(title="Contract Toolchain Fixture", version="1.0.0")

    @app.post(
        "/fixture/resources",
        operation_id="createFixtureResource",
        status_code=201,
        response_model=CreatedResource,
        responses={
            201: {"headers": {"ETag": {"schema": {"type": "string"}}}},
            409: {"model": Problem, "description": "Conflict"},
        },
    )
    async def create_resource() -> CreatedResource:
        raise NotImplementedError

    @app.post(
        "/fixture/tasks",
        operation_id="startFixtureTask",
        status_code=202,
        response_model=TaskAccepted,
        responses={422: {"model": Problem, "description": "Invalid request"}},
    )
    async def start_task() -> TaskAccepted:
        raise NotImplementedError

    @app.get(
        "/fixture/tasks/{task_id}",
        operation_id="getFixtureTask",
        response_model=TaskResponse,
        responses={404: {"model": Problem, "description": "Not found"}},
    )
    async def get_task(task_id: UUID) -> TaskResponse:
        raise NotImplementedError

    return app.openapi()


def node24(*arguments: str, env: dict[str, str] | None = None) -> subprocess.CompletedProcess[str]:
    pnpm = shutil.which("pnpm")
    assert pnpm
    return subprocess.run(
        [pnpm, "dlx", "node@24.18.0", *arguments],
        cwd=FRONTEND,
        env={**os.environ, **(env or {})},
        check=False,
        capture_output=True,
        text=True,
    )


def test_committed_generator_config_uses_the_unique_contract_chain() -> None:
    assert CONFIG.is_file()
    text = CONFIG.read_text()
    assert "../backend/openapi/openapi.json" in text
    assert "./src/services/generated" in text
    assert "@/lib/request" in text
    assert "nullable: true" in text
    assert "enumStyle: \"string-literal\"" in text
    assert REQUEST.is_file()


def test_openapi_31_generates_typecheckable_umi_services(tmp_path: Path) -> None:
    schema = tmp_path / "fixture.json"
    output = tmp_path / "generated"
    schema.write_text(json.dumps(representative_openapi()))
    generator = """
import { generateService } from '@umijs/openapi';
await generateService({
  schemaPath: process.env.SCHEMA,
  serversPath: process.env.OUTPUT,
  namespace: 'API',
  nullable: true,
  enumStyle: 'string-literal',
  requestOptionsType: 'RequestOptions',
  requestImportStatement: "import { request, type RequestOptions } from '@/lib/request';",
});
"""
    generated = node24(
        "--input-type=module",
        "--eval",
        generator,
        env={"SCHEMA": str(schema), "OUTPUT": str(output)},
    )
    assert generated.returncode == 0, generated.stderr
    assert list(output.rglob("*.ts"))

    tsconfig = tmp_path / "tsconfig.json"
    tsconfig.write_text(
        json.dumps(
            {
                "extends": str(FRONTEND / "tsconfig.json"),
                "compilerOptions": {
                    "noEmit": True,
                    "types": ["node"],
                    "typeRoots": [str(FRONTEND / "node_modules" / "@types")],
                },
                "include": [str(output / "**/*.ts"), str(REQUEST)],
            }
        )
    )
    compiler = FRONTEND / "node_modules" / "typescript" / "bin" / "tsc"
    checked = node24(str(compiler), "--project", str(tsconfig))
    assert checked.returncode == 0, checked.stdout + checked.stderr
