from __future__ import annotations

import io

from minio import Minio
from minio.error import S3Error
from urllib3.exceptions import HTTPError

from integrations.object_storage import ObjectStoreUnavailable, RemoteObject


class MinioTransport:
    def __init__(self, client: Minio) -> None:
        self._client = client

    @classmethod
    def from_credentials(
        cls,
        *,
        endpoint: str,
        access_key: str,
        secret_key: str,
        secure: bool,
    ) -> MinioTransport:
        client = Minio(
            endpoint,
            access_key=access_key,
            secret_key=secret_key,
            secure=secure,
        )
        return cls(client)

    def stat(self, bucket: str, object_key: str) -> RemoteObject | None:
        try:
            item = self._client.stat_object(bucket, object_key)
        except S3Error as error:
            if error.code in {"NoSuchKey", "NoSuchObject", "NoSuchVersion"}:
                return None
            raise ObjectStoreUnavailable("MinIO stat failed") from error
        except (HTTPError, OSError) as error:
            raise ObjectStoreUnavailable("MinIO stat failed") from error
        if item.size is None:
            raise ObjectStoreUnavailable("MinIO returned an object without size")
        metadata = {
            str(key).lower(): str(value) for key, value in (item.metadata or {}).items()
        }
        sha256 = metadata.get("x-amz-meta-sha256", metadata.get("sha256", ""))
        content_type = item.content_type or metadata.get("content-type", "")
        return RemoteObject(item.size, sha256, content_type)

    def put(
        self,
        bucket: str,
        object_key: str,
        data: bytes,
        content_type: str,
        sha256: str,
    ) -> None:
        try:
            self._client.put_object(
                bucket,
                object_key,
                io.BytesIO(data),
                len(data),
                content_type=content_type,
                metadata={"sha256": sha256},
            )
        except (S3Error, HTTPError, OSError) as error:
            raise ObjectStoreUnavailable("MinIO put failed") from error
