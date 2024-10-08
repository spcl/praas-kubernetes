package test

import (
	"context"
	"fmt"
	"reflect"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// GetIdx returns the index of elem in slice arr, or -1 if elem is not found
func GetIdx[T comparable](arr []T, elem T) int {
	for idx, arrElem := range arr {
		if arrElem == elem {
			return idx
		}
	}

	return -1
}

func GetField[T any](response Response, fieldName string) (T, error) {
	var empty T
	untyped, exists := response[fieldName]
	if !exists {
		return empty, ResponseError(Missing(fieldName), response)
	}

	typed, ok := untyped.(T)
	if !ok {
		return empty, ResponseError(Format(fieldName), response)
	}

	return typed, nil
}

func GetDataField[T any](response Response, fieldName string) (T, error) {
	var empty T
	untypedData, exists := response["data"]
	if !exists {
		return empty, ResponseError(Missing("data"), response)
	}
	data, ok := untypedData.(map[string]any)
	if !ok {
		return empty, ResponseError(Format("data"), response)
	}

	untyped, exists := data[fieldName]
	if !exists {
		return empty, ResponseError(MissingData(fieldName), data)
	}

	typed, ok := untyped.(T)
	if !ok {
		return empty, ResponseError(Format(fieldName), data)
	}

	return typed, nil
}

func GetArrayDataField[T any](response Response, containingField, fieldName string) ([]T, error) {
	untypedData, exists := response["data"]
	if !exists {
		return nil, ResponseError(Missing("data"), response)
	}
	data, ok := untypedData.(map[string]any)
	if !ok {
		fmt.Println(reflect.TypeOf(untypedData))
		return nil, ResponseError(Format("data"), response)
	}

	untypedOuter, exists := data[containingField]
	if !exists {
		return nil, ResponseError(Missing(containingField), response)
	}
	outer, ok := untypedOuter.([]any)
	if !ok {
		return nil, ResponseError(Format(containingField), response)
	}

	var values []T
	for _, untypedEntry := range outer {
		entry, ok := untypedEntry.(map[string]any)
		if !ok {
			fmt.Println(reflect.TypeOf(untypedEntry))
			return nil, ResponseError(Format("data:entry"), response)
		}

		untypedValue, exists := entry[fieldName]
		if !exists {
			return nil, ResponseError(Missing(fieldName), response)
		}

		value, ok := untypedValue.(T)
		if !ok {
			return nil, ResponseError(Format(fieldName), response)
		}
		values = append(values, value)
	}

	return values, nil
}

func ResponseError(reason string, response Response) error {
	return fmt.Errorf("unexpected response: %s. Response: %v", reason, response)
}

func Missing(fieldName string) string {
	return fmt.Sprintf("Missing '%s' field", fieldName)
}

func MissingData(fieldName string) string {
	return fmt.Sprintf("Missing field from data: '%s'", fieldName)
}

func Format(fieldName string) string {
	return fmt.Sprintf("field %s incorrect format", fieldName)
}

func CreateAppWithProcesses(
	clients *Clients,
	numSessions int,
	sessionIDs *[]string,
	podToPID map[string]string,
) (int, error) {
	process, err := clients.Praas.CreateApp()
	if err != nil {
		return -1, err
	}

	var id string
	var pod string
	for i := 0; i < numSessions; i++ {
		id, pod, err = clients.Praas.CreateProcess(process)
		if err != nil {
			_ = clients.Praas.DeleteApp(process)
			return -1, err
		}
		if sessionIDs != nil {
			*sessionIDs = append(*sessionIDs, id)
		}
		if podToPID != nil {
			podToPID[pod] = id
		}
	}

	// Make sure all pods are up and running
	isAtScale := false
	for !isAtScale {
		scale, err := clients.Praas.GetScaleForApp(process, true)
		if err == nil {
			isAtScale = scale == numSessions
		}
		if err != nil || !isAtScale {
			time.Sleep(time.Second)
		}
	}

	return process, nil
}

func MarkDeletePod(proxy ProxyClient, podName string) error {
	fmt.Println("Delete pod: ", podName)
	result, err := proxy.SendRequest(podName, "/delete")
	if err != nil {
		return err
	}

	if result != "OK " {
		return fmt.Errorf("delete pod %s returned unexpected result: %s", podName, result)
	}

	return nil
}

func IsPodAlive(kube kubernetes.Interface, podName string) bool {
	pod, err := kube.CoreV1().Pods("default").Get(context.Background(), podName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return false
		}
		panic(err)
	}

	if pod.DeletionTimestamp != nil || pod.Status.Phase != v1.PodRunning || pod.Status.PodIP == "" {
		return false
	}

	return true
}
