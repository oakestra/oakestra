import logging
import re

from db import addons_db
from flask_smorest import Blueprint
from marshmallow import Schema, fields

uiaddonsblp = Blueprint("UI Addons Api", "ui_addons_api", url_prefix="/api/v1/ui-addons")

# Same pattern as the dashboard. The name ends up in the addonui-<name> host and in the
# /addon-assets/<name>/ path.
ADDON_NAME_PATTERN = re.compile(r"^[a-z0-9-]+$")
DIGEST_PREFIX_LENGTH = 12


class UiAddonEntrySchema(Schema):
    name = fields.String(required=True)
    version = fields.String(required=True)
    sdk = fields.String(required=True)


def image_version(image_ref: str) -> str:
    name, _, digest = image_ref.partition("@")

    # A colon before the last slash is a registry port (localhost:5000/ui), not a tag.
    _, colon, tag = name.rsplit("/", 1)[-1].rpartition(":")
    if colon and tag:
        return tag

    if digest:
        algorithm, _, hex_digest = digest.partition(":")
        return f"{algorithm}:{hex_digest[:DIGEST_PREFIX_LENGTH]}"

    return "latest"


def _ui_addon_entry(addon):
    name = addon.get("name")
    ui = addon.get("ui") or {}

    if not isinstance(name, str) or not ADDON_NAME_PATTERN.match(name):
        logging.warning(f"Skipping UI addon {addon.get('_id')}: invalid name {name!r}")
        return None

    service = next(
        (s for s in addon.get("services", []) if s.get("service_name") == ui.get("service")),
        None,
    )
    if service is None or not service.get("image"):
        logging.warning(
            f"Skipping UI addon {name}: ui.service {ui.get('service')!r} "
            "is not one of its services or has no image"
        )
        return None

    # The marketplace requires sdk now, but older or hand-edited records may not have it.
    sdk = ui.get("sdk")
    if not isinstance(sdk, str) or not sdk:
        logging.warning(f"Skipping UI addon {name}: missing ui.sdk")
        return None

    return {"name": name, "version": image_version(service["image"]), "sdk": sdk}


@uiaddonsblp.route("/")
@uiaddonsblp.response(200, UiAddonEntrySchema(many=True))
def list_ui_addons():
    # No auth on purpose, the dashboard calls this without a token.
    # Names have to be unique since the dashboard builds the asset host addonui-<name> from them.
    entries = {}
    for addon in addons_db.find_active_addons(with_ui=True):
        entry = _ui_addon_entry(addon)
        if entry is None:
            continue
        if entry["name"] in entries:
            logging.warning(f"Skipping duplicate UI addon {entry['name']}")
            continue
        entries[entry["name"]] = entry

    return list(entries.values())
