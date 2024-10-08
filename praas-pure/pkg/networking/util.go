package networking

import "strings"

func CreateURI(base, port, endpoint string) string {
	path := base
	if !strings.HasPrefix(path, "http") {
		path = "http://" + path
	}
	if port[0] != ':' {
		path += ":"
	}
	path += port
	if endpoint[0] != '/' {
		path += "/"
	}
	path += endpoint
	return strings.TrimSpace(path)
}
