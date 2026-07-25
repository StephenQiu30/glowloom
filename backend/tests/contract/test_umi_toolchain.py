from __future__ import annotations

import json
import os
import shutil
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
FRONTEND = ROOT / "frontend"
CONFIG = FRONTEND / "openapi2ts.config.ts"
REQUEST = FRONTEND / "src" / "lib" / "request.ts"


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
    assert "LANVERSE_OPENAPI_URL" in text
    assert "http://127.0.0.1:8000/openapi.json" in text
    assert "backend/openapi" not in text
    assert "./src/services/generated" in text
    assert "@/lib/request" in text
    assert "nullable: true" in text
    assert "enumStyle: \"string-literal\"" in text
    assert REQUEST.is_file()


def test_openapi_31_url_generates_typecheckable_umi_services(
    tmp_path: Path, live_openapi_url: str
) -> None:
    output = tmp_path / "generated"
    generated = node24(
        str(FRONTEND / "scripts" / "generate-openapi.mjs"),
        env={
            "LANVERSE_OPENAPI_URL": live_openapi_url,
            "LANVERSE_OPENAPI_OUTPUT": str(output),
        },
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
