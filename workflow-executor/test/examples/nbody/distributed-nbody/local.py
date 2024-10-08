import psutil
import time

import numpy as np
import matplotlib.pyplot as plt

from visualiser import visualise
from test_solver import getAccSub

"""
Create Your Own N-body Simulation (With Python)
Philip Mocz (2020) Princeton Univeristy, @PMocz

Simulate orbits of stars interacting due to gravity
Code calculates pairwise forces according to Newton's Law of Gravity
"""


def getAcc(pos, mass, G, softening):
    """
    Calculate the acceleration on each particle due to Newton's Law
	pos  is an N x 3 matrix of positions
	mass is an N x 1 vector of masses
	G is Newton's Gravitational constant
	softening is the softening length
	a is N x 3 matrix of accelerations
	"""
    # positions r = [x,y,z] for all particles
    x = pos[:, 0:1]
    y = pos[:, 1:2]
    z = pos[:, 2:3]

    # matrix that stores all pairwise particle separations: r_j - r_i
    dx = x.T - x
    dy = y.T - y
    dz = z.T - z

    # matrix that stores 1/r^3 for all particle pairwise particle separations
    inv_r3 = (dx ** 2 + dy ** 2 + dz ** 2 + softening ** 2)
    inv_r3[inv_r3 > 0] = inv_r3[inv_r3 > 0] ** (-1.5)

    ax = G * (dx * inv_r3) @ mass
    ay = G * (dy * inv_r3) @ mass
    az = G * (dz * inv_r3) @ mass

    # pack together the acceleration components
    a = np.hstack((ax, ay, az))

    return a


def main():
    """ N-body simulation """
    start_time = time.perf_counter()

    # Simulation parameters
    N = 50  # Number of particles
    t = 0  # current time of the simulation
    tEnd = 10.0  # time at which simulation ends
    dt = 0.01  # timestep
    softening = 0.1  # softening length
    G = 1.0  # Newton's Gravitational Constant
    plot = True  # switch on for plotting as the simulation goes along
    funcs = 5
    block_size = int(N/funcs)

    # Generate Initial Conditions
    np.random.seed(17)  # set the random number generator seed

    mass = 20.0 * np.ones((N, 1)) / N  # total mass of particles is 20
    pos = np.random.randn(N, 3)  # randomly selected positions and velocities
    vel = np.random.randn(N, 3)

    # Convert to Center-of-Mass frame
    vel -= np.mean(mass * vel, 0) / np.mean(mass)

    # calculate initial gravitational accelerations
    acc = getAcc(pos, mass, G, softening)

    # number of timesteps
    Nt = int(np.ceil(tEnd / dt))

    # save energies, particle orbits for plotting trails
    pos_save = np.zeros((N, 3, Nt + 1))
    pos_save[:, :, 0] = pos

    # Simulation Main Loop
    increment = int(Nt / 100)
    for i in range(Nt):
        # print(i, "-", psutil.Process().memory_info(), flush=True)
        if i % increment == 0:
            print(i/increment, "%", sep="", end="\r")
        # (1/2) kick
        vel += acc * dt / 2.0

        # drift
        pos += vel * dt

        # update accelerations
        sub_accs = list()
        for j in range(funcs):
            start = j * block_size
            end = start + block_size
            if N - end < block_size:
                end = N
            sub_accs.append(getAccSub(pos, mass, G, softening, start, end))
        acc = np.vstack(sub_accs)

        # (1/2) kick
        vel += acc * dt / 2.0

        # update time
        t += dt

        # save positions for plotting trail
        pos_save[:, :, i + 1] = pos

    duration = time.perf_counter() - start_time
    print(f"\nDuration: {duration}", flush=True)

    if plot:
        visualise(pos_save, tEnd, dt)
    return 0


if __name__ == "__main__":
    main()
