import numpy as np
from visualiser import visualise
from solver import getPositions, getPositionsBlocked, getPositionsBlockedInverted


def getEnergy(pos, vel, mass, G):
    """
	Get kinetic energy (KE) and potential energy (PE) of simulation
	pos is N x 3 matrix of positions
	vel is N x 3 matrix of velocities
	mass is an N x 1 vector of masses
	G is Newton's Gravitational constant
	KE is the kinetic energy of the system
	PE is the potential energy of the system
	"""
    # Kinetic Energy:
    KE = 0.5 * np.sum(np.sum(mass * vel ** 2))

    # Potential Energy:

    # positions r = [x,y,z] for all particles
    x = pos[:, 0:1]
    y = pos[:, 1:2]
    z = pos[:, 2:3]

    # matrix that stores all pairwise particle separations: r_j - r_i
    dx = x.T - x
    dy = y.T - y
    dz = z.T - z

    # matrix that stores 1/r for all particle pairwise particle separations
    inv_r = np.sqrt(dx ** 2 + dy ** 2 + dz ** 2)
    inv_r[inv_r > 0] = 1.0 / inv_r[inv_r > 0]

    # sum over upper triangle, to count each interaction only once
    PE = G * np.sum(np.sum(np.triu(-(mass * mass.T) * inv_r, 1)))

    return KE, PE


def main():
    """ N-body simulation """

    # Simulation parameters
    N = 10  # Number of particles
    tEnd = 2.0  # time at which simulation ends
    dt = 0.01  # timestep
    softening = 0.1  # softening length
    G = 1.0  # Newton's Gravitational Constant

    # positions = getPositions(G, softening, tEnd, dt, N)
    # positions = getPositionsBlocked(G, softening, tEnd, dt, N)
    positions = getPositionsBlockedInverted(G, softening, tEnd, dt, N)
    visualise(positions, tEnd, dt)


if __name__ == "__main__":
    main()
