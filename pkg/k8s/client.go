package k8s

import (
	"fmt"
	"os"
	"path/filepath"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func GetClientset() (*kubernetes.Clientset, error) {
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("не удалось определить домашнюю директорию: %w", err)
		}
		kubeconfig = filepath.Join(home, ".kube", "config")
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания конфига: %w", err)
	}

	return kubernetes.NewForConfig(config)
}

// GetNodeZone возвращает зону узла по его лейблам
func GetNodeZone(node v1.Node) string {
	zoneLabels := []string{
		"topology.kubernetes.io/zone",
		"failure-domain.beta.kubernetes.io/zone",
		"zone",
	}

	for _, label := range zoneLabels {
		if value, ok := node.Labels[label]; ok {
			return value
		}
	}

	return "<unknown>"
}
