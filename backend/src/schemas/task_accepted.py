from schemas.common import TaskAccepted
from schemas.tasks import TaskAcceptedSnapshot


def task_accepted(value: TaskAcceptedSnapshot) -> TaskAccepted:
    return TaskAccepted(
        task_id=value.task_id,
        status="queued",
        resource_version=value.resource_version,
        status_url=value.status_url,
    )
