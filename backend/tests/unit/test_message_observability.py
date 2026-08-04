from prometheus_client import generate_latest

from app.modules.messaging.metrics import (
    message_event_type_label,
    observe_message_result,
    observe_outbox_publish_result,
    queue_label_for_routing_key,
)


def test_message_metric_labels_are_registered_and_unknown_values_are_bounded() -> None:
    hostile_event_type = "attacker-controlled-event-9f6297b8"

    assert message_event_type_label("script_extraction.requested") == (
        "script_extraction.requested"
    )
    assert message_event_type_label(hostile_event_type) == "unregistered"
    assert queue_label_for_routing_key("io.script.extract") == "lanverse.io"
    assert queue_label_for_routing_key("media.probe") == "lanverse.media"
    assert queue_label_for_routing_key("attacker.route") == "unregistered"

    observe_message_result(
        queue="lanverse.io",
        event_type=hostile_event_type,
        result="rejected",
        duration_seconds=0.002,
    )
    observe_outbox_publish_result(
        routing_key="io.script.extract",
        event_type="script_extraction.requested",
        result="published",
        duration_seconds=0.003,
    )

    rendered = generate_latest().decode("utf-8")
    assert (
        'lanverse_message_results_total{event_type="unregistered",'
        'queue="lanverse.io",result="rejected"}'
    ) in rendered
    assert hostile_event_type not in rendered
    assert (
        'lanverse_outbox_publish_results_total{event_type="script_extraction.requested",'
        'queue="lanverse.io",result="published"}'
    ) in rendered
