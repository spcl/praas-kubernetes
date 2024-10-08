package e2e

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"praas/test"
	"praas/test/assertions"
)

func TestProcessRequests(t *testing.T) {
	var knownPIDs []int
	// Setup
	clients := test.NewTestClients(GetKubeConfig())
	cases := []struct {
		name          string
		method        string
		body          string
		shouldSucceed bool
	}{
		{"valid creation", http.MethodPost, "{}", true},
		{"invalid HTTP method", http.MethodPut, "{}", false},
		{"valid creation", http.MethodPost, "{}", true},

		{"valid deletion request body", http.MethodDelete, `{"app-id":%d}`, true},
		{"invalid deletion with no data", http.MethodDelete, `{}`, false},
		{"invalid deletion with unknown pid", http.MethodDelete, `{"app-id":80083}`, false},
		{"invalid deletion with unknown field", http.MethodDelete, `{"app-id":%d,"foo":"bar"}`, false},
		{"invalid deletion with malformed json", http.MethodDelete, `{"app-id"%d}`, false},
		{"invalid deletion with bad pid format", http.MethodDelete, `{"app-id":"bar"}`, false},
		{"valid deletion request body", http.MethodDelete, `{"app-id":%d}`, true},
	}

	// Execute test cases
	for _, tCase := range cases {
		t.Run(
			tCase.name, func(t *testing.T) {
				pidUsed := 123456
				if len(knownPIDs) > 0 {
					pidUsed = knownPIDs[0]
				}
				body := fmt.Sprintf(tCase.body, pidUsed)
				req := clients.HTTPPraas.App()
				req.Method = tCase.method
				response, err := req.Do(body)
				if err != nil {
					t.Fatal(err)
				}

				assertions.AssertRequestSuccess(t, response, tCase.shouldSucceed)
				if tCase.method != http.MethodDelete {
					// Validate and save ID if it was a successful request
					pid := assertions.AssertPIDFieldValid(t, response)
					if tCase.shouldSucceed && pid != -1 {
						knownPIDs = append(knownPIDs, pid)
					}
				} else {
					if tCase.shouldSucceed {
						// Make sure there are no kubernetes resources for this process
						assertions.AssertScale(t, clients.Praas, pidUsed, 0, true, time.Minute)
						if pidUsed == knownPIDs[0] {
							// Remove ID from known pids
							knownPIDs = knownPIDs[1:]
						}
					}
				}
			},
		)
	}
	for _, pid := range knownPIDs {
		err := clients.Praas.DeleteApp(pid)
		if err != nil {
			t.Logf("Error while cleaning up: %s", err.Error())
		}
	}
}

func TestSessionRequests(t *testing.T) {
	// Create a process
	clients := test.NewTestClients(GetKubeConfig())
	pid, err := clients.Praas.CreateApp()
	if err != nil {
		t.Fatalf("Failed to create process: %s", err.Error())
	}

	// Sub-tests
	cases := []struct {
		name          string
		body          string
		expectSuccess bool
		numSessions   int
	}{
		{"1 valid request", `{"app-id":%d,"processes":[{}]}`, true, 1},
		{"2 valid requests", `{"app-id":%d,"processes":[{},{}]}`, true, 2},
		{"invalid request with extra fields", `{"app-id":%d,"processes":[{}], "foo":"bar"}`, false, 0},
		{"invalid request with bad app-id", `{"app-id":"bar","processes":[{}]}`, false, 0},
		{"invalid request with invalid app-id", `{"app-id":-1,"processes":[{}]}`, false, 0},
		{"invalid request with unknown app-id", `{"app-id":80038,"processes":[{}]}`, false, 0},
		{"invalid request with missing app-id", `{"processes":[{}]}`, false, 0},
		{"invalid request with missing processes", `{"app-id":%d}`, false, 0},
		{"invalid request with bad pid", `{"app-id":%d,"processes":[{"pid": 80039}]}`, false, 0},
		{"invalid request with non-existent pid", `{"app-id":%d,"processes":[{"pid": "asd"}]}`, false, 0},
	}

	// Execute test cases
	var sessionIDs []string
	for _, tCase := range cases {
		t.Run(
			tCase.name, func(t *testing.T) {
				scale, _ := clients.Praas.GetScaleForApp(pid, true)
				scale += tCase.numSessions

				realBody := fmt.Sprintf(tCase.body, pid)
				response, err := clients.HTTPPraas.Process().Create().Do(realBody)
				if err != nil {
					t.Fatal(err)
				}

				// Ensure request state is as expected
				assertions.AssertRequestSuccess(t, response, tCase.expectSuccess)
				assertions.AssertScale(t, clients.Praas, pid, scale, true, scaleUpTimeout)

				// If success make sure pod state is as expected
				if tCase.expectSuccess {
					assertions.AssertSessionID(t, response, &sessionIDs)
				}
			},
		)
	}

	// Clean up
	err = clients.Praas.DeleteApp(pid)
	if err != nil {
		t.Fatalf("Failed to delete process: %s", err.Error())
	}
}
