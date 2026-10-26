from enum import Enum


class HookEvent(str, Enum):
    POST_CREATE = "post_create"
    POST_DELETE = "post_delete"
    POST_UPDATE = "post_update"
    PRE_CREATE = "pre_create"
    PRE_DELETE = "pre_delete"
    PRE_UPDATE = "pre_update"

    def __str__(self) -> str:
        return str(self.value)
