from __future__ import annotations

import asyncio
import os
import socket
import sys
import time
from collections.abc import AsyncGenerator
from contextlib import asynccontextmanager
from pathlib import Path
from uuid import uuid4

import httpx
import pytest
from temporalio.client import Client, WorkflowExecutionStatus
from temporalio.service import RPCError, RPCStatusCode

from app.creation.contract import Command
from app.creation.repository import Repository
from app.creation.temporal import TemporalStarter, WorkflowIdentityConflict
from tests.creation.test_contract import SECRET, authorization
from tests.creation.test_repository import new_command

AGENT_ROOT = Path(__file__).resolve().parents[2]


def native_temporal_address() -> str:
    address = os.getenv("LANVERSE_TEST_TEMPORAL_ADDRESS")
    if not address:
        pytest.skip("requires the existing native Temporal service")
    return address


@asynccontextmanager
async def agent_process(repository: Repository, address: str, queue: str) -> AsyncGenerator[str]:
    # A short-lived application test process; no new infrastructure or native-service restart.
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        listener.listen()
        port = listener.getsockname()[1]
        env = {
            key: value
            for key, value in os.environ.items()
            if key in {"PATH", "LANG", "LC_ALL", "SYSTEMROOT", "TMPDIR"}
        }
        env.update(
            CREATION_DATABASE_URL=repository.dsn,
            CREATION_AGENT_SECRET=SECRET,
            AGENT_EXECUTION_SECRET="native-agent-execution-secret-with-32-bytes",
            CREATION_HARNESS_SECRET="native-agent-harness-secret-with-32-bytes",
            CREATION_PLATFORM_URL="http://127.0.0.1:8686",
            CREATION_HARNESS_URL="http://127.0.0.1:8787",
            CREATION_TEXT_RELEASE_HASH="352d46c51661e7d989b42ddeb0a0ff0a4b48165e8e3f7700f3e60d170e4c58cb",
            CREATION_TEMPORAL_ADDRESS=address,
            CREATION_TASK_QUEUE=queue,
        )
        process = await asyncio.create_subprocess_exec(
            sys.executable,
            "-m",
            "uvicorn",
            "app.main:create_app",
            "--factory",
            "--fd",
            str(listener.fileno()),
            "--no-access-log",
            "--log-level",
            "error",
            cwd=AGENT_ROOT,
            env=env,
            pass_fds=(listener.fileno(),),
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.STDOUT,
        )
    url = f"http://127.0.0.1:{port}"
    try:
        async with httpx.AsyncClient(timeout=1, trust_env=False) as client:
            async with asyncio.timeout(15):
                while True:
                    if process.returncode is not None:
                        output, _ = await process.communicate()
                        pytest.fail(
                            "creation test process exited before readiness:\n"
                            + output.decode(errors="replace")
                        )
                    try:
                        # Uvicorn serves only after the repository, dispatcher, and Temporal
                        # worker lifespan has started. Harness readiness additionally requires
                        # Codex, which this command-handoff job intentionally does not install.
                        if (await client.get(url + "/healthz")).status_code == 200:
                            break
                    except httpx.HTTPError:
                        pass
                    await asyncio.sleep(0.05)
        yield url
    finally:
        if process.returncode is None:
            process.terminate()
            try:
                async with asyncio.timeout(10):
                    await process.communicate()
            except TimeoutError:
                process.kill()
                await process.communicate()


def headers(command: Command, *, lookup: bool = False) -> dict[str, str]:
    path = "/internal/creation/commands"
    body = command.model_dump_json(by_alias=True).encode()
    if lookup:
        path += "/" + command.command_id
        body = b""
    return {
        "X-Lanverse-Creation-Authorization": authorization(
            body,
            path,
            "GET" if lookup else "POST",
            int(time.time()) + 60,
        )
    }


