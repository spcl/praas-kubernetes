package control_plane

import (
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"praas/pkg/types"
)

func parseData(data map[string]any, fields ...func(map[string]any) error) error {
	for _, field := range fields {
		err := field(data)
		if err != nil {
			return err
		}
	}
	return nil
}

func asID(fieldName string, target *types.IDType) func(data map[string]any) error {
	return func(data map[string]any) error {
		val, exists := data[fieldName]
		if !exists {
			return missingField(fieldName)
		}

		switch val.(type) {
		case int:
			*target = types.IDType(val.(int))
		case float64:
			*target = types.IDType(val.(float64))
		default:
			return format(fieldName)
		}

		return nil
	}
}

func asInt(fieldName string, target *int) func(data map[string]any) error {
	return func(data map[string]any) error {
		val, exists := data[fieldName]
		if !exists {
			return missingField(fieldName)
		}

		switch val.(type) {
		case int:
			*target = val.(int)
		case float64:
			*target = int(val.(float64))
		default:
			return format(fieldName)
		}

		return nil
	}
}

func asOptionalInt(fieldName string, target *int, fallback int) func(data map[string]any) error {
	return func(data map[string]any) error {
		val, exists := data[fieldName]
		if !exists {
			*target = fallback
			return nil
		}

		switch val.(type) {
		case int:
			*target = val.(int)
		case float64:
			*target = int(val.(float64))
		default:
			return format(fieldName)
		}

		return nil
	}
}

func asString(key string, target *string) func(map[string]any) error {
	return func(data map[string]any) error {
		val, exists := data[key]
		if !exists {
			return missingField(key)
		}
		str, ok := val.(string)
		if !ok {
			return fmt.Errorf("could not parse field '%s'", key)
		}
		*target = str
		return nil
	}
}

func asOptionalString(key string, target *string, fallback string) func(map[string]any) error {
	return func(data map[string]any) error {
		val, exists := data[key]
		if exists {
			var ok bool
			fallback, ok = val.(string)
			if !ok {
				return fmt.Errorf("could not parse field '%s'", key)
			}
		}
		*target = fallback
		return nil
	}
}

func asDataArray(key string, target *[]types.ProcessData) func(map[string]any) error {
	return func(data map[string]any) error {
		untyped, exists := data[key]
		if !exists {
			return missingField(key)
		}

		array, ok := untyped.([]any)
		if !ok {
			return fmt.Errorf("failed to parse array, type is: %s", reflect.TypeOf(untyped)) // format(key)
		}

		for _, untypedElem := range array {
			mapElem, ok := untypedElem.(map[string]any)
			if !ok {
				return fmt.Errorf("failed to parse element, type is: %s", reflect.TypeOf(untypedElem)) // format(key)
			}
			untypedSessionID, exists := mapElem[PIDKey]
			var sessionID string
			if !exists {
				sessionID = ""
			} else {
				var ok bool
				sessionID, ok = untypedSessionID.(string)
				if !ok {
					return fmt.Errorf("failed to parse process id, type is: %s", reflect.TypeOf(untypedSessionID))
				}
			}
			*target = append(
				*target,
				types.ProcessData{PID: sessionID},
			)
		}

		return nil
	}
}

func asStringArray(key string, target *[]string) func(map[string]any) error {
	return func(data map[string]any) error {
		untyped, exists := data[key]
		if !exists {
			return missingField(key)
		}

		array, ok := untyped.([]any)
		if !ok {
			return fmt.Errorf("failed to parse array, type is: %s", reflect.TypeOf(untyped)) // format(key)
		}

		for _, untypedElem := range array {
			elem, ok := untypedElem.(string)
			if !ok {
				return fmt.Errorf("failed to parse element, type is: %s", reflect.TypeOf(untypedElem)) // format(key)
			}
			*target = append(
				*target,
				elem,
			)
		}

		return nil
	}
}

func missingField(fieldName string) error {
	return fmt.Errorf("could not parse data, field '%s' is missing", fieldName)
}

func format(fieldName string) error {
	return fmt.Errorf("could not parse data, field '%s' is wrong format", fieldName)
}

func podRunning(p *corev1.Pod) bool {
	return p.Status.Phase == corev1.PodRunning && p.DeletionTimestamp == nil
}

// podReady checks whether pod's Ready status is True.
func podReady(p *corev1.Pod) bool {
	for _, cond := range p.Status.Conditions {
		if cond.Type == corev1.PodReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	// No ready status, probably not even running.
	return false
}

func getPodIPIfReady(p *corev1.Pod) string {
	ip := ""
	if podReady(p) && podRunning(p) {
		ip = p.Status.PodIP
	}
	return ip
}
