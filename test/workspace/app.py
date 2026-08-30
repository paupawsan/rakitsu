"""Sample application for testing neuro-cli agent file analysis."""

import json
from dataclasses import dataclass


@dataclass
class User:
    name: str
    email: str
    age: int

    def greet(self) -> str:
        return f"Hello, {self.name}!"

    def to_dict(self) -> dict:
        return {"name": self.name, "email": self.email, "age": self.age}


def load_users(filepath: str) -> list[User]:
    with open(filepath) as f:
        data = json.load(f)
    return [User(**item) for item in data]


def find_adults(users: list[User]) -> list[User]:
    return [u for u in users if u.age >= 18]


if __name__ == "__main__":
    users = [
        User("Alice", "alice@example.com", 30),
        User("Bob", "bob@example.com", 17),
        User("Charlie", "charlie@example.com", 25),
    ]
    adults = find_adults(users)
    for user in adults:
        print(user.greet())
