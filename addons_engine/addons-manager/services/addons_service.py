import logging
import os
from datetime import datetime, timezone

import requests
from addon_status import AddonStatusEnum
from db import addons_db

MARKETPLACE_ADDR = os.environ.get("MARKETPLACE_ADDR") or "http://localhost:11102"
MARKETPLACE_API = f"{MARKETPLACE_ADDR}/api/v1/marketplace/addons"
MARKETPLACE_TIMEOUT_SECONDS = 10

addons_service = None


class MarketplaceAddonNotFoundError(Exception):
    pass


class MarketplaceUnavailableError(Exception):
    pass


def get_addon_in_marketplace(marketplace_id, check_is_verified=True):
    try:
        response = requests.get(
            f"{MARKETPLACE_API}/{marketplace_id}", timeout=MARKETPLACE_TIMEOUT_SECONDS
        )
        if response.status_code == 404:
            raise MarketplaceAddonNotFoundError(marketplace_id)
        response.raise_for_status()
        marketplace_addon = response.json()
    except requests.RequestException as e:
        # Connection errors, timeouts, other error codes and broken JSON all point at the
        # marketplace, not at a missing addon.
        raise MarketplaceUnavailableError(str(e)) from e

    if check_is_verified and marketplace_addon.get("status") != "approved":
        return None

    return marketplace_addon


def install_addon(addon):
    marketplace_id = addon.get("marketplace_id")
    marketplace_addon = get_addon_in_marketplace(marketplace_id, check_is_verified=False)

    if marketplace_addon is None:
        return None

    services = marketplace_addon.get("services", [])

    if not services:
        logging.error(f"Addon-{marketplace_id} has no services")
        return None

    now = datetime.now(timezone.utc)

    addon["name"] = marketplace_addon.get("name")
    addon["description"] = marketplace_addon.get("description", "")
    addon["services"] = services
    addon["volumes"] = marketplace_addon.get("volumes", [])
    addon["networks"] = marketplace_addon.get("networks", [])
    addon["ui"] = marketplace_addon.get("ui")
    addon["status"] = str(AddonStatusEnum.INSTALLING)
    addon["created_at"] = now
    addon["updated_at"] = now

    created_addon = addons_db.create_addon(addon)

    return created_addon
