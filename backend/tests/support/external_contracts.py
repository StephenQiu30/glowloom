import ipaddress
import os
from urllib.parse import urlsplit

DEFAULT_KAFKA_CONTRACT_BOOTSTRAP_SERVERS = "127.0.0.1:9092"


def require_local_kafka_bootstrap_servers(value: str) -> str:
    servers = [item.strip() for item in value.split(",") if item.strip()]
    if not servers:
        raise ValueError("Kafka contract bootstrap servers are required")
    for server in servers:
        parsed = urlsplit(f"//{server}")
        if parsed.hostname is None or parsed.port is None:
            raise ValueError("Kafka contract bootstrap server must include host and port")
        if parsed.hostname == "localhost":
            continue
        try:
            address = ipaddress.ip_address(parsed.hostname)
        except ValueError as error:
            raise ValueError("Kafka contracts may connect only to a loopback broker") from error
        if not address.is_loopback:
            raise ValueError("Kafka contracts may connect only to a loopback broker")
    return ",".join(servers)


def kafka_contract_bootstrap_servers() -> str:
    return require_local_kafka_bootstrap_servers(
        os.getenv("KAFKA_CONTRACT_BOOTSTRAP_SERVERS")
        or os.getenv("KAFKA_BOOTSTRAP_SERVERS")
        or DEFAULT_KAFKA_CONTRACT_BOOTSTRAP_SERVERS
    )
