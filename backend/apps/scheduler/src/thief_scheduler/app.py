from celery import Celery  # type: ignore[import-untyped]

from thief_scheduler.settings import SchedulerSettings


settings = SchedulerSettings.from_env()
app = Celery("thief-scheduler", broker=settings.rabbitmq_url)