async def test_real_agent_process_restart_preserves_receipt_and_temporal_identity(
    repository: Repository,
) -> None:
    address = native_temporal_address()
    async with asyncio.timeout(10):
        temporal = await Client.connect(address)
    command = new_command()
    queue = "creation-native-test-" + uuid4().hex
    handle = temporal.get_workflow_handle(command.workflow_id)
    try:
        async with (
            agent_process(repository, address, queue) as url,
            httpx.AsyncClient(timeout=5, trust_env=False) as http,
        ):
            go_env = {
                key: value
                for key, value in os.environ.items()
                if key in {"PATH", "HOME", "LANG", "LC_ALL", "TMPDIR", "GOCACHE", "GOPATH"}
            }
            go_env.update(
                LANVERSE_TEST_CREATION_URL=url,
                LANVERSE_TEST_CREATION_SECRET=SECRET,
                LANVERSE_TEST_CREATION_COMMAND=command.model_dump_json(by_alias=True),
            )
            go_test = await asyncio.create_subprocess_exec(
                "go",
                "test",
                "-count=1",
                "-race",
                "./tests/production/creation",
                "-run",
                "^TestNativeAgentCommandAcceptance$",
                cwd=AGENT_ROOT.parent / "backend",
                env=go_env,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.STDOUT,
            )
            try:
                async with asyncio.timeout(60):
                    output, _ = await go_test.communicate()
                assert go_test.returncode == 0, output.decode()
            finally:
                if go_test.returncode is None:
                    go_test.kill()
                    await go_test.wait()
            response = await http.post(
                url + "/internal/creation/commands",
                content=command.model_dump_json(by_alias=True),
                headers=headers(command),
            )
            assert response.status_code == 202
            receipt = response.json()
            async with asyncio.timeout(20):
                while True:
                    async with await repository.connect() as conn:
                        row = await (
                            await conn.execute(
                                "SELECT state, temporal_run_id FROM creation_start_outbox "
                                "WHERE command_id = %s",
                                (command.command_id,),
                            )
                        ).fetchone()
                    if row and row["state"] == "started":
                        break
                    await asyncio.sleep(0.1)
            description = await handle.describe()
            temporal_run_id = description.run_id
            assert str(row["temporal_run_id"]) == temporal_run_id
            assert await description.memo_value("creation_payload_hash") == command.payload_hash
            assert description.task_queue == queue
        async with (
            agent_process(repository, address, queue) as url,
            httpx.AsyncClient(timeout=5, trust_env=False) as http,
        ):
            lookup = await http.get(
                url + "/internal/creation/commands/" + command.command_id,
                headers=headers(command, lookup=True),
            )
            assert lookup.status_code == 200 and lookup.json() == receipt
            repeated = await http.post(
                url + "/internal/creation/commands",
                content=command.model_dump_json(by_alias=True),
                headers=headers(command),
            )
            assert repeated.status_code == 202 and repeated.json() == receipt
            assert (await handle.describe()).run_id == temporal_run_id
        # Closed history still resolves to the original execution; it is not started again.
        await handle.terminate(reason="synthetic creation acceptance test cleanup")
        assert await TemporalStarter(temporal).ensure_started(command, queue) == temporal_run_id
    finally:
        try:
            if (await handle.describe()).status == WorkflowExecutionStatus.RUNNING:
                await handle.terminate(reason="synthetic creation acceptance test cleanup")
        except RPCError as error:
            if error.status != RPCStatusCode.NOT_FOUND:
                raise


async def test_native_temporal_rejects_history_with_conflicting_identity() -> None:
    async with asyncio.timeout(10):
        client = await Client.connect(native_temporal_address())
    command = new_command()
    queue = "creation-native-test-" + uuid4().hex
    handle = await client.start_workflow(
        command.flow_type,
        {},
        id=command.workflow_id,
        task_queue=queue,
        memo={"creation_payload_hash": "wrong"},
    )
    try:
        with pytest.raises(WorkflowIdentityConflict):
            await TemporalStarter(client).ensure_started(command, queue)
    finally:
        await handle.terminate(reason="synthetic creation collision test cleanup")
