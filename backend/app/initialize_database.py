import asyncio

from app.core.database import engine, initialize_empty_database
from app.modules.identity import models as identity_models
from app.modules.projects import models as project_models

# Importing the accepted slice models registers their tables on Base.metadata.
_ = (identity_models, project_models)


async def main() -> None:
    await initialize_empty_database()
    await engine.dispose()


if __name__ == "__main__":
    asyncio.run(main())
