import numpy as np
import math
from shapely.geometry import asPoint
from shapely.geometry import asLineString
from shapely.geometry import asPolygon
from shapely.ops import transform
from functools import partial
import pyproj
from shapely.geometry import Point, LineString, Polygon, MultiPolygon
from shapely.ops import unary_union
from sklearn.cluster import DBSCAN
import alphashape

def get_node_coords(nodes):
    lat_long_coords = []
    for n in nodes:
        lat = float(n.get('lat'))
        long = float(n.get('long'))
        lat_long_coords.append((lat, long))

    return lat_long_coords

def create_geo_of_node_locations(nodes):
    lat_long_coords = get_node_coords(nodes)
    # Number of nodes = 1 -> Point
    if len(lat_long_coords) == 1:
        return Point(*lat_long_coords)
    # Number of nodes = 2 -> LineString
    elif len(lat_long_coords) == 2:
        return LineString(lat_long_coords)
    # Number of nodes >= 3 -> Polygon
    elif len(lat_long_coords) >= 3:
        return Polygon(lat_long_coords)


def create_obfuscated_polygons_based_in_alphashapes(coords, max_dist, buffer_size):
    clusters = cluster_worker_nodes(coords, max_dist)

    polygons = []
    for cluster in clusters:
        # Create alpha shape
        alpha_shape = alphashape.alphashape(cluster, 0.5)
        # Add buffer to obfuscate node locations
        buffered_shape = alpha_shape.buffer(buffer_size)
        polygons.append(buffered_shape)

    return unary_union(polygons)


def create_obfuscated_polygons_based_on_concave_hull(coords, max_dist=200, buffer_size=500):
    node_clusters = cluster_worker_nodes(coords, max_dist)
    print(f"Node Clusters: {len(node_clusters)}")
    if len(node_clusters) == 0:
        return None

    polygons = []
    for cluster in node_clusters:
        concave_hull = ConcaveHull(cluster)
        hull_array = concave_hull.calculate()
        # Cluster consists of single point -> Point
        if len(hull_array) == 1:
            geo = Point(*hull_array)
        # Cluster consists of two points -> LineString
        elif len(hull_array) == 2:
            geo = LineString(hull_array)
        # Clusters consists of >= 3 points -> Polygon
        else:
            geo = Polygon(hull_array)

        # Obfuscate nodes located on concave hull by adding buffer to geometric object
        buffered_geo = buffer_in_meters(geo, buffer_size)
        polygons.append(buffered_geo)

    return unary_union(polygons)

def cluster_worker_nodes(lat_long_coords, max_dist):
    """
    lat_long_coords: latitude and longitude coordinates
    epsilon: max distance that points can be from each other to be considered a cluster.

    Cluster the given latitude, longitude coordinates.
    Set min_samples to 1 so that every data point gets assigned to either a cluster or forms its own cluster.
    Nothing will be classified as noise.
    Use haversince metric and ball tree algorithm to calculate great circle distances between points.
    epsilon and coordiantes get converted to radians, because scikit-learn's haversine metric needs radian units.

    return: clustered coordianates:
    """
    if len(lat_long_coords) == 0:
        return []
    # print(lat_long_coords)
    kms_per_radian = 6371.0088
    epsilon = max_dist / kms_per_radian
    clustering = DBSCAN(eps=epsilon, min_samples=1, algorithm='ball_tree', metric='haversine').fit(
        np.radians(lat_long_coords))
    labels = clustering.labels_

    points_w_cluster = np.concatenate((lat_long_coords, labels[:, np.newaxis]), axis=1)
    clusters = []
    for label in set(labels):
        clusters.append(points_w_cluster[points_w_cluster[:, 2] == label][:, 0:2])
    # print(f"Clusters: {clusters}")
    return clusters

def buffer_in_meters(hull, meters):
    # Shapely knows nothing about the units. Therefore, when calling the buffer(x) method, shapely will buffer
    # the coordinates by x units. To buffer in meters, we first need to reproject the polygon into a Coordinate
    # Reference System (CRS) that uses meters.
    proj_meters = pyproj.Proj(init='epsg:3857')
    proj_latlng = pyproj.Proj(init='epsg:4326')

    project_to_meters = partial(pyproj.transform, proj_latlng, proj_meters)
    project_to_latlng = partial(pyproj.transform, proj_meters, proj_latlng)

    hull_meters = transform(project_to_meters, hull)

    buffer_meters = hull_meters.buffer(meters)
    buffer_latlng = transform(project_to_latlng, buffer_meters)
    return buffer_latlng

