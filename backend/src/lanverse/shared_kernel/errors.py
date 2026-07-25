class ApplicationError(Exception):
    """Base class for expected application failures."""


class ConfigurationError(ApplicationError):
    """Raised when runtime configuration cannot satisfy an invariant."""
