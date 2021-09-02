import csv
import ipaddress
import requests
from flask import request


def query_geolocation_for_ip(ip_address):
    # If IP Adress is private just return artificial coordinates contained in url params or 0 if no params were given
    if ipaddress.ip_address(ip_address).is_private:
        lat = request.args.get("lat") or 0
        long = request.args.get("long") or 0
        return lat, long

    # Geolite2 index:column names
    # 0:network, 1:geoname_id, 2:registered_country_geoname_id, 3:represented_country_geoname_id, 4:is_anonymous_proxy,
    # 5:is_satellite_provider, 6:postal_code, 7:latitude, 8:longitude, 9:accuracy_radius
    geolite2 = csv.reader(open('GeoLite2-City-Blocks-IPv4.csv'), delimiter=",")

    for row in geolite2:
        ip_network = ipaddress.ip_network(row[0])
        if ip_address in ip_network:
            # lat, long
            return row[7], row[8]

    raise EOFError(f"IP Address {ip_address} not in geolite2 database file.")

def geolocate_ip_via_api():
    url = f"http://ip-api.com/json/{request.remote_addr}"
    resp = requests.get(url)
    data = resp.json()
    lat = data.get("lat")
    long = data.get("lon")

    return lat, long


def user_in_cluster(user, cluster):
    """
    Checks whether the 'user' is located within the cluster or its boundaries. Since shapely is coordinate-agnostic it
    will handle geographic coordinates expressed in latitudes and longitudes exactly the same way as coordinates on a
    Cartesian plane. But on a sphere the behavior is different and angles are not constant along a geodesic.
    For that reason we do a small distance correction here.
    """
    return True if cluster.intersects(user) or user.within(cluster) or cluster.distance(user) < 1e-5 else False
