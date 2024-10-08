package e2e

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"praas/test"
	"praas/test/assertions"
)

func TestProcessGarbageCollection(t *testing.T) {
	rand.Seed(time.Now().Unix())
	clients := test.NewTestClients(GetKubeConfig())
	pid, err := test.CreateAppWithProcesses(clients, 50, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Clean up
	defer func() {
		err = clients.Praas.DeleteApp(pid)
		if err != nil {
			t.Logf("Failed to delete app: %s", err.Error())
		}
	}()

	time.Sleep(time.Minute)
	// Sub-tests
	cases := []struct {
		name string
	}{
		{"delete 1"},
		{"delete 2"},
		{"delete 3"},
		{"delete 4"},
		{"delete 5"},
	}

	// Execute test cases
	for _, tCase := range cases {
		t.Run(
			tCase.name, func(t *testing.T) {
				// Get a random pod
				pods, randIdx := getRandomPod(t, clients.Praas, pid)
				podName := pods[randIdx]

				// Send delete command
				res, err := clients.Proxy.SendRequest(podName, "/delete")
				if err != nil {
					t.Fatalf(
						"Failed to send delete command to pod %s, error: %s, result: %s",
						podName,
						err.Error(),
						res,
					)
				}
				pods[randIdx] = pods[0]
				livePods := pods[1:]

				// Ensure selected session is terminating
				assertions.AssertTerminating(t, clients.Kube, podName, scaleDownTimeout)
				// Ensure pods that were not supposed to be deleted are still around
				assertions.AssertPodsAlive(t, clients.Kube, livePods)
				// Ensure pod actually gets deleted
				assertions.AssertScale(t, clients.Praas, pid, len(livePods), false, scaleDownTimeout)
			},
		)
	}
}

func TestAppDeleteDeletesProcesses(t *testing.T) {
	clients := test.NewTestClients(GetKubeConfig())
	pid, err := test.CreateAppWithProcesses(clients, 3, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Clean up
	defer func() {
		_ = clients.Praas.DeleteApp(pid)
	}()

	// Assert setup is valid
	assertions.AssertScale(t, clients.Praas, pid, 3, true, time.Second)

	err = clients.Praas.DeleteApp(pid)
	if err != nil {
		t.Fatal(err)
	}

	// Assert sessions have been deleted
	assertions.AssertScale(t, clients.Praas, pid, 0, false, scaleDownTimeout)
}

func TestAppDeletionOnlyDeletesSelected(t *testing.T) {
	clients := test.NewTestClients(GetKubeConfig())
	var pids []int

	for i := 0; i < 3; i++ {
		pid, err := test.CreateAppWithProcesses(clients, 3, nil, nil)
		if err != nil {
			t.Fatal(err)
		}

		// Assert setup is valid
		assertions.AssertScale(t, clients.Praas, pid, 3, true, time.Second)
		pids = append(pids, pid)
	}

	// Clean up
	defer func() {
		for _, pid := range pids {
			_ = clients.Praas.DeleteApp(pid)
		}
	}()

	if pids == nil || len(pids) < 3 {
		t.Fatal("Length of pids is not 3: ", len(pids))
	}

	// Gather pods that should survive the deletion
	survivePids := []int{pids[0], pids[2]}
	var survivePods []string
	for _, sPid := range survivePids {
		pods, err := clients.Praas.GetPodsForApp(sPid)
		if err != nil {
			t.Fatal(err)
		}
		survivePods = append(survivePods, pods...)
	}

	// Perform delete
	err := clients.Praas.DeleteApp(pids[1])
	if err != nil {
		t.Fatal(err)
	}

	// Assert deletion was successful
	assertions.AssertScale(t, clients.Praas, pids[1], 0, false, scaleDownTimeout)

	// Assert the other pods are still available and running
	assertions.AssertPodsAlive(t, clients.Kube, survivePods)
}

func TestContinuousCreateDelete(t *testing.T) {
	podToPid := make(map[string]string)
	var PIDs []string
	var deletedPods []string
	startScale := 50
	deleteDelay := 35
	clients := test.NewTestClients(GetKubeConfig())
	pid, err := test.CreateAppWithProcesses(clients, startScale, &PIDs, podToPid)
	if err != nil {
		t.Fatal(err)
	}
	// Clean up
	defer func(Praas test.PraasClient, i int) {
		if t.Failed() {
			fmt.Println("Pods which were marked as deleted, but are alive:")
			pods, err := clients.Praas.GetPodsForApp(pid)
			fmt.Println(pods)
			if err == nil {
				for _, pod := range pods {
					if canDownscale(clients.Proxy, pod) {
						fmt.Println("    ", pod)
					}
				}
			}
			fmt.Println("Pods, which should exist, but do not: ")
			for pod, pid := range podToPid {
				if !test.IsPodAlive(clients.Kube, pod) {
					fmt.Println("    ", pod, " (", pid, ")")
				}
			}
			_ = Praas.DeleteApp(i)
		} else {
			_ = Praas.DeleteApp(i)
		}
	}(clients.Praas, pid)

	// Validate starting state
	assertions.AssertScale(t, clients.Praas, pid, startScale, true, scaleUpTimeout)

	// Continuously create and delete pods
	for i := 0; i < startScale+deleteDelay; i++ {
		var pods []string
		var idx int
		foundToDelete := false
		for !foundToDelete {
			pods, idx = getRandomPod(t, clients.Praas, pid)

			// Make sure it's not reporting as to be deleted
			if !canDownscale(clients.Proxy, pods[idx]) {
				foundToDelete = true
			}
		}
		if pods == nil {
			t.Fatal("Pods is nil")
		}

		if i >= deleteDelay {
			id, podName, err := clients.Praas.CreateProcess(pid)
			if err != nil {
				t.Fatal(err)
			}
			fmt.Println("Create pod: ", podName)
			podToPid[podName] = id
		}

		if i < startScale {
			podToDelete := pods[idx]
			err = test.MarkDeletePod(clients.Proxy, podToDelete)
			if err != nil {
				t.Fatal(err)
			}
			delete(podToPid, podToDelete)
			deletedPods = append(deletedPods, podToDelete)
		}

		// Make sure we can delete pods while under constant creation requests
		assertions.AssertScaleUnder(t, clients.Praas, pid, startScale+i+1)
		assertions.AssertScaleOver(t, clients.Praas, pid, startScale-deleteDelay+1)
		time.Sleep(time.Second)
	}

	// Make sure we get the expected state and scale
	assertions.AssertScale(t, clients.Praas, pid, startScale, false, 2*scaleDownTimeout)
}

func TestProcessSwapIn(t *testing.T) {
	// Create a process
	clients := test.NewTestClients(GetKubeConfig())
	pid, err := clients.Praas.CreateApp()
	if err != nil {
		t.Fatalf("Failed to create process: %s", err.Error())
	}

	// Clean up
	defer func() {
		err = clients.Praas.DeleteApp(pid)
		if err != nil {
			t.Fatalf("Failed to delete process: %s", err.Error())
		}
	}()

	// Create session
	sessionID, pod, err := clients.Praas.CreateProcess(pid)
	if err != nil {
		t.Fatal(err)
	}
	assertions.AssertScale(t, clients.Praas, pid, 1, true, scaleUpTimeout)

	time.Sleep(5 * time.Second)

	// Delete session (swap out)
	err = test.MarkDeletePod(clients.Proxy, pod)
	if err != nil {
		t.Fatal(err)
	}
	assertions.AssertScale(t, clients.Praas, pid, 0, false, scaleDownTimeout+time.Minute)

	// Swap in new session
	pod, err = clients.Praas.SwapProcessIn(pid, sessionID)
	if err != nil {
		t.Fatal(err)
	}

	// Try to swap in the same session again
	pod, err = clients.Praas.SwapProcessIn(pid, sessionID)
	if err == nil {
		t.Fatal("Second swap-in expected to fail, but it did not")
	}
}

/**** HELPERS ****/

func canDownscale(proxy test.ProxyClient, podName string) bool {
	resp, err := proxy.SendRequest(podName, "/custom-metrics")
	if err != nil {
		fmt.Println("Error retrieving custom metrics: ", err)
		return true
	}

	var userMetrics struct {
		Value    float64 `json:"metric_value"`
		Interval int     `json:"interval"`
	}
	err = json.Unmarshal([]byte(resp), &userMetrics)
	if err != nil {
		fmt.Println("Error unmarshaling custom metric: ", err)
		return true
	}

	return userMetrics.Value == 0
}

func getRandomPod(t *testing.T, client test.PraasClient, pid int) ([]string, int) {
	pods, err := client.GetPodsForApp(pid)
	if err != nil || len(pods) == 0 {
		t.Fatal(err, len(pods))
	}
	randIdx := rand.Intn(len(pods))
	return pods, randIdx
}
