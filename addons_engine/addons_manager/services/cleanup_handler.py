from db import addons_db
from services.addons_runner import stop_addons


def handle_shutdown():
    addons = addons_db.find_active_addons()
    stop_addons(addons)
