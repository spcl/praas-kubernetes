package e2e

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	scaleUpTimeout   = time.Minute
	scaleDownTimeout = 2 * time.Minute
)

var (
	KubeConfig  *rest.Config
	kubeCfgPath = flag.String("kubecfg", filepath.Join(os.Getenv("HOME"), ".kube", "config"), "Kubernetes config path")
)

func TestMain(m *testing.M) {
	flag.Parse()
	ensureDeploymentIsReady()
	code := m.Run()
	os.Exit(code)
}

func ensureDeploymentIsReady() {
	// TODO(gr): check if kubernetes and praas is deployed and accessible

	cfg, err := clientcmd.BuildConfigFromFlags("", *kubeCfgPath)
	if err != nil {
		panic(err)
	}
	KubeConfig = cfg
}

func GetKubeConfig() *rest.Config {
	return KubeConfig
}
