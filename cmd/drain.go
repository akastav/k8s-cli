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

var drainForce bool
var drainIgnoreDaemonSets bool
var drainDeleteEmptyDir bool
var drainTimeout int
var drainZone string

// Получение зоны узла
func getDrainNodeZone(node v1.Node) string {
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

var drainCmd = &cobra.Command{
	Use:   "drain [node-name]",
	Short: "Освободить узел для обслуживания",
	Long:  `Освобождает узел для обслуживания путем вытеснения подов и пометки узла как unschedulable.`,
	Run: func(cmd *cobra.Command, args []string) {
		clientset, err := k8s.GetClientset()
		if err != nil {
			fmt.Printf("Ошибка подключения: %v\n", err)
			return
		}

		var nodesToDrain []v1.Node

		// Если указана зона, drain все узлы в зоне
		if drainZone != "" {
			nodes, err := clientset.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
			if err != nil {
				fmt.Printf("Ошибка получения узлов: %v\n", err)
				return
			}

			for _, node := range nodes.Items {
				nodeZone := getDrainNodeZone(node)
				if nodeZone == drainZone {
					nodesToDrain = append(nodesToDrain, node)
				}
			}

			if len(nodesToDrain) == 0 {
				fmt.Printf("Узлы в зоне %s не найдены\n", drainZone)
				return
			}

			fmt.Printf("Найдено узлов в зоне %s: %d\n", drainZone, len(nodesToDrain))
		} else {
			// Drain конкретного узла
			if len(args) < 1 {
				fmt.Println("Ошибка: необходимо указать имя узла или использовать флаг --zone")
				fmt.Println("Пример: ./k8s-cli drain worker-node-1")
				fmt.Println("Пример: ./k8s-cli drain -z us-east-1")
				return
			}

			nodeName := args[0]
			node, err := clientset.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
			if err != nil {
				fmt.Printf("Ошибка получения узла %s: %v\n", nodeName, err)
				return
			}

			nodesToDrain = []v1.Node{*node}
		}

		// Обработка каждого узла
		for _, targetNode := range nodesToDrain {
			nodeName := targetNode.Name
			fmt.Printf("\n%s Обработка узла: %s (зона: %s)%s\n", strings.Repeat("=", 60), nodeName, getDrainNodeZone(targetNode), strings.Repeat("=", 60))

			// Получаем все поды на узле
			pods, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{
				FieldSelector: fmt.Sprintf("spec.nodeName=%s", nodeName),
			})
			if err != nil {
				fmt.Printf("Ошибка получения подов на узле %s: %v\n", nodeName, err)
				continue
			}

			// Фильтруем поды для удаления
			var podsToEvict []v1.Pod
			var daemonSetPods []v1.Pod
			var emptyDirPods []v1.Pod
			var terminatingPods []v1.Pod

			for _, pod := range pods.Items {
				// Собираем поды в статусе Terminating для очистки
				if pod.DeletionTimestamp != nil {
					terminatingPods = append(terminatingPods, pod)
					continue
				}

				// Пропускаем статические поды
				if _, ok := pod.Annotations["kubernetes.io/config.source"]; ok {
					continue
				}

				// Пропускаем поды без ownerReference
				if len(pod.OwnerReferences) == 0 {
					fmt.Printf("⚠ Пропущен под %s/%s (нет владельца)\n", pod.Namespace, pod.Name)
					continue
				}

				// Проверяем DaemonSet
				isDaemonSet := false
				for _, owner := range pod.OwnerReferences {
					if owner.Kind == "DaemonSet" {
						isDaemonSet = true
						break
					}
				}

				if isDaemonSet {
					if drainIgnoreDaemonSets {
						fmt.Printf("⚠ Пропущен DaemonSet под %s/%s\n", pod.Namespace, pod.Name)
						continue
					}
					daemonSetPods = append(daemonSetPods, pod)
				}

				// Проверяем emptyDir volumes
				hasEmptyDir := false
				for _, volume := range pod.Spec.Volumes {
					if volume.EmptyDir != nil {
						hasEmptyDir = true
						break
					}
				}

				if hasEmptyDir {
					if drainDeleteEmptyDir {
						emptyDirPods = append(emptyDirPods, pod)
					} else {
						fmt.Printf("⚠ Под %s/%s имеет emptyDir volume\n", pod.Namespace, pod.Name)
					}
					continue
				}

				podsToEvict = append(podsToEvict, pod)
			}

			// Предупреждения
			if len(daemonSetPods) > 0 && !drainIgnoreDaemonSets {
				fmt.Printf("\n⚠ Найдено %d подов DaemonSet. Используйте --ignore-daemonsets для пропуска.\n", len(daemonSetPods))
			}

			if len(emptyDirPods) > 0 && !drainDeleteEmptyDir {
				fmt.Printf("⚠ Найдено %d подов с emptyDir. Используйте --delete-emptydir-data для удаления.\n", len(emptyDirPods))
			}

			// Вывод списка подов
			totalPods := len(podsToEvict) + len(emptyDirPods) + len(terminatingPods)
			if totalPods > 0 {
				fmt.Println("\nПоды для обработки:")
				fmt.Println(strings.Repeat("-", 70))
				table := tablewriter.NewWriter(os.Stdout)
				table.Header([]string{"NAMESPACE", "NAME", "ACTION"})

				for _, pod := range podsToEvict {
					table.Append([]string{pod.Namespace, pod.Name, "Will be evicted"})
				}
				for _, pod := range emptyDirPods {
					table.Append([]string{pod.Namespace, pod.Name, "Will be deleted (emptyDir)"})
				}
				for _, pod := range terminatingPods {
					table.Append([]string{pod.Namespace, pod.Name, "Force delete (Terminating)"})
				}
				table.Render()
			}

			// Подтверждение
			if !drainForce {
				fmt.Printf("\nВы уверены, что хотите освободить узел %s? (yes/no): ", nodeName)
				var confirm string
				fmt.Scanln(&confirm)

				if strings.ToLower(confirm) != "yes" {
					fmt.Println("Операция отменена")
					continue
				}
			}

			// Помечаем узел как unschedulable
			targetNode.Spec.Unschedulable = true
			_, err = clientset.CoreV1().Nodes().Update(context.TODO(), &targetNode, metav1.UpdateOptions{})
			if err != nil {
				fmt.Printf("Ошибка пометки узла %s как unschedulable: %v\n", nodeName, err)
				continue
			}
			fmt.Printf("✓ Узел %s помечен как unschedulable\n", nodeName)

			// Вытеснение подов
			var evictedCount int
			var failedCount int

			allPods := append(podsToEvict, emptyDirPods...)

			for _, pod := range allPods {
				fmt.Printf("Вытеснение пода %s/%s... ", pod.Namespace, pod.Name)

				err := clientset.CoreV1().Pods(pod.Namespace).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})
				if err != nil {
					fmt.Printf("✗ Ошибка: %v\n", err)
					failedCount++
				} else {
					fmt.Println("✓")
					evictedCount++
				}
			}

			// Очистка подов в статусе Terminating
			for _, pod := range terminatingPods {
				fmt.Printf("Очистка Terminating пода %s/%s... ", pod.Namespace, pod.Name)

				// Force delete
				gracePeriod := int64(0)
				err := clientset.CoreV1().Pods(pod.Namespace).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{
					GracePeriodSeconds: &gracePeriod,
				})
				if err != nil {
					fmt.Printf("✗ Ошибка: %v\n", err)
					failedCount++
				} else {
					fmt.Println("✓")
					evictedCount++
				}
			}

			fmt.Println(strings.Repeat("-", 60))
			fmt.Printf("Узел %s: Вытеснено подов: %d, Ошибок: %d\n", nodeName, evictedCount, failedCount)
		}

		fmt.Printf("\n✓ Завершена обработка %d узла(ов)\n", len(nodesToDrain))
	},
}

func init() {
	drainCmd.Flags().BoolVarP(&drainForce, "force", "f", false, "Пропустить подтверждение")
	drainCmd.Flags().BoolVar(&drainIgnoreDaemonSets, "ignore-daemonsets", false, "Игнорировать поды DaemonSet")
	drainCmd.Flags().BoolVar(&drainDeleteEmptyDir, "delete-emptydir-data", false, "Удалять поды с emptyDir volumes")
	drainCmd.Flags().IntVar(&drainTimeout, "timeout", 300, "Таймаут ожидания удаления пода (секунды)")
	drainCmd.Flags().StringVarP(&drainZone, "zone", "z", "", "Освободить все узлы в зоне")
	rootCmd.AddCommand(drainCmd)
}
