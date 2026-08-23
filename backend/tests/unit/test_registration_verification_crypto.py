import re

from app.modules.identity.registration_verifications.crypto import (
    email_fingerprint,
    generate_registration_code,
    generate_registration_ticket,
    registration_code_digest,
    registration_ticket_digest,
)


def test_registration_secrets_are_random_and_bound_to_email_and_purpose() -> None:
    secret = "lanverse-registration-test-secret-with-32-bytes"
    code = generate_registration_code()
    another_code = generate_registration_code()
    ticket = generate_registration_ticket()

    assert re.fullmatch(r"\d{6}", code)
    assert re.fullmatch(r"\d{6}", another_code)
    assert code != another_code
    assert len(ticket) >= 43
    assert registration_code_digest(secret, "creator@example.com", code) != code
    assert registration_code_digest(secret, "creator@example.com", code) != (
        registration_code_digest(secret, "other@example.com", code)
    )
    assert email_fingerprint(secret, "creator@example.com") != "creator@example.com"
    assert registration_ticket_digest(secret, ticket) != ticket
