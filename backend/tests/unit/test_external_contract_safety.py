import pytest

from tests.support.external_contracts import require_isolated_rabbitmq_url


@pytest.mark.parametrize(
    "url",
    (
        "amqp://guest:guest@127.0.0.1:5672/",
        "amqp://guest:guest@127.0.0.1:5672/%2F",
        "amqp://guest:guest@127.0.0.1:5672",
    ),
)
def test_rabbitmq_contract_rejects_the_default_vhost(url: str) -> None:
    with pytest.raises(ValueError, match="isolated RabbitMQ vhost"):
        require_isolated_rabbitmq_url(url)


def test_rabbitmq_contract_accepts_a_named_vhost() -> None:
    assert (
        require_isolated_rabbitmq_url(
            "amqp://guest:guest@127.0.0.1:5672/lanverse_contract"
        )
        == "amqp://guest:guest@127.0.0.1:5672/lanverse_contract"
    )
