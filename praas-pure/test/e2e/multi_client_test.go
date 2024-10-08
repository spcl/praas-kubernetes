package e2e

import (
	"math/rand"
	"sync"
	"testing"
	"time"

	"praas/test"
	"praas/test/assertions"
)

func TestMultiClientSwapIn(t *testing.T) {
	// Set up
	clients := test.NewTestClients(GetKubeConfig())

	lotsOfTimes := 10
	var sessionIDs []string
	pid, err := test.CreateAppWithProcesses(clients, lotsOfTimes, &sessionIDs, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Clean up
	defer func() {
		err = clients.Praas.DeleteApp(pid)
		if err != nil {
			t.Log(err)
		}
	}()

	// Now delete (swap out) all pods
	pods, err := clients.Praas.GetPodsForApp(pid)
	if err != nil {
		t.Fatal(err)
	}
	for _, pod := range pods {
		err = test.MarkDeletePod(clients.Proxy, pod)
		if err != nil {
			t.Fatal(err)
		}
	}
	assertions.AssertScale(t, clients.Praas, pid, 0, false, scaleDownTimeout+time.Minute)

	// DoGet the test lots of times to increase the chance of finding a bug
	var err1 error
	var err2 error
	for i := 0; i < lotsOfTimes; i++ {
		sessionID := sessionIDs[i]
		runFuncsInParallel(
			func() {
				_, err1 = clients.Praas.SwapProcessIn(pid, sessionID)
			}, func() {
				_, err2 = clients.Praas.SwapProcessIn(pid, sessionID)
			},
		)

		assertions.AssertScale(t, clients.Praas, pid, i+1, true, scaleUpTimeout)
		if err1 == nil && err2 == nil {
			t.Fatal("Both swap in calls were successful")
		} else if err1 != nil && err2 != nil {
			t.Log(err1)
			t.Log(err2)
			continue
		}
	}
}

func TestMultiClientSessionCreation(t *testing.T) {
	// Set up
	clients := test.NewTestClients(GetKubeConfig())

	numProcesses := 3
	pids := make([]int, numProcesses)
	for i := 0; i < numProcesses; i++ {
		pid, err := test.CreateAppWithProcesses(clients, 1, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		pids[i] = pid
	}

	// Clean up
	defer func() {
		for i := 0; i < numProcesses; i++ {
			err := clients.Praas.DeleteApp(pids[i])
			if err != nil {
				t.Log(err)
			}
		}
	}()

	// Sub-tests
	cases := []struct {
		name   string
		pid1   int
		pid2   int
		scale1 int
		scale2 int
	}{
		{"same process", pids[0], pids[0], 3, 3},
		{"different process", pids[1], pids[2], 2, 2},
	}

	// Execute test cases
	for _, tCase := range cases {
		t.Run(
			tCase.name, func(t *testing.T) {
				var err1 error
				var err2 error

				runFuncsInParallel(
					func() {
						_, _, err1 = clients.Praas.CreateProcess(tCase.pid1)
					}, func() {
						_, _, err2 = clients.Praas.CreateProcess(tCase.pid2)
					},
				)
				if err1 != nil {
					t.Fatal(err1)
				}
				if err2 != nil {
					t.Fatal(err2)
				}

				// Assert correct scales
				assertions.AssertScale(t, clients.Praas, tCase.pid1, tCase.scale1, true, scaleUpTimeout)
				assertions.AssertScale(t, clients.Praas, tCase.pid2, tCase.scale2, true, scaleUpTimeout)
			},
		)
	}
}

func TestMultiClientSessionCreateDelete(t *testing.T) {
	// Set up
	clients := test.NewTestClients(GetKubeConfig())

	startPods := 40
	var sessionIDs []string
	sessionToPod := make(map[string]string)
	podToSession := make(map[string]string)
	sessionDeleted := make(map[string]bool)
	pid, err := clients.Praas.CreateApp()
	if err != nil {
		t.Fatal(err)
	}
	// Clean up
	defer func() {
		err = clients.Praas.DeleteApp(pid)
		if err != nil {
			t.Log(err)
		}
	}()

	sessionCreateFunc := func(payloadID int) {
		id, podName, err := clients.Praas.CreateProcess(pid)
		if err != nil {
			t.Fatal(err)
		}
		assertions.AssertUniqueSessionID(t, &sessionIDs, []string{id})
		podToSession[podName] = id
		sessionToPod[id] = podName
	}

	for i := 0; i < startPods; i++ {
		sessionCreateFunc(i)
	}

	assertions.AssertScale(t, clients.Praas, pid, startPods, true, scaleUpTimeout)

	// Run two loops of create and delete
	createCounter := 0
	runFuncsInParallel(
		// Create
		func() {
			for i := 0; i < startPods; i++ {
				sessionCreateFunc(startPods + i)
				createCounter++
				time.Sleep(5 * time.Second)
			}
		},
		// Delete
		func() {
			for i := 0; i < startPods; i++ {
				// Find suitable pod to delete
				var pods []string
				var idx int
				foundToDelete := false
				for !foundToDelete {
					pods, err = clients.Praas.GetPodsForApp(pid)
					if err != nil || len(pods) == 0 {
						t.Log(err, len(pods))
						continue
					}
					idx = rand.Intn(len(pods))

					// Make sure it's not reporting as to be deleted
					if !canDownscale(clients.Proxy, pods[idx]) {
						foundToDelete = true
					}
				}
				if pods == nil {
					t.Log("Pods is nil")
					continue
				}

				// DoGet the deletion
				podToDelete := pods[idx]
				sessionID := podToSession[podToDelete]
				err = test.MarkDeletePod(clients.Proxy, podToDelete)
				if err != nil {
					t.Log(err)
				} else {
					sessionDeleted[sessionID] = true
				}
				time.Sleep(5 * time.Second)
			}
		},
	)
	deletedCount := 0
	for _, sessionID := range sessionIDs {
		if _, exists := sessionDeleted[sessionID]; exists {
			deletedCount++
		}
	}
	expectedScale := startPods + createCounter - deletedCount
	assertions.AssertScale(t, clients.Praas, pid, expectedScale, false, 2*scaleDownTimeout+time.Minute)

	// Make sure the things that shouldn't have been deleted are not
	for _, sessionID := range sessionIDs {
		_, exists := sessionDeleted[sessionID]
		if !exists {
			// This session shouldn't have been deleted, so check that
			pod := sessionToPod[sessionID]
			assertions.AssertPodsAlive(t, clients.Kube, []string{pod})
		}
	}
}

func runFuncsInParallel(funcs ...func()) {
	var grp sync.WaitGroup
	grp.Add(len(funcs))
	for _, f := range funcs {
		currFunc := f
		go func() {
			defer grp.Done()
			currFunc()
		}()
	}
	grp.Wait()
}
