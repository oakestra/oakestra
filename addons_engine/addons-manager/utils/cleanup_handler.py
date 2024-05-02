from services import addons_service


def handle_shutdown():
    addons_service.stop_all_addons()
