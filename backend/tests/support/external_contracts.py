import os
from urllib.parse import unquote, urlparse

DEFAULT_RABBITMQ_CONTRACT_URL = (
    "amqp://guest:guest@127.0.0.1:5672/lanverse_contract"
)


def require_isolated_rabbitmq_url(url: str) -> str:
    vhost = unquote(urlparse(url).path).strip("/")
    if not vhost:
        raise ValueError(
            "real messaging contracts require an isolated RabbitMQ vhost"
        )
    return url


def rabbitmq_contract_url() -> str:
    return require_isolated_rabbitmq_url(
        os.getenv("RABBITMQ_CONTRACT_URL")
        or os.getenv("RABBITMQ_URL")
        or DEFAULT_RABBITMQ_CONTRACT_URL
    )
