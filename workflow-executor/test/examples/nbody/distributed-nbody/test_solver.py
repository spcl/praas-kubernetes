import numpy as np


def getAccSub(pos, mass, G, softening, start, end):
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
    dx = x.T - x[start:end]
    dy = y.T - y[start:end]
    dz = z.T - z[start:end]

    # matrix that stores 1/r^3 for all particle pairwise particle separations
    inv_r3 = (dx ** 2 + dy ** 2 + dz ** 2 + softening ** 2)
    inv_r3[inv_r3 > 0] = inv_r3[inv_r3 > 0] ** (-1.5)

    ax = G * (dx * inv_r3) @ mass
    ay = G * (dy * inv_r3) @ mass
    az = G * (dz * inv_r3) @ mass

    # pack together the acceleration components
    a = np.hstack((ax, ay, az))

    return a

import numpy as np

state = dict()

def getPositionsBlockedInverted(G, softening, tEnd, dt, N):
    # Generate Initial Conditions
    np.random.seed(17)  # set the random number generator seed
    mass = 20.0 * np.ones((N, 1)) / N  # total mass of particles is 20
    pos = np.random.randn(N, 3)  # randomly selected positions and velocities
    vel = np.random.randn(N, 3)
    vel -= np.mean(mass * vel, 0) / np.mean(mass)  # Convert to Center-of-Mass frame
    t = 0

    # calculate initial gravitational accelerations
    acc = getAcc(pos, mass, G, softening)

    # number of timesteps
    Nt = int(np.ceil(tEnd / dt))

    # save particle orbits for plotting
    pos_save = np.zeros((N, 3, Nt + 1))
    pos_save[:, :, 0] = pos

    functions = 3
    blockSize = int(N/functions)
    for j in range(functions):
        state[j] = dict()
        block_start = j * blockSize
        block_end = block_start + blockSize
        if N - block_end < blockSize:
            block_end = N
        print(block_start, block_end)
        state[j]["pos"] = pos[block_start:block_end]

    for j in range(functions):
        sub_vel = vel[block_start:block_end]
        sub_acc = acc[block_start:block_end]
        positions_func(j, tEnd, dt, G, mass, softening, N, )
    return pos_save


def positions_func(j, tEnd, dt, G, mass, softening, N, sub_vel, sub_acc):
    t = 0
    Nt = int(np.ceil(tEnd / dt))
    sub_pos = state[j]["pos"]
    size = len(sub_pos)
    print(sub_pos)
    for i in range(Nt):
        # (1/2) kick
        sub_vel += sub_acc * dt / 2.0

        # drift
        sub_pos += sub_vel * dt
        state[j]["pos"] = sub_pos  # Send message to everyone about our positions

        # Read the other messages and reconstruct the whole position space

        # update accelerations
        sub_acc[:] = 0
        for k in range(size):
            for other_obj in range(N):
                dx = pos[other_obj, 0] - sub_pos[k, 0]
                dy = pos[other_obj, 1] - sub_pos[k, 1]
                dz = pos[other_obj, 2] - sub_pos[k, 2]
                inv_r3 = (dx ** 2 + dy ** 2 + dz ** 2 + softening ** 2) ** (-1.5)
                sub_acc[k, 0] += G * (dx * inv_r3) * mass[other_obj]
                sub_acc[k, 1] += G * (dy * inv_r3) * mass[other_obj]
                sub_acc[k, 2] += G * (dz * inv_r3) * mass[other_obj]

        # (1/2) kick
        sub_vel += sub_acc * dt / 2.0

        # update time
        t += dt

        # save energies, positions for plotting trail
        pos_save[block_start:block_end, :, i + 1] = sub_pos


def getPositions(G, softening, tEnd, dt, N):
    # Generate Initial Conditions
    np.random.seed(17)  # set the random number generator seed


    mass = 20.0 * np.ones((N, 1)) / N  # total mass of particles is 20
    pos = np.random.randn(N, 3)  # randomly selected positions and velocities
    vel = np.random.randn(N, 3)
    t = 0

    # Convert to Center-of-Mass frame
    vel -= np.mean(mass * vel, 0) / np.mean(mass)

    # calculate initial gravitational accelerations
    acc = np.zeros((N, 3))
    for i in range(N):
        getAccObj(i, pos, acc, N, mass, G, softening)

    # number of timesteps
    Nt = int(np.ceil(tEnd / dt))

    # save energies, particle orbits for plotting trails
    pos_save = np.zeros((N, 3, Nt + 1))
    pos_save[:, :, 0] = pos

    # Simulation Main Loop
    for i in range(Nt):
        # (1/2) kick
        vel += acc * dt / 2.0

        # drift
        pos += vel * dt

        # update accelerations
        acc[:] = 0
        for j in range(N):
            getAccObj(j, pos, acc, N, mass, G, softening)

        # (1/2) kick
        vel += acc * dt / 2.0

        # update time
        t += dt

        # save energies, positions for plotting trail
        pos_save[:, :, i + 1] = pos
    return pos_save


def getAcc(pos, mass, G, softening):
    """
    Calculate the acceleration on each particle due to Newton's Law
    pos  is an N x 3 matrix of positions
    mass is an N x 1 vector of masses
    G is Newton's Gravitational constant
    softening is the softening length
    a is N x 3 matrix of accelerations
    """

    N = pos.shape[0]
    a = np.zeros((N, 3))

    for i in range(N):
        getAccObj(i, pos, a, N, mass, G, softening)

    return a


def getAccObj(i, pos, a, N, mass, G, softening):
    for j in range(N):
        dx = pos[j, 0] - pos[i, 0]
        dy = pos[j, 1] - pos[i, 1]
        dz = pos[j, 2] - pos[i, 2]
        inv_r3 = (dx ** 2 + dy ** 2 + dz ** 2 + softening ** 2) ** (-1.5)
        a[i, 0] += G * (dx * inv_r3) * mass[j]
        a[i, 1] += G * (dy * inv_r3) * mass[j]
        a[i, 2] += G * (dz * inv_r3) * mass[j]
