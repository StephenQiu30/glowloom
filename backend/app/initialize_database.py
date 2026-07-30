import asyncio

from app.core.database import engine, initialize_empty_database
from app.modules.governance import models as governance_models
from app.modules.identity import models as identity_models
from app.modules.media import models as media_models
from app.modules.messaging import models as messaging_models
from app.modules.production import models as production_models
from app.modules.projects import models as project_models
from app.modules.scripts import models as script_models

# Importing implemented slice models registers their tables on Base.metadata.
_ = (
    identity_models,
    governance_models,
    messaging_models,
    media_models,
    production_models,
    project_models,
    script_models,
)


async def main() -> None:
    await initialize_empty_database()
    await engine.dispose()


if __name__ == "__main__":
    asyncio.run(main())
