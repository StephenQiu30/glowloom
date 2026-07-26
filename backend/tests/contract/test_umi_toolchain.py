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
MAKEFILE = ROOT / "Makefile"


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
    assert 'path.resolve(process.cwd(), "./src")' in text
    assert "src/services" not in text
    assert "@/lib/request" in text
    assert "nullable: true" in text
    assert "enumStyle: \"string-literal\"" in text
    assert REQUEST.is_file()
    makefile = MAKEFILE.read_text()
    assert "frontend/src/api" in makefile
    assert "frontend/src/services" not in makefile


def test_openapi_31_url_generates_typecheckable_umi_services(
    tmp_path: Path, live_openapi_url: str
) -> None:
    output = tmp_path / "src"
    output.mkdir()
    handwritten = output / "store.ts"
    handwritten.write_text("export const handwritten = true;\n")
    api_root = output / "api"
    api_root.mkdir()
    stale = api_root / "stale.ts"
    stale.write_text("export const stale = true;\n")
    generated = node24(
        str(FRONTEND / "scripts" / "generate-openapi.mjs"),
        env={
            "LANVERSE_OPENAPI_URL": live_openapi_url,
            "LANVERSE_OPENAPI_OUTPUT": str(output),
        },
    )
    assert generated.returncode == 0, generated.stderr
    assert handwritten.read_text() == "export const handwritten = true;\n"
    assert not stale.exists()
    assert {path.name for path in output.iterdir() if path.is_dir()} == {"api"}
    assert not [path for path in api_root.iterdir() if path.is_dir()]
    generated_files = list(api_root.glob("*.ts"))
    assert generated_files

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
                "include": [str(api_root / "*.ts"), str(REQUEST)],
            }
        )
    )
    compiler = FRONTEND / "node_modules" / "typescript" / "bin" / "tsc"
    checked = node24(str(compiler), "--project", str(tsconfig))
    assert checked.returncode == 0, checked.stdout + checked.stderr
