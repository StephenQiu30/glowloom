from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

from lanverse.bootstrap.api import create_app
from lanverse.shared_kernel.config import ApplicationSettings

BACKEND = Path(__file__).resolve().parents[1]
DEFAULT_OUTPUT = BACKEND / "openapi" / "openapi.json"


def rendered_openapi() -> bytes:
    app = create_app(ApplicationSettings(environment="test", docs_enabled=True))
    document = app.openapi()
    return (json.dumps(document, ensure_ascii=False, indent=2, sort_keys=True) + "\n").encode()


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Export the Lanverse FastAPI OpenAPI contract")
    parser.add_argument("--output", type=Path, default=DEFAULT_OUTPUT)
    parser.add_argument("--check", action="store_true")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    expected = rendered_openapi()
    if args.check:
        if not args.output.is_file() or args.output.read_bytes() != expected:
            print("OpenAPI artifact is out of date", file=sys.stderr)
            return 1
        return 0

    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_bytes(expected)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
