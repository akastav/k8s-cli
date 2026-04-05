package cmd

import (
	"context"
	"fmt"
	"k8s-cli/pkg/k8s"
	"strings"

	"github.com/spf13/cobra"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var deleteNamespace string
var deleteStatusFilter string
var deleteAllNamespaces bool
var deleteForce bool

// Проверка соответствия статуса фильтру (с поддержкой инверсии через not:)
func matchDeleteStatus(podStatus, filter string) bool {
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

var deletePodsCmd = &cobra.Command{
	Use:   "pods",
	Short: "Удалить поды по статусу",
	Long:  `Удаление подов с указанием фильтра по статусу. Требует подтверждения.`,
	Run: func(cmd *cobra.Command, args []string) {
		clientset, err := k8s.GetClientset()
		if err != nil {
			fmt.Printf("Ошибка подключения: %v\n", err)
			return
		}

		// Проверка: должен быть указан фильтр статуса
		if deleteStatusFilter == "" {
			fmt.Println("Ошибка: необходимо указать фильтр статуса (--status или -s)")
			fmt.Println("Пример: ./k8s-cli delete pods -s Failed")
			fmt.Println("Пример: ./k8s-cli delete pods -s not:Running -A")
			return
		}

		var pods *v1.PodList

		// Если поиск по всем namespace
		if deleteAllNamespaces {
			pods, err = clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
		} else {
			pods, err = clientset.CoreV1().Pods(deleteNamespace).List(context.TODO(), metav1.ListOptions{})
		}

		if err != nil {
			fmt.Printf("Ошибка получения подов: %v\n", err)
			return
		}

		// Фильтрация по статусу
		var podsToDelete []v1.Pod

		for _, pod := range pods.Items {
			podStatus := string(pod.Status.Phase)

			if matchDeleteStatus(podStatus, deleteStatusFilter) {
				podsToDelete = append(podsToDelete, pod)
			}
		}

		if len(podsToDelete) == 0 {
			fmt.Println("Поды для удаления не найдены")
			return
		}

		// Вывод списка подов на удаление
		fmt.Println("\nПоды, предназначенные для удаления:")
		fmt.Println(strings.Repeat("-", 80))
		fmt.Printf("%-30s %-20s %-15s\n", "NAME", "NAMESPACE", "STATUS")
		fmt.Println(strings.Repeat("-", 80))

		for _, pod := range podsToDelete {
			fmt.Printf("%-30s %-20s %-15s\n", pod.Name, pod.Namespace, string(pod.Status.Phase))
		}
		fmt.Println(strings.Repeat("-", 80))
		fmt.Printf("\nВсего подов для удаления: %d\n", len(podsToDelete))

		// Подтверждение удаления
		if !deleteForce {
			fmt.Print("\nВы уверены, что хотите удалить эти поды? (yes/no): ")
			var confirm string
			fmt.Scanln(&confirm)

			if strings.ToLower(confirm) != "yes" {
				fmt.Println("Удаление отменено")
				return
			}
		}

		// Удаление подов
		var deletedCount int
		var failedCount int

		for _, pod := range podsToDelete {
			err := clientset.CoreV1().Pods(pod.Namespace).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})
			if err != nil {
				fmt.Printf("Ошибка удаления пода %s/%s: %v\n", pod.Namespace, pod.Name, err)
				failedCount++
			} else {
				fmt.Printf("✓ Удален под: %s/%s\n", pod.Namespace, pod.Name)
				deletedCount++
			}
		}

		fmt.Println(strings.Repeat("-", 80))
		fmt.Printf("Удалено подов: %d\n", deletedCount)
		if failedCount > 0 {
			fmt.Printf("Ошибок при удалении: %d\n", failedCount)
		}
	},
}

func init() {
	deletePodsCmd.Flags().StringVarP(&deleteNamespace, "namespace", "n", "default", "Namespace")
	deletePodsCmd.Flags().StringVarP(&deleteStatusFilter, "status", "s", "", "Фильтр по статусу (обязательно, используйте not: для инверсии)")
	deletePodsCmd.Flags().BoolVarP(&deleteAllNamespaces, "all-namespaces", "A", false, "Поиск по всем namespace")
	deletePodsCmd.Flags().BoolVarP(&deleteForce, "force", "f", false, "Пропустить подтверждение удаления")
	deleteCmd.AddCommand(deletePodsCmd)
}
