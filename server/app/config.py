from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    app_name: str = "群策"
    app_version: str = "0.1.0"
    default_pair_token: str = "dev-pair-token"

    model_config = SettingsConfigDict(
        env_prefix="QUNCE_",
        env_file=".env",
        extra="ignore",
    )


settings = Settings()
