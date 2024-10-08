import numpy as np
import matplotlib.pyplot as plt
from matplotlib import animation, rc


def visualise(positions, tEnd, dt):
    # number of timesteps
    Nt = int(np.ceil(tEnd / dt))

    # prep figure
    rc('animation', html='html5')
    fig = plt.figure(figsize=(2, 2), dpi=300)
    # ax1 = plt.subplot()

    line1 = plt.scatter([], [], s=1, color=[.7, .7, 1])
    line2 = plt.scatter([], [], s=10, color='blue')
    ax = plt.gca()
    ax.set(xlim=(-4, 4), ylim=(-4, 4))
    # hide x-axis
    ax.get_xaxis().set_visible(False)

    # hide y-axis
    ax.get_yaxis().set_visible(False)
    # return line1, line2

    anim = animation.FuncAnimation(
        fig,
        animate,
        # init_func=init,
        fargs=([positions, line1, line2]),
        frames=Nt,
        interval=1,
        blit=True
    )

    # for i in range(Nt):
    #     # plot in real time
    #     plt.sca(ax1)
    #     plt.cla()
    #     x = positions[:, 0, i]
    #     y = positions[:, 1, i]
    #     xx = positions[:, 0, max(i - 50, 0):i + 1]
    #     yy = positions[:, 1, max(i - 50, 0):i + 1]
    #
    #     plt.scatter(xx, yy, s=1, color=[.7, .7, 1])
    #     plt.scatter(x, y, s=10, color='blue')
    #     ax1.set(xlim=(-2, 2), ylim=(-2, 2))
    #     ax1.set_aspect('equal', 'box')
    #     ax1.set_xticks([-2, -1, 0, 1, 2])
    #     ax1.set_yticks([-2, -1, 0, 1, 2])
    #
    #     plt.pause(0.001)

    # Save figure
    # plt.savefig('nbody.png', dpi=240)
    # plt.show()
    anim.save('nbody.gif', writer='imagemagick', fps=60)


def animate(i, positions, line1, line2):
    x = positions[:, 0, i]
    y = positions[:, 1, i]
    xx = positions[:, 0, max(i - 50, 0):i + 1]
    yy = positions[:, 1, max(i - 50, 0):i + 1]

    # line1 = plt.scatter(xx, yy, s=1, color=[.7, .7, 1])
    # line2 = plt.scatter(x, y, s=10, color='blue')

    line1.set_offsets(np.hstack((xx.flatten()[:,np.newaxis], yy.flatten()[:,np.newaxis])))
    line2.set_offsets(np.hstack((x[:,np.newaxis], y[:,np.newaxis])))

    return line1, line2
