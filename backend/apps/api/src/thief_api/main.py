from fastapi import FastAPI


app = FastAPI(title="Thief API", version="0.1.0")


@app.get("/health/live")
async def live() -> dict[str, str]:
    return {"service": "api", "status": "ok"}


@app.get("/health/ready")
async def ready() -> dict[str, str]:
    return {"service": "api", "status": "ready"}
