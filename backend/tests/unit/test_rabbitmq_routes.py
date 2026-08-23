from app.integrations.rabbitmq import ALLOWED_ROUTING_KEYS


def test_storyboard_draft_route_is_registered_for_outbox_delivery() -> None:
    assert "io.storyboard.draft" in ALLOWED_ROUTING_KEYS
