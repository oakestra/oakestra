import os


def get_coordinates():
    # Check whether static coordinates are defined
    lat = os.environ.get("LAT")
    long = os.environ.get("LONG")
    print(lat, long)
    # If no coordinates are defined in start-up.sh check if device has GPS module
    if lat is None and long is None:
        gps_info()

    return lat, long


def gps_info():
    # TODO: figure out how to check if device ahs gps module
    # For now just return mock values
    lat = 48.2
    long = 11.2

    return lat, long