from sqlmodel import SQLModel, create_engine, Session
from app.config import settings

# Create engine with pool size and overflow for production readiness
engine = create_engine(
    settings.DATABASE_URL,
    echo=settings.DEBUG,
    future=True,
    pool_size=20,
    max_overflow=10,
)

# Dependency to provide a Session instance per request
def get_session() -> Session:
    with Session(engine) as session:
        yield session