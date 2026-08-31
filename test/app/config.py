from pydantic_settings import BaseSettings, SettingsConfigDict
from typing import Literal

class Settings(BaseSettings):
    # Core
    APP_NAME: str = "FastAPI E‑Commerce"
    DEBUG: bool = False
    
    # Database
    # Fixture-only placeholder — override via .env for anything real.
    DATABASE_URL: str = "sqlite:///./test_app.db"

    # Security
    # Fixture-only placeholder, not a real secret — override via .env.
    SECRET_KEY: str = "fixture-placeholder-not-a-real-secret"
    ALGORITHM: Literal["HS256"] = "HS256"
    ACCESS_TOKEN_EXPIRE_MINUTES: int = 30
    REFRESH_TOKEN_EXPIRE_DAYS: int = 7

    # JWT
    # Fixture-only placeholder, not a real secret — override via .env.
    # Could be same as SECRET_KEY but separated for clarity.
    JWT_SECRET_KEY: str = "fixture-placeholder-not-a-real-secret"
    
    # Logging
    LOG_LEVEL: Literal["DEBUG", "INFO", "WARNING", "ERROR"] = "INFO"
    
    model_config = SettingsConfigDict(env_file=".env", env_file_encoding="utf-8")

settings = Settings()