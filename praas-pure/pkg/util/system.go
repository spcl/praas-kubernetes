package util

import "os"

func Namespace() string {
	return os.Getenv("SYSTEM_NAMESPACE")
}
