import json
from pathlib import Path

from app.main import app

OUTPUT_PATH = Path(__file__).resolve().parents[1] / "openapi.json"


def main() -> None:
    schema = app.openapi()
    OUTPUT_PATH.write_text(
        json.dumps(schema, ensure_ascii=False, indent=2, sort_keys=True) + "\n",
        encoding="utf-8",
    )


if __name__ == "__main__":
    main()
