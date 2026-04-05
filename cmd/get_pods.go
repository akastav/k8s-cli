package cmd

import (
	"context"
	"fmt"
	"k8s-cli/pkg/k8s"
	"os"
	"strings"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var namespace string
var statusFilter string
var allNamespaces bool

// Проверка соответствия статуса фильтру (с поддержкой инверсии через not:)
func matchStatus(podStatus, filter string) bool {
	if filter == "" {
		return true
	}

	// Проверка на инверсию
	invert := strings.HasPrefix(filter, "not:")
	if invert {
		filter = strings.TrimPrefix(filter, "not:")
	}

	match := strings.EqualFold(podStatus, filter)

	if invert {
		return !match
	}
	return match
}

var getPodsCmd = &cobra.Command{
	Use:   "pods",
	Short: "Получить список подов",
	Run: func(cmd *cobra.Command, args []string) {
		clientset, err := k8s.GetClientset()
		if err != nil {
			fmt.Printf("Ошибка подключения: %v\n", err)
			return
		}

		var pods *v1.PodList

		// Если поиск по всем namespace
		if allNamespaces {
			pods, err = clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
		} else {
			pods, err = clientset.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{})
		}

		if err != nil {
			fmt.Printf("Ошибка получения подов: %v\n", err)
			return
		}

		// Фильтрация по статусу
		var filteredPods []v1.Pod

		for _, pod := range pods.Items {
			podStatus := string(pod.Status.Phase)

			if matchStatus(podStatus, statusFilter) {
				filteredPods = append(filteredPods, pod)
			}
		}

		if len(filteredPods) == 0 {
			fmt.Println("Поды не найдены")
			return
		}

		// Вывод результатов
		table := tablewriter.NewWriter(os.Stdout)
		table.Header([]string{"NAMESPACE", "NAME", "STATUS", "IP", "NODE"})

		for _, pod := range filteredPods {
			table.Append([]string{
				pod.Namespace,
				pod.Name,
				string(pod.Status.Phase),
				pod.Status.PodIP,
				pod.Spec.NodeName,
			})
		}
		table.Render()

		// Информация о фильтре
		filterInfo := ""
		if statusFilter != "" {
			if strings.HasPrefix(statusFilter, "not:") {
				filterInfo = fmt.Sprintf(" (фильтр: все кроме %s)", strings.TrimPrefix(statusFilter, "not:"))
			} else {
				filterInfo = fmt.Sprintf(" (фильтр: %s)", statusFilter)
			}
		}

		fmt.Printf("\nВсего подов: %d (отфильтровано: %d)%s\n", len(pods.Items), len(filteredPods), filterInfo)
	},
}

func init() {
	getPodsCmd.Flags().StringVarP(&namespace, "namespace", "n", "default", "Namespace")
	getPodsCmd.Flags().StringVarP(&statusFilter, "status", "s", "", "Фильтр по статусу (используйте not: для инверсии, например not:Running)")
	getPodsCmd.Flags().BoolVarP(&allNamespaces, "all-namespaces", "A", false, "Поиск по всем namespace")
	getCmd.AddCommand(getPodsCmd)
}
