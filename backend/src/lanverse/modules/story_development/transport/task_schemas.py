from lanverse.modules.production_jobs.public import TaskAcceptedSnapshot
from lanverse.shared_kernel.http_contracts import TaskAccepted


def task_accepted(value: TaskAcceptedSnapshot) -> TaskAccepted:
    return TaskAccepted(
        task_id=value.task_id,
        status="queued",
        resource_version=value.resource_version,
        status_url=value.status_url,
    )
