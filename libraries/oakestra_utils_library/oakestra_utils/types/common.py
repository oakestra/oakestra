import enum


class CustomEnum(enum.Enum):
    def __str__(self) -> str:
        return self.value
