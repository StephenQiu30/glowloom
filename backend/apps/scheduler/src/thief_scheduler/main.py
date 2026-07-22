from __future__ import annotations

import json


def health() -> dict[str, str]:
    return {"service": "scheduler", "status": "ok"}


def main() -> None:
    print(json.dumps(health()))


if __name__ == "__main__":
    main()