# Code base from https://github.com/joaofig/uk-accidents/blob/master/geomath/hulls.py
class ConcaveHull(object):

    def __init__(self, points, prime_ix=0):
        if isinstance(points, np.core.ndarray):
            self.data_set = points
        elif isinstance(points, list):
            self.data_set = np.array(points)
        else:
            raise ValueError('Please provide an [N,2] numpy array or a list of lists.')

        # Clean up duplicates
        self.data_set = np.unique(self.data_set, axis=0)

        # Create the initial index
        self.indices = np.ones(self.data_set.shape[0], dtype=bool)

        self.prime_k = np.array([3, 5, 7, 11, 13, 17, 21, 23, 29, 31, 37, 41, 43,
                                 47, 53, 59, 61, 67, 71, 73, 79, 83, 89, 97])
        self.prime_ix = prime_ix

    def get_next_k(self):
        if self.prime_ix < len(self.prime_k):
            return self.prime_k[self.prime_ix]
        else:
            return -1

    def haversine_distance(self, loc_ini, loc_end):
        lon1, lat1, lon2, lat2 = map(np.radians,
                                     [loc_ini[0], loc_ini[1],
                                      loc_end[:, 0], loc_end[:, 1]])

        delta_lon = lon2 - lon1
        delta_lat = lat2 - lat1

        a = np.square(np.sin(delta_lat / 2.0)) + np.cos(lat1) * np.cos(lat2) * np.square(np.sin(delta_lon / 2.0))
        c = 2 * np.arctan2(np.sqrt(a), np.sqrt(1.0 - a))
        meters = 6371000.0 * c
        return meters

    @staticmethod
    def get_lowest_latitude_index(points):
        indices = np.argsort(points[:, 1])
        return indices[0]

    def get_k_nearest(self, ix, k):
        """
        Calculates the k nearest point indices to the point indexed by ix
        :param ix: Index of the starting point
        :param k: Number of neighbors to consider
        :return: Array of indices into the data set array
        """
        ixs = self.indices

        base_indices = np.arange(len(ixs))[ixs]
        distances = self.haversine_distance(self.data_set[ix, :], self.data_set[ixs, :])
        sorted_indices = np.argsort(distances)

        kk = min(k, len(sorted_indices))
        k_nearest = sorted_indices[range(kk)]
        return base_indices[k_nearest]

    def calculate_headings(self, ix, ixs, ref_heading=0.0):
        """
        Calculates the headings from a source point to a set of target points.
        :param ix: Index to the source point in the data set
        :param ixs: Indexes to the target points in the data set
        :param ref_heading: Reference heading measured in degrees counterclockwise from North
        :return: Array of headings in degrees with the same size as ixs
        """
        if ref_heading < 0 or ref_heading >= 360.0:
            raise ValueError('The reference heading must be in the range [0, 360)')

        r_ix = np.radians(self.data_set[ix, :])
        r_ixs = np.radians(self.data_set[ixs, :])

        delta_lons = r_ixs[:, 0] - r_ix[0]
        y = np.multiply(np.sin(delta_lons), np.cos(r_ixs[:, 1]))
        x = math.cos(r_ix[1]) * np.sin(r_ixs[:, 1]) - \
            math.sin(r_ix[1]) * np.multiply(np.cos(r_ixs[:, 1]), np.cos(delta_lons))
        bearings = (np.degrees(np.arctan2(y, x)) + 360.0) % 360.0 - ref_heading
        bearings[bearings < 0.0] += 360.0
        return bearings

    def recurse_calculate(self):
        """
        Calculates the concave hull using the next value for k while reusing the distances dictionary
        :return: Concave hull
        """
        recurse = ConcaveHull(self.data_set, self.prime_ix + 1)
        next_k = recurse.get_next_k()
        if next_k == -1:
            return None
        # print("k={0}".format(next_k))
        return recurse.calculate(next_k)

    def calculate(self, k=3):
        """
        Calculates the concave hull of the data set as an array of points
        :param k: Number of nearest neighbors
        :return: Array of points (N, 2) with the concave hull of the data set
        """
        # For point, line, triangle immediately return the coords
        if self.data_set.shape[0] <= 3:
            return self.data_set

        # Make sure that k neighbors can be found
        kk = min(k, self.data_set.shape[0])

        first_point = self.get_lowest_latitude_index(self.data_set)
        current_point = first_point

        # Note that hull and test_hull are matrices (N, 2)
        hull = np.reshape(np.array(self.data_set[first_point, :]), (1, 2))
        test_hull = hull

        # Remove the first point
        self.indices[first_point] = False

        prev_angle = 270    # Initial reference id due west. North is zero, measured clockwise.
        step = 2
        stop = 2 + kk

        while ((current_point != first_point) or (step == 2)) and len(self.indices[self.indices]) > 0:
            if step == stop:
                self.indices[first_point] = True

            knn = self.get_k_nearest(current_point, kk)

            # Calculates the headings between first_point and the knn points
            # Returns angles in the same indexing sequence as in knn
            angles = self.calculate_headings(current_point, knn, prev_angle)

            # Calculate the candidate indexes (largest angles first)
            candidates = np.argsort(-angles)

            i = 0
            invalid_hull = True

            while invalid_hull and i < len(candidates):
                candidate = candidates[i]

                # Create a test hull to check if there are any self-intersections
                next_point = np.reshape(self.data_set[knn[candidate]], (1, 2))
                test_hull = np.append(hull, next_point, axis=0)

                line = asLineString(test_hull)
                invalid_hull = not line.is_simple
                i += 1

            if invalid_hull:
                return self.recurse_calculate()

            # prev_angle = self.calculate_headings(current_point, np.array([knn[candidate]]))
            prev_angle = self.calculate_headings(knn[candidate], np.array([current_point]))
            current_point = knn[candidate]
            hull = test_hull

            # write_line_string(hull)

            self.indices[current_point] = False
            step += 1

        poly = asPolygon(hull)

        count = 0
        total = self.data_set.shape[0]
        for ix in range(total):
            pt = asPoint(self.data_set[ix, :])
            if poly.intersects(pt) or pt.within(poly):
                count += 1
            else:
                d = poly.distance(pt)
                if d < 1e-5:
                    count += 1

        if count == total:
            return hull
        else:
            return self.recurse_calculate()