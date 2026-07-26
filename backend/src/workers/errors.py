from __future__ import annotations

from datetime import timedelta


class RetryableJobError(RuntimeError):
    def __init__(
        self,
        code: str,
        *,
        retry_after: timedelta = timedelta(seconds=1),
    ) -> None:
        super().__init__(code)
        self.code = code
        self.retry_after = retry_after
