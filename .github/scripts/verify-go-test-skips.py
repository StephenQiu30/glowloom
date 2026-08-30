#!/usr/bin/env python3

import json
import sys
from pathlib import Path


ALLOWED_TEST_SKIPS = {
    ("tests/generation", "TestProviderTemporalRestartWorkerHelper"),
    ("tests/workflow", "TestFormalGenerationTemporalWorkerProcessHelper"),
    ("tests/workflow", "TestReferenceAssetTemporalWorkerProcessHelper"),
    ("tests/workflow", "TestTemporalWorkerProcessHelper"),
    ("tests/workflow", "TestWorkflowWorkerProcessHelper"),
}


def backend_package(package: str) -> str:
    marker = "/backend/"
    if marker not in package:
        return package
    return package.split(marker, 1)[1]


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: verify-go-test-skips.py <go-test-json>", file=sys.stderr)
        return 2

    report = Path(sys.argv[1])
    allowed = set()
    unexpected = set()
    with report.open(encoding="utf-8") as stream:
        for line_number, line in enumerate(stream, start=1):
            if not line.strip():
                continue
            try:
                event = json.loads(line)
            except json.JSONDecodeError as error:
                print(f"invalid go test JSON at line {line_number}: {error}", file=sys.stderr)
                return 1
            if event.get("Action") != "skip" or not event.get("Test"):
                continue
            skipped = (backend_package(str(event.get("Package", ""))), str(event["Test"]))
            if skipped in ALLOWED_TEST_SKIPS:
                allowed.add(skipped)
            else:
                unexpected.add(skipped)

    if unexpected:
        for package, test in sorted(unexpected):
            print(f"unexpected skipped Go test: {package} {test}", file=sys.stderr)
        return 1

    print(f"Go test skip gate passed: {len(allowed)} subprocess helper skip(s), 0 unexpected")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
