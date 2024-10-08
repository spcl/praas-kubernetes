import psutil
import time

import pickle

import sys
import boto3

import numpy as np
import praassdk


def leader(args):
    start = time.perf_counter()
    result = dict()
    num_workers = int(args.pop(0))
    simulation_length = float(args.pop(0))
    time_step = float(args.pop(0))

    positions = []
    steps = int(np.ceil(simulation_length / time_step))
    session = boto3.Session(
        aws_access_key_id=args.pop(0),
        aws_secret_access_key=args.pop(0)
    )
    s3 = session.resource("s3")
    chunk = 0

    total_comm_time = 0
    total_s3_time = 0
    for i in range(steps):
        # Get all results
        comm_start = time.perf_counter()
        pos = praassdk.msg.get(1, f"all-{i}")
        positions.append(pos.tolist())
        mid_time = time.perf_counter()
        comm_duration = mid_time - comm_start
        total_comm_time += comm_duration * 1000

        if len(positions) * len(pos) > 32000000:
            obj = s3.Object("praas-workflow-benchmarks", "benchmark_result_"+str(chunk))
            obj.put(Body=pickle.dumps(positions))
            positions.clear()
            chunk += 1
        s3_duration = time.perf_counter() - mid_time
        total_s3_time += s3_duration * 1000

    result["comm_time"] = total_comm_time

    s3_start = time.perf_counter()
    obj = s3.Object("praas-workflow-benchmarks", "benchmark_result_"+str(chunk))
    obj.put(Body=pickle.dumps(positions))
    result["chunks"] = chunk + 1
    s3_duration = time.perf_counter() - s3_start
    total_s3_time += s3_duration * 1000
    result["s3_time"] = total_s3_time

    duration = time.perf_counter() - start
    result["total_time"] = duration * 1000
    return result


def n_body(args):
    init_start = time.perf_counter()
    idx = praassdk.func_id
    N = int(args.pop(0))
    simulation_length = float(args.pop(0))
    time_step = float(args.pop(0))
    softening = float(args.pop(0))
    G = float(args.pop(0))
    num_workers = int(args.pop(0))

    # Calculate input range
    block_size = int(N/num_workers)
    remainder = N % num_workers
    start = (idx-1) * block_size + min(idx-1, remainder)
    if remainder >= idx:
        block_size += 1
    end = start + block_size
    debug("worker range:", start, "-", end)

    np.random.seed(17 + idx)  # set the random number generator seed
    masses = 20.0 * np.ones((N, 1)) / N  # total mass of particles is 20

    sub_masses = masses[start:end]
    pos = np.random.randn(block_size, 3)  # randomly selected positions and velocities
    vel = np.random.randn(block_size, 3)
    vel -= np.mean(sub_masses * vel, 0) / np.mean(sub_masses)  # Convert to Center-of-Mass frame
    init_duration = time.perf_counter() - init_start
    result = positions_func(idx, simulation_length, time_step, G, softening, N, masses, vel, pos, start, end, num_workers)
    result["total_time"] = (time.perf_counter() - init_start) * 1000
    result["init_time"] = init_duration * 1000
    p = psutil.Process()
    result["cpu_times"] = p.cpu_times()
    return result


def positions_func(idx, tEnd, dt, G, softening, N, mass, sub_vel, sub_pos, start, end, num_workers):
    t = 0
    Nt = int(np.ceil(tEnd / dt))
    size = end-start
    sub_acc = np.zeros((size, 3))
    total_comm_seconds = 0
    total_comp_seconds = 0
    for i in range(Nt):
        # (1/2) kick
        comp_start = time.perf_counter()
        sub_vel += sub_acc * dt / 2.0

        # drift
        sub_pos += sub_vel * dt
        quarter_time = time.perf_counter()
        total_comp_seconds += quarter_time - comp_start
        pos = scatter_gather(idx, i, num_workers, sub_pos)
        mid_time = time.perf_counter()
        total_comm_seconds += mid_time - quarter_time

        # update accelerations
        sub_acc = getAccSub(pos, mass, G, softening, start, end)

        # (1/2) kick
        sub_vel += sub_acc * dt / 2.0

        # update time
        t += dt
        total_comp_seconds += time.perf_counter() - mid_time

    return {"comm_time": total_comm_seconds * 1000, "compute_time": total_comp_seconds * 1000}


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
    return np.hstack((ax, ay, az))


def scatter_gather(idx, iteration, n_workers, pos):
    sub_msg_name = f"sub-{iteration}"
    all_msg_name = f"all-{iteration}"

    if idx == 1:
        positions = [pos]
        others = list(range(2, n_workers + 2))
        for other in others[:-1]:
            positions.append(praassdk.msg.get(other, sub_msg_name))
        all_pos = np.vstack(positions)
        praassdk.msg.broadcast(others, all_msg_name, all_pos)
        return all_pos
    else:
        praassdk.msg.put(1, sub_msg_name, pos)
        return praassdk.msg.get(1, all_msg_name)


def debug(*args):
    print(praassdk.func_id, *args, file=sys.stderr, flush=True)
