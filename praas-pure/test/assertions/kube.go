package assertions

import (
	"context"
	"fmt"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"praas/test"
)

func AssertScale(t *testing.T, praas test.PraasClient, pid, scale int, filter bool, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	var err error
	sleep := false
	isAtScale := false
	var currentScale int

	// Try until timeout or success
	for time.Now().Before(deadline) && !isAtScale {
		if sleep {
			time.Sleep(time.Second)
		}

		currentScale, err = praas.GetScaleForApp(pid, filter)
		if err != nil || currentScale != scale {
			sleep = true
			fmt.Printf("Expected scale: %d, current: %d\n", scale, currentScale)
		} else {
			isAtScale = true
		}
	}

	if !isAtScale {
		t.Fatalf(
			"App %d did not reach the expected scale (%d) by the given deadline (current: %d)",
			pid,
			scale,
			currentScale,
		)
	}
}

func AssertScaleUnder(t *testing.T, praas test.PraasClient, pid, scale int) {
	reqSuccess := false
	for !reqSuccess {
		currentScale, err := praas.GetScaleForApp(pid, true)
		if err == nil {
			reqSuccess = true
			if currentScale > scale {
				t.Fatalf("Expected scale to be under %d, but is %d", scale, currentScale)
			}
			return
		}
	}
}

func AssertScaleOver(t *testing.T, praas test.PraasClient, pid, scale int) {
	reqSuccess := false
	for !reqSuccess {
		currentScale, err := praas.GetScaleForApp(pid, false)
		if err == nil {
			reqSuccess = true
			if currentScale < scale {
				t.Fatalf("Expected scale to be over %d, but is %d", scale, currentScale)
			}
			return
		}
	}
}

func AssertPodsAlive(t *testing.T, kube kubernetes.Interface, livePods []string) {
	for _, podName := range livePods {
		if podName == "" {
			t.Fatal("No pod name given to AssertPodsAlive")
		}
		pod, err := kube.CoreV1().Pods("default").Get(context.Background(), podName, metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}

		if pod.DeletionTimestamp != nil || pod.Status.Phase != v1.PodRunning || pod.Status.PodIP == "" {
			t.Fatalf("Expected pod %s to be running, but it is not", podName)
		}
	}
}

func AssertTerminating(t *testing.T, kube kubernetes.Interface, podName string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)

	var pod *v1.Pod
	var err error
	sleep := false
	for time.Now().Before(deadline) {
		if sleep {
			time.Sleep(500 * time.Millisecond)
		}
		sleep = true
		pod, err = kube.CoreV1().Pods("default").Get(context.Background(), podName, metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				// It was probably already deleted
				return
			}
			continue
		}

		if pod.DeletionTimestamp == nil {
			// t.Logf("Pod %s has no deletion time", pod.Name)
			continue
		}

		// If we got to this line all checks passed, return
		return
	}

	// Now we check the last state
	if err != nil {
		t.Fatal(err)
	}

	if pod.DeletionTimestamp == nil {
		t.Fatalf("Expected pod %s to have a termination timestamp, but it is nil", pod.Name)
	}
}

func AssertPodsDeleted(t *testing.T, kube kubernetes.Interface, pods []string) {
	for _, pod := range pods {
		_, err := kube.CoreV1().Pods("default").Get(context.Background(), pod, metav1.GetOptions{})
		if !errors.IsNotFound(err) {
			if err != nil {
				t.Logf("Unexpected error: %s", err.Error())
			}
			t.Fatalf("Expected pod %s to not exist, but it does", pod)
		}
	}
}
