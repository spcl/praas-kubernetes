package assertions

import (
	"testing"

	"praas/test"
)

func AssertRequestSuccess(t *testing.T, response test.Response, shouldSucceed bool) {
	didSucceed := getDidSucceed(t, response)

	if shouldSucceed {
		if !didSucceed {
			t.Fatalf("Expected request to succeed, but it failed (%v)", response)
		}
	} else {
		if didSucceed {
			t.Fatalf("Expected request to fail, but it succeeded (%v)", response)
		}
	}
}

func AssertFailWithReason(t *testing.T, response test.Response, reason string) {
	didSucceed := getDidSucceed(t, response)
	if didSucceed {
		t.Fatalf("Expected request to fail, but it succeeded (%v)", response)
	}

	respReason, err := test.GetField[string](response, "reason")
	if err != nil {
		t.Fatal(err)
	}

	if respReason != reason {
		t.Fatalf("Expected request to fail with reason: '%s', but reason was: '%s'", reason, respReason)
	}
}

func AssertPIDFieldValid(t *testing.T, response test.Response) int {
	didSucceed := getDidSucceed(t, response)
	// We assume that if this failed, but should have succeeded it's already reported
	if !didSucceed {
		if pid, exists := response["app-id"]; exists {
			t.Fatalf("Request failed, but response still contains a pid field (%v)", pid)
		}
		return -1
	}

	// Get ID checks the existence and validity
	return getPID(t, response)
}

func AssertSessionID(t *testing.T, response test.Response, sessionIDs *[]string) {
	ids := getSessionIDs(t, response)
	AssertUniqueSessionID(t, sessionIDs, ids)
}

func AssertUniqueSessionID(t *testing.T, PIDs *[]string, ids []string) {
	// Make sure it's not empty
	if ids == nil {
		t.Fatal("App ID list is empty")
	}

	for _, id := range ids {
		// Make sure this id is unique
		if test.GetIdx(*PIDs, id) != -1 {
			t.Fatalf("Process ID: %s is not unique", id)
		}
		// Update session IDs seen so far
		*PIDs = append(*PIDs, id)
	}
}

/*****************************
 ********** HELPERS **********
 *****************************/

func getSessionIDs(t *testing.T, response test.Response) []string {
	ids, err := test.GetArrayDataField[string](response, "processes", "pid")
	if err != nil {
		t.Fatal(err)
	}
	return ids
}

func getDidSucceed(t *testing.T, response test.Response) bool {
	result, err := test.GetField[bool](response, "success")
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func getPID(t *testing.T, response test.Response) int {
	// First get float (json unmarshalls all numbers into float64)
	floatPID, err := test.GetDataField[float64](response, "app-id")
	if err != nil {
		t.Fatal(err)
	}
	// Make sure it is actually an integer
	intPID := int(floatPID)
	if floatPID != float64(intPID) {
		t.Fatal(test.ResponseError(test.Format("app-id"), response))
	}
	return intPID
}
