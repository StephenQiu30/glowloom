import hashlib
import hmac
import secrets


def normalize_email(email: str) -> str:
    return email.strip().casefold()


def generate_registration_code() -> str:
    return f"{secrets.randbelow(1_000_000):06d}"


def generate_registration_ticket() -> str:
    return secrets.token_urlsafe(32)


def _hmac_digest(secret: str, value: str) -> str:
    return hmac.new(
        secret.encode("utf-8"),
        value.encode("utf-8"),
        hashlib.sha256,
    ).hexdigest()


def email_fingerprint(secret: str, email: str) -> str:
    return _hmac_digest(secret, f"registration-email:{email}")


def source_fingerprint(secret: str, source: str) -> str:
    return _hmac_digest(secret, f"registration-source:{source}")


def registration_code_digest(secret: str, email: str, code: str) -> str:
    return _hmac_digest(secret, f"registration-code:{email}:{code}")


def registration_ticket_digest(secret: str, ticket: str) -> str:
    return _hmac_digest(secret, f"registration-ticket:{ticket}")
