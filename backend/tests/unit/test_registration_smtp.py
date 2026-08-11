import smtplib
from email.message import EmailMessage
from typing import Any

import pytest
from pydantic import SecretStr

from app.modules.identity.registration_verifications.smtp import (
    RegistrationMailUnavailableError,
    SMTPRegistrationMailer,
)


class RecordingSMTP:
    def __init__(self, host: str, port: int, *, timeout: float) -> None:
        self.host = host
        self.port = port
        self.timeout = timeout
        self.started_tls = False
        self.logged_in: tuple[str, str] | None = None
        self.messages: list[EmailMessage] = []

    def __enter__(self) -> "RecordingSMTP":
        return self

    def __exit__(self, *args: object) -> None:
        return None

    def ehlo(self) -> tuple[int, bytes]:
        return 250, b"ok"

    def starttls(self, *, context: object) -> tuple[int, bytes]:
        assert context is not None
        self.started_tls = True
        return 220, b"ready"

    def login(self, username: str, password: str) -> tuple[int, bytes]:
        self.logged_in = (username, password)
        return 235, b"ok"

    def send_message(self, message: EmailMessage) -> dict[str, Any]:
        self.messages.append(message)
        return {}


@pytest.mark.asyncio
async def test_smtp_mailer_sends_plain_and_html_registration_code(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    smtp = RecordingSMTP("smtp.example.com", 465, timeout=3)

    def smtp_ssl_factory(*args: object, **kwargs: object) -> RecordingSMTP:
        return smtp

    monkeypatch.setattr(smtplib, "SMTP_SSL", smtp_ssl_factory)
    mailer = SMTPRegistrationMailer(
        enabled=True,
        host="smtp.example.com",
        port=465,
        tls_mode="tls",
        username="mailer@example.com",
        password=SecretStr("smtp-test-secret"),
        from_email="mailer@example.com",
        from_name="Lanverse",
        timeout_seconds=3,
    )

    await mailer.send_registration_code(
        email="creator@example.com",
        code="123456",
        expires_minutes=10,
    )

    assert smtp.logged_in == ("mailer@example.com", "smtp-test-secret")
    assert len(smtp.messages) == 1
    message = smtp.messages[0]
    assert message["To"] == "creator@example.com"
    assert message["Subject"] == "Lanverse 注册验证码"
    plain = message.get_body(preferencelist=("plain",))
    html = message.get_body(preferencelist=("html",))
    assert plain is not None and "123456" in plain.get_content()
    assert html is not None and "10 分钟" in html.get_content()


@pytest.mark.asyncio
async def test_smtp_mailer_uses_starttls_and_sanitizes_provider_failures(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    smtp = RecordingSMTP("smtp.example.com", 587, timeout=3)

    def fail_send(message: EmailMessage) -> dict[str, Any]:
        raise smtplib.SMTPAuthenticationError(535, b"provider-secret-detail")

    smtp.send_message = fail_send

    def smtp_factory(*args: object, **kwargs: object) -> RecordingSMTP:
        return smtp

    monkeypatch.setattr(smtplib, "SMTP", smtp_factory)
    mailer = SMTPRegistrationMailer(
        enabled=True,
        host="smtp.example.com",
        port=587,
        tls_mode="starttls",
        username="mailer@example.com",
        password=SecretStr("smtp-test-secret"),
        from_email="mailer@example.com",
        from_name="Lanverse",
        timeout_seconds=3,
    )

    with pytest.raises(RegistrationMailUnavailableError) as captured:
        await mailer.send_registration_code(
            email="creator@example.com",
            code="123456",
            expires_minutes=10,
        )

    assert smtp.started_tls
    assert str(captured.value) == "registration mail dependency is unavailable"
    assert "provider-secret-detail" not in str(captured.value)
