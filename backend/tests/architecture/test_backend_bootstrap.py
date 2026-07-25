from __future__ import annotations

import importlib
import subprocess
import sys

from fastapi import FastAPI


def test_bootstrap_modules_import_without_external_io() -> None:
    script = """
import socket

def blocked(*args, **kwargs):
    raise AssertionError("network access during import")

socket.socket.connect = blocked
socket.create_connection = blocked
import lanverse.bootstrap.api
import lanverse.entrypoints.api
import lanverse.entrypoints.worker
"""
    result = subprocess.run(
        [sys.executable, "-c", script],
        check=False,
        capture_output=True,
        text=True,
    )
    assert result.returncode == 0, result.stderr


def test_app_factory_returns_independent_fastapi_apps() -> None:
    module = importlib.import_module("lanverse.bootstrap.api")

    first = module.create_app()
    second = module.create_app()

    assert isinstance(first, FastAPI)
    assert isinstance(second, FastAPI)
    assert first is not second
    assert first.title == "Lanverse API"
    assert first.version == "0.1.0"


def test_console_entrypoints_are_callable() -> None:
    api = importlib.import_module("lanverse.entrypoints.api")
    worker = importlib.import_module("lanverse.entrypoints.worker")

    assert callable(api.main)
    assert callable(worker.main)
