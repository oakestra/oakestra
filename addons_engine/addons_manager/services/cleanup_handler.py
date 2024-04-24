from services.addons_runner import stop_addons


def handle_shutdown():
    stop_addons()
