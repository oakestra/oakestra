import numpy as np
import sys

class VivaldiCoordinate:
    """
    Coordinate represents a node in a Vivaldi network coordinate system.

    Coordinate consists of a euclidean and non-euclidean component.
    - Euclidean portion models a high-speed Internet core with latencies proportional to geographic distance
    - Non-euclidean height models the time it takes packets to travel the access link from the node to the core
    A packet sent from one node to another must travel the source node's height, then travel in the Euclidean space,
    then travel the destinations node's height. Even if the two nodes have the same height, the distance between them
    is their Euclidean distance plus the two heights.

    Based on: "Vivaldi: A Decentralized Network Coordinate System" by Dabek, et al.
    """

    def __init__(self, dims: int, max_error=1.5, min_height=1e-5, ce=0.25, cc=0.25, rho=150.0):
        """
        :param dims: Dimensionality of the coordinate system
        :param max_error: Limit to the error anz observation maz induce
        :param min_height: The minimum height of any Coordinate
        :param ce: A tuning factor which impacts the maximum impact an observation can have on a Coordinate
        :param cc: A tuning factor which impacts the maximum impact an observation can have on a Coordinate
        :param rho: A tuning factor for the effect of gravity exerted by the origin to control drift.
                    See "Network Coordinates in the Wild" by Ledlie, et al. for more information
        """
        self.dims = dims
        self.max_error = max_error
        self.min_height = min_height
        self.ce = ce
        self.cc = cc
        self.rho = rho
        # Euclidean component
        self.vector = np.zeros(dims)
        # Non-euclidean component
        self.height = min_height
        # Measure of confidence that this 'Coordinate' represents the true distance from the origin
        self.error = max_error

        assert dims > 0
        assert 0.0 <= ce < 1
        assert 0.0 <= cc < 1

    def update(self, rtt: float, remote: 'VivaldiCoordinate', local_adjustment=0.0, remote_adjustment=0.0):
        """
        Updates this nodes 'Coordiante' based on the measured 'rtt' to 'remote'
        Cf. Figure 3: The Vivaldi algorithm, with an adaptive timestep

        :param rtt: Round-trip time to 'remote'
        :param remote: 'Coordinate' of another node in the Vivaldi network
        :param local_adjustment: tuning parameter
        :param remote_adjustment: tuning parameter
        """
        # Estimate the RTT by calculating the Vivaldi distance
        dist = self.distance(remote)
        dist = max(dist, dist + local_adjustment + remote_adjustment)

        # Protect against divide by zero
        rtt = max(rtt, sys.float_info.min)

        # Weight is used to push in proportion to the error: large error -> large force
        # See pape figure 3: Sample weight balances lcoal and remote error
        weight = self.error / max(self.error + remote.error, sys.float_info.min)

        # Calculate relative error of this 'Coordinate'
        # See paper figure 3: Compute relative error of this sample
        err = abs(dist - rtt) / rtt

        # See paper figure 3: Update weighted moving average of local error
        self.error = min(err * self.ce * weight + self.error * (1.0 - self.ce * weight), self.max_error)

        # Update local coordinates
        delta = self.cc * weight
        force = delta * (rtt - dist)
        # Apply the force exerted by the remote node
        self.apply_force(remote, force)

        # Uncomment to add gravity effect
        #origin = VivaldiCoordinate(self.dims)

        #dist = self.distance(origin)
        #dist = max(dist, dist + local_adjustment)

        # Gravity towards the origin exerts a pulling force which is a small fraction of the expected diameter
        # of the network. "Network Coordinates in the Wild", Sec. 7.2
        #force = -1.0 * pow(dist / self.rho, 2.0)

        # Apply the force of gravity exerted by the origin
        #self.apply_force(origin, force)

    def distance(self, remote_coordinate: 'VivaldiCoordinate'):
        """
        Returns the Vivaldi distance to the remote node in estimated RTT.
        Since we are using heigh vectors the norm disatance has to be calculated accordingly:
        Let [x,x_h] and [y,y_h] be height vectors.
        ||[x,x_h]|| = ||x|| + x_h
        [x,x_h] - [y,y_h] = [(x-y),x_h + y_h]
        -> ||[x,x_h] - [y,y_h]|| = ||x-y|| + x_h + y_h
                                 = euclidean_distance(x,y) + x_h + y_h

        :param remote_coordinate: Remote node
        """
        euclidean_dist = np.linalg.norm(self.vector - remote_coordinate.vector)
        return euclidean_dist + self.height + remote_coordinate.height

    def apply_force(self, remote_coordinate: 'VivaldiCoordinate', force: float):
        """
        Applies a force against this 'Coordinate' from the direction of the remote 'Coordinate'.
        If the force is positive, this 'Coordinate' will be pushed away from remote.
        If the force is negative, this 'Coordinate' will be pulled closer to remote.
        """
        unit_vector = self.unit_vector(self.vector, remote_coordinate.vector)  # u(x_i - X_j)
        unit_vector *= force  # delta * (rtt - ||x_i - x_j||) * u(x_i - x_j) = force * u(x_i, x_j)
        self.vector += unit_vector  # x_i = x_i + delta * (rtt - ||x_i - x_j||) * u(x_i - x_j)

        # Since we are working with height vectors we have to adjust the height accordingly
        # cf. section 5.4 Height vectors
        diff = self.vector - remote_coordinate.vector
        mag = np.linalg.norm(diff)
        self.height = max(
            (self.height + remote_coordinate.height) * force / (mag + self.height + remote_coordinate.height),
            self.min_height)

    def unit_vector(self, src: np.ndarray, dest: np.ndarray) -> np.ndarray:
        diff = src - dest
        mag = np.linalg.norm(diff)

        # Push apart if the vectors aren't too close
        if not np.isclose(mag, 0):
            diff /= mag
            return diff

        # Otherwise if they are very close (e.g. at the beginning) push in random direction
        # cf. "Two nodes occupying the same location will have a spring
        #      pushing them away from each other in some arbitrary direction."
        rand_vector = np.random.rand(self.dims)
        rand_unit_vector = rand_vector / np.linalg.norm(rand_vector)

        return rand_unit_vector

    def __str__(self):
        return f"VivaldiCoordinate: Vector: {self.vector} Height: {self.height} Error: {self.error}"
