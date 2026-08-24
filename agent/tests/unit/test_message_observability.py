from prometheus_client import generate_latest

from app.modules.messaging.metrics import (
    message_event_type_label,
    observe_message_result,
    observe_outbox_publish_result,
    topic_label,
)


def test_message_metric_labels_are_registered_and_unknown_values_are_bounded() -> None:
    hostile_event_type = "attacker-controlled-event-9f6297b8"

    assert message_event_type_label("script_extraction.requested") == (
        "script_extraction.requested"
    )
    assert message_event_type_label("production_bible.requested") == ("production_bible.requested")
    assert message_event_type_label(hostile_event_type) == "unregistered"
    assert topic_label("lanverse.io.v1") == "lanverse.io.v1"
    assert topic_label("lanverse.media.v1") == "lanverse.media.v1"
    assert topic_label("attacker.route") == "unregistered"

    observe_message_result(
        topic="lanverse.io.v1",
        event_type=hostile_event_type,
        result="rejected",
        duration_seconds=0.002,
    )
    observe_outbox_publish_result(
        topic="lanverse.io.v1",
        event_type="script_extraction.requested",
        result="published",
        duration_seconds=0.003,
    )

    rendered = generate_latest().decode("utf-8")
    assert (
        'lanverse_message_results_total{event_type="unregistered",'
        'result="rejected",topic="lanverse.io.v1"}'
    ) in rendered
    assert hostile_event_type not in rendered
    assert (
        'lanverse_outbox_publish_results_total{event_type="script_extraction.requested",'
        'result="published",topic="lanverse.io.v1"}'
    ) in rendered
