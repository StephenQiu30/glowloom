def register_implemented_models() -> None:
    """Import every implemented model module at application composition boundaries."""
    from app.modules.assets import models as assets_models
    from app.modules.governance import models as governance_models
    from app.modules.governance.audit import models as governance_audit_models
    from app.modules.identity import models as identity_models
    from app.modules.media import models as media_models
    from app.modules.messaging import models as messaging_models
    from app.modules.production import models as production_models
    from app.modules.projects import models as project_models
    from app.modules.scripts import models as script_models
    from app.modules.storyboards import models as storyboard_models

    _ = (
        assets_models,
        governance_audit_models,
        governance_models,
        identity_models,
        media_models,
        messaging_models,
        production_models,
        project_models,
        script_models,
        storyboard_models,
    )
