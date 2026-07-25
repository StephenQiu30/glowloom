from __future__ import annotations

from dataclasses import dataclass

from lanverse.shared_kernel.clock import Clock, SystemClock
from lanverse.shared_kernel.config import ApplicationSettings


@dataclass(frozen=True, slots=True)
class ApplicationContainer:
    settings: ApplicationSettings
    clock: Clock


def create_container(settings: ApplicationSettings) -> ApplicationContainer:
    return ApplicationContainer(settings=settings, clock=SystemClock())
