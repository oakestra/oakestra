import os

def get_coordinates():
    use_gps = os.environ.get("GPS")
    if use_gps:
        return gps_info()
    else:
        # get static coordinates from env vars
        lat = os.environ.get("LAT")
        long = os.environ.get("LONG")
    if lat is None and long is None:
        gps_info()

    return float(lat), float(long)


def gps_info():
    # Return mock gps data specified in "gps_mock.txt"
    try:
        with open("gps_mock.txt") as f:
            coords = f.readlines()[0]
    except Exception:
        print("No GPS info available.")
        return None
    lat, long = coords.split(",")

    return float(lat), float(long)