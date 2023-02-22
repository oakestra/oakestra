import ipaddress
import requests
from flask import request
import pandas as pd
import time
import numpy as np
import logging
from decorators.singleton import singleton


@singleton
class Geolite():
    instance = None

    def __init__(self):
        logging.debug("Start building GeoLite2 dataframe...")
        self.df = None
        start = time.time()
        chunk = pd.read_csv('geolocation/geoip2-ipv4.csv', chunksize=500000,
                            usecols=['network', 'latitude', 'longitude'],
                            dtype={'network': 'str', 'latitude': np.float64, 'longitude': np.float64})
        self.df = pd.concat(chunk)
        end = time.time()
        logging.debug(f"...done building the dataframe. Took {end - start}s")

    def query_geolocation_for_ips(self, ip_addresses):

        ip_addresses = [ipaddress.ip_address(ip) for ip in ip_addresses]
        ip_locations = {}
        for ip in ip_addresses:
            logging.info(f"Lookup: {ip}")
            # If IP Adress is private just return artificial coordinates contained in url params or 0 if no params were given
            if ip.is_private:
                lat = request.args.get("lat") or 0
                long = request.args.get("long") or 0
                return {'lat': lat, 'long': long}

            # Get first byte of IP
            first_byte = str(ip).split(".")[0]

            # In case the first byte is not contained in the GeoLite2 database, keep decrementing the first byte and check if it exists
            start_idx = 0
            for i in range(int(first_byte), -1, -1):
                indices = self.df[self.df.network.str.startswith(f"{i}.")].index
                if len(indices) >= 1:
                    start_idx = indices[0]
                    logging.info(f"Start lookup at index {start_idx}/{self.df['network'].size} with first byte {i}")
                    break

            # Start at start_idx to reduce number of iterations
            for i in range(start_idx, self.df['network'].size):
                ip_network = ipaddress.ip_network(self.df.at[i, 'network'])
                if ip in ip_network:
                    lat = self.df.at[i, 'latitude']
                    long = self.df.at[i, 'longitude']
                    ip_locations[str(ip)] = {"lat": lat, "long": long}
                    logging.info(f"Found coords: {lat},{long} for IP {str(ip)}")
                    # Stop lookup when IP was found to avoid long running process
                    break
        return ip_locations

    def user_in_cluster(self, user, cluster):
        """
        Checks whether the 'user' is located within the cluster or its boundaries. Since shapely is coordinate-agnostic it
        will handle geographic coordinates expressed in latitudes and longitudes exactly the same way as coordinates on a
        Cartesian plane. But on a sphere the behavior is different and angles are not constant along a geodesic.
        For that reason we do a small distance correction here.
        """
        return True if cluster.intersects(user) or user.within(cluster) or cluster.distance(user) < 1e-5 else False
