from pydantic import BaseModel

from app.modules.production.contracts import TaskResponse

__all__ = [
    "PaginatedTasks",
    "TaskResponse",
]


class PaginatedTasks(BaseModel):
    items: list[TaskResponse]
    total: int
    limit: int
    offset: int
