from __future__ import annotations

import argparse
import json


def health() -> dict[str, str]:
    return {"service": "scheduler", "status": "ok"}


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--healthcheck", action="store_true")
    arguments = parser.parse_args(argv)
    if arguments.healthcheck:
        print(json.dumps(health()))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
