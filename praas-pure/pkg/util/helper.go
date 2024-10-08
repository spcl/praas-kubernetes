package util

func RemoveElem[T any](arr []T, idx int) []T {
	// Overwrite with last element and then reduce size
	end := len(arr) - 1
	arr[idx] = arr[end]
	return arr[:end]
}
