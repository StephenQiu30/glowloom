import asyncio
import smtplib
import ssl
from email.message import EmailMessage
from email.utils import formataddr
from typing import Literal

from pydantic import SecretStr

from app.modules.identity.registration_verifications.contracts import (
    RegistrationMailUnavailableError,
)

__all__ = ["RegistrationMailUnavailableError", "SMTPRegistrationMailer"]


class SMTPRegistrationMailer:
    def __init__(
        self,
        *,
        enabled: bool,
        host: str | None,
        port: int,
        tls_mode: Literal["tls", "starttls"],
        username: str | None,
        password: SecretStr | None,
        from_email: str | None,
        from_name: str,
        timeout_seconds: float,
    ) -> None:
        self._enabled = enabled
        self._host = host
        self._port = port
        self._tls_mode = tls_mode
        self._username = username
        self._password = password
        self._from_email = from_email
        self._from_name = from_name
        self._timeout_seconds = timeout_seconds

    async def send_registration_code(
        self,
        *,
        email: str,
        code: str,
        expires_minutes: int,
    ) -> None:
        try:
            await asyncio.wait_for(
                asyncio.to_thread(
                    self._send_registration_code,
                    email,
                    code,
                    expires_minutes,
                ),
                timeout=self._timeout_seconds + 1,
            )
        except RegistrationMailUnavailableError:
            raise
        except (smtplib.SMTPException, OSError, TimeoutError, ssl.SSLError) as error:
            raise RegistrationMailUnavailableError(
                "registration mail dependency is unavailable"
            ) from error

    def _send_registration_code(
        self,
        email: str,
        code: str,
        expires_minutes: int,
    ) -> None:
        if not self._enabled or self._host is None or self._from_email is None:
            raise RegistrationMailUnavailableError(
                "registration mail dependency is unavailable"
            )
        message = self._message(email, code, expires_minutes)
        context = ssl.create_default_context()
        if self._tls_mode == "tls":
            with smtplib.SMTP_SSL(
                self._host,
                self._port,
                timeout=self._timeout_seconds,
                context=context,
            ) as client:
                self._authenticate(client)
                client.send_message(message)
            return
        with smtplib.SMTP(
            self._host,
            self._port,
            timeout=self._timeout_seconds,
        ) as client:
            client.ehlo()
            client.starttls(context=context)
            client.ehlo()
            self._authenticate(client)
            client.send_message(message)

    def _authenticate(self, client: smtplib.SMTP) -> None:
        if self._username is None and self._password is None:
            return
        if self._username is None or self._password is None:
            raise RegistrationMailUnavailableError(
                "registration mail dependency is unavailable"
            )
        client.login(self._username, self._password.get_secret_value())

    def _message(self, email: str, code: str, expires_minutes: int) -> EmailMessage:
        message = EmailMessage()
        message["Subject"] = "Lanverse 注册验证码"
        message["From"] = formataddr((self._from_name, self._from_email or ""))
        message["To"] = email
        message.set_content(
            "\n".join(
                (
                    "你好，",
                    "",
                    f"你的 Lanverse 注册验证码是：{code}",
                    f"验证码将在 {expires_minutes} 分钟后失效。",
                    "如果不是你本人操作，请忽略这封邮件。",
                )
            )
        )
        message.add_alternative(
            (
                "<html><body>"
                "<p>你好，</p>"
                f"<p>你的 Lanverse 注册验证码是：<strong>{code}</strong></p>"
                f"<p>验证码将在 {expires_minutes} 分钟后失效。</p>"
                "<p>如果不是你本人操作，请忽略这封邮件。</p>"
                "</body></html>"
            ),
            subtype="html",
        )
        return message
