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

		if drainZone != "" {
			nodes, err := clientset.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
			if err != nil {
				fmt.Printf("Ошибка получения узлов: %v\n", err)
				return
			}

			for _, node := range nodes.Items {
				nodeZone := k8s.GetNodeZone(node)
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
			if len(args) < 1 {
				fmt.Println("Ошибка: необходимо указать имя узла или использовать флаг --zone")
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

		for _, targetNode := range nodesToDrain {
			nodeName := targetNode.Name
			fmt.Printf("\n%s Обработка узла: %s (зона: %s)%s\n", strings.Repeat("=", 60), nodeName, k8s.GetNodeZone(targetNode), strings.Repeat("=", 60))

			// СНАЧАЛА помечаем узел как unschedulable, чтобы предотвратить race condition
			nodeForUpdate := targetNode
			nodeForUpdate.Spec.Unschedulable = true
			_, err := clientset.CoreV1().Nodes().Update(context.TODO(), &nodeForUpdate, metav1.UpdateOptions{})
			if err != nil {
				fmt.Printf("Ошибка пометки узла %s как unschedulable: %v\n", nodeName, err)
				continue
			}
			fmt.Printf("Узел %s помечен как unschedulable\n", nodeName)

			// Теперь получаем поды на узле
			pods, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{
				FieldSelector: fmt.Sprintf("spec.nodeName=%s", nodeName),
			})
			if err != nil {
				fmt.Printf("Ошибка получения подов на узле %s: %v\n", nodeName, err)
				continue
			}

			var podsToEvict []v1.Pod
			var daemonSetPods []v1.Pod
			var emptyDirPods []v1.Pod
			var terminatingPods []v1.Pod

			for _, pod := range pods.Items {
				if pod.DeletionTimestamp != nil {
					terminatingPods = append(terminatingPods, pod)
					continue
				}

				if _, ok := pod.Annotations["kubernetes.io/config.source"]; ok {
					continue
				}

				if len(pod.OwnerReferences) == 0 {
					fmt.Printf("Пропущен под %s/%s (нет владельца)\n", pod.Namespace, pod.Name)
					continue
				}

				isDaemonSet := false
				for _, owner := range pod.OwnerReferences {
					if owner.Kind == "DaemonSet" {
						isDaemonSet = true
						break
					}
				}

				if isDaemonSet {
					if drainIgnoreDaemonSets {
						continue
					}
					daemonSetPods = append(daemonSetPods, pod)
				}

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
					}
					continue
				}

				podsToEvict = append(podsToEvict, pod)
			}

			if len(daemonSetPods) > 0 && !drainIgnoreDaemonSets {
				fmt.Printf("\nНайдено %d подов DaemonSet. Используйте --ignore-daemonsets.\n", len(daemonSetPods))
			}
			if len(emptyDirPods) > 0 && !drainDeleteEmptyDir {
				fmt.Printf("Найдено %d подов с emptyDir. Используйте --delete-emptydir-data.\n", len(emptyDirPods))
			}

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

			if !drainForce {
				fmt.Printf("\nВы уверены? (yes/no): ")
				var confirm string
				fmt.Scanln(&confirm)
				if strings.ToLower(confirm) != "yes" {
					fmt.Println("Операция отменена")
					continue
				}
			}

			var evictedCount int
			var failedCount int

			allPods := append(podsToEvict, emptyDirPods...)

			for _, pod := range allPods {
				fmt.Printf("Удаление пода %s/%s... ", pod.Namespace, pod.Name)
				err := clientset.CoreV1().Pods(pod.Namespace).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})
				if err != nil {
					fmt.Printf("Ошибка: %v\n", err)
					failedCount++
				} else {
					fmt.Println("OK")
					evictedCount++
				}
			}

			for _, pod := range terminatingPods {
				fmt.Printf("Очистка Terminating пода %s/%s... ", pod.Namespace, pod.Name)
				gracePeriod := int64(0)
				err := clientset.CoreV1().Pods(pod.Namespace).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{
					GracePeriodSeconds: &gracePeriod,
				})
				if err != nil {
					fmt.Printf("Ошибка: %v\n", err)
					failedCount++
				} else {
					fmt.Println("OK")
					evictedCount++
				}
			}

			fmt.Println(strings.Repeat("-", 60))
			fmt.Printf("Узел %s: Вытеснено: %d, Ошибок: %d\n", nodeName, evictedCount, failedCount)
		}

		fmt.Printf("\nЗавершена обработка %d узла(ов)\n", len(nodesToDrain))
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
