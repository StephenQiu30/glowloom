import pytest

from tests.support.external_contracts import require_local_kafka_bootstrap_servers


@pytest.mark.parametrize(
    "servers",
    (
        "",
        "kafka.example.com:9092",
        "10.0.0.8:9092",
        "127.0.0.1",
    ),
)
def test_kafka_contract_rejects_missing_or_non_loopback_brokers(servers: str) -> None:
    with pytest.raises(ValueError, match="Kafka contract|Kafka contracts"):
        require_local_kafka_bootstrap_servers(servers)


def test_kafka_contract_accepts_loopback_brokers() -> None:
    assert (
        require_local_kafka_bootstrap_servers("127.0.0.1:9092, localhost:9093")
        == "127.0.0.1:9092,localhost:9093"
    )
