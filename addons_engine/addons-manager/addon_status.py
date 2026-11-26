from enum import Enum


# Not in services/ because addons_service imports addons_db, and addons_db needs this enum too.
# TODO(ME): reuse this enum in addons_monitor
class AddonStatusEnum(Enum):
    INSTALLING = "installing"
    DISABLING = "disabling"

    DISABLED = "disabled"
    FAILED = "failed"
    ACTIVE = "active"
    PARTIALLY_ACTIVE = "partially_active"

    def __str__(self):
        return self.value
