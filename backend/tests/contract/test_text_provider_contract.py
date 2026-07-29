import json
import os

import httpx
import pytest
from pydantic import BaseModel, ConfigDict, Field, ValidationError

DASHSCOPE_CHAT_COMPLETIONS_URL = (
    "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions"
)
TEXT_PROVIDER_MODEL = "qwen3.7-plus-2026-05-26"


class ExtractionContract(BaseModel):
    model_config = ConfigDict(extra="forbid")

    character_count: int = Field(ge=0)
    scene_count: int = Field(ge=0)


class ProviderUsage(BaseModel):
    prompt_tokens: int = Field(gt=0)
    completion_tokens: int = Field(gt=0)


class ProviderMessage(BaseModel):
    content: str


class ProviderChoice(BaseModel):
    message: ProviderMessage


class ProviderResponse(BaseModel):
    model: str = Field(min_length=1)
    choices: list[ProviderChoice] = Field(min_length=1, max_length=1)
    usage: ProviderUsage
    request_id: str | None = None
    id: str | None = None


def require_api_key() -> str:
    if os.getenv("LANVERSE_RUN_TEXT_PROVIDER_CONTRACT") != "1":
        pytest.skip(
            "run make contract-text-provider after configuring DASHSCOPE_API_KEY"
        )
    api_key = os.getenv("DASHSCOPE_API_KEY")
    if not api_key:
        pytest.fail("DASHSCOPE_API_KEY is required for the explicit provider contract")
    return api_key


def sanitized_provider_evidence(body: ProviderResponse, request_id: str) -> str:
    return json.dumps(
        {
            "provider_request_id": request_id,
            "model": body.model,
            "prompt_tokens": body.usage.prompt_tokens,
            "completion_tokens": body.usage.completion_tokens,
            "structured_output_valid": True,
        },
        ensure_ascii=False,
        sort_keys=True,
    )


@pytest.mark.asyncio
async def test_dashscope_structured_extraction_contract() -> None:
    api_key = require_api_key()
    request_payload = {
        "model": TEXT_PROVIDER_MODEL,
        "messages": [
            {
                "role": "system",
                "content": "Return one JSON object only. Do not add prose or Markdown.",
            },
            {
                "role": "user",
                "content": (
                    "Extract counts from this synthetic fixture: one fictional character "
                    "named Lin is in one studio scene. Return exactly the keys "
                    "character_count and scene_count."
                ),
            },
        ],
        "enable_thinking": False,
        "temperature": 0.1,
        "top_p": 0.8,
        "max_tokens": 200,
        "response_format": {"type": "json_object"},
    }

    try:
        async with httpx.AsyncClient(timeout=60) as client:
            response = await client.post(
                DASHSCOPE_CHAT_COMPLETIONS_URL,
                headers={"Authorization": f"Bearer {api_key}"},
                json=request_payload,
            )
    except httpx.RequestError as error:
        pytest.fail(f"provider request failed before HTTP response: {type(error).__name__}")

    request_id = response.headers.get("x-request-id", "")
    if response.status_code != 200:
        pytest.fail(
            f"provider returned HTTP {response.status_code}; request_id={request_id or 'missing'}"
        )
    try:
        raw_body: object = response.json()
    except ValueError:
        pytest.fail("provider returned a non-JSON response")
    try:
        body = ProviderResponse.model_validate(raw_body)
    except ValidationError:
        pytest.fail("provider JSON did not match the expected response contract")
    assert body.model == TEXT_PROVIDER_MODEL

    if not request_id:
        request_id = body.request_id or body.id or ""
    assert request_id, "provider response must include a request identifier"

    try:
        extraction = ExtractionContract.model_validate_json(body.choices[0].message.content)
    except ValidationError:
        pytest.fail("provider content did not match the structured extraction contract")
    assert extraction == ExtractionContract(character_count=1, scene_count=1)

    print(sanitized_provider_evidence(body, request_id))
