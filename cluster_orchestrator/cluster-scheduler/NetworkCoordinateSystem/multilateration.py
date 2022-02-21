import numpy as np
from scipy.optimize import minimize
import math

"""
Solving the multilateration as a geometrical problem is often impractical because it requires a high accuracy on the
measurements which is not given in this context due to the variance of ping results. The worst case scenario would be
if the circles to not meet in a single point. Then the set of equations has no solutions. Even with perfect accuracy
the approach does not scale well. Therefore, try solving the problem from an optimisation point of view. Ignoring
circles and intersections, which is the point X = (x,y) that provides us with the best approximation of the actual 
position of the user U. Given an point X, we can estimate how well it replaces U by simply calculating its distance 
from each reference point R_i. If those distances perfectly match with their respective distance d_i, then X is 
indeed P. The more X deviates from these distances, the further it is assumed from U. 
Optimisation problem: We need to find the point X that minimises a certain error function. For our X, we have not on
but n sources of error, with n being the number of reference points used for the multilateration process. One for
each reference point:
                                    e_1 = d_1 - dist(X,R_1)
                                    e_2 = d-2 - dist(X,R_2)
                                            ...
                                    e_n = d-n - dist(X,R_n)
A common way to merge these different contributions is to average their squares. This takes away the possibility of 
negative and positive errors to cancel each others out. Mean Squared Error:
                                sum(d_i-dist(X,R_i))^2 / N
This solution can take into account an arbitrary number of reference points.
"""

point_progress = []
it = 1
vivaldi_locations = None
ping_distances = None

def mean_squared_error(x, vivaldi_locations, ping_distances):
    mse = 0.0
    for loc, dist in zip(vivaldi_locations, ping_distances):
        dist_calculated = np.linalg.norm(loc - x)
        mse += math.pow(dist_calculated - dist, 2.0)

    return mse / len(ping_distances)

def multilateration(data):
    """
    Find the point X that minimises the mean squared error. Scipy comes with several optimisation algorithms. A very
    important aspect for the optimisation algorithm is the initial guess. Providing a good starting point can reduce
    the execution time significantly. There are several initial guesses we could use. A reasonable one is to use the
    center of the reference point which detected the closest distance, i.e., the lowest latency.
    """
    global vivaldi_locations
    global ping_distances
    global point_progress
    min_distance = float('inf')
    min_distances = {}
    closest_locations = {}
    for vivaldi_coords, ip_rtt_stats in data:
        for ip, rtt in ip_rtt_stats.items():
            # Our distance measurements here are the latencies
            if ip in min_distances:
                if rtt < min_distances[ip]:
                    min_distances[ip] = rtt
                    closest_locations[ip] = vivaldi_coords
            else:
                if rtt < min_distance:
                    min_distances[ip] = rtt
                    closest_locations[ip] = vivaldi_coords

    print(f"initital guesses: {closest_locations}")
    initial_locations = closest_locations

    vivaldi_locations, ping_distances = zip(*data)

    # Group ping results for each IP
    ip_rtts = {}
    for e in ping_distances:
        for ip, rtt in e.items():
            ip_rtts.setdefault(ip, []).append(rtt)

    ip_locations = {}
    for i, (key, values) in enumerate(ip_rtts.items()):
        ping_distances = values
        print(f"Start minimizing MSE for target IP {key}.")
        print(f"Initial location: {initial_locations[key]}")
        result = minimize(
            mean_squared_error,
            initial_locations[key],
            callback=callbackF,
            args=(vivaldi_locations, values),
            method='L-BFGS-B',
            options={
                'ftol': 1e-5,
                'maxiter': 1e+7
            })
        point_progress = []

        ip_locations[key] = result.x
        print(f"Finished minimizing MSE.")

    return ip_locations

def callbackF(Xi):
    global it
    global vivaldi_locations
    global ping_distances
    global point_progress

    print(
        f"{it:<10}{round(Xi[0], 4):^10}{round(Xi[1], 4):^10}{round(mean_squared_error(Xi, vivaldi_locations, ping_distances), 4):>5}")
    point_progress.append(Xi)
    it += 1