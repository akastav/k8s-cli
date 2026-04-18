package cmd

import (
	"context"
	"fmt"
	"k8s-cli/pkg/k8s"
	"os"
	"strings"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var maintenanceZone string
var maintenanceForce bool
var maintenanceTimeout int
var maintenanceIgnoreDaemonSets bool
var maintenanceDeleteEmptyDir bool

var maintenanceStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Включить режим обслуживания",
	Long:  `Включает режим обслуживания для всех узлов в указанной зоне. Узлы помечаются как unschedulable, поды вытесняются.`,
	Run: func(cmd *cobra.Command, args []string) {
		if maintenanceZone == "" {
			fmt.Println("Ошибка: необходимо указать зону (--zone)")
			fmt.Println("Пример: ./k8s-cli maintenance start --zone=us-east-1a")
			return
		}

		clientset, err := k8s.GetClientset()
		if err != nil {
			fmt.Printf("Ошибка подключения: %v\n", err)
			return
		}

		labelSelector := fmt.Sprintf("topology.kubernetes.io/zone=%s", maintenanceZone)
		nodes, err := clientset.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{
			LabelSelector: labelSelector,
		})
		if err != nil {
			fmt.Printf("Ошибка получения узлов в зоне %s: %v\n", maintenanceZone, err)
			return
		}

		if len(nodes.Items) == 0 {
			fmt.Printf("Узлы в зоне %s не найдены\n", maintenanceZone)
			return
		}

		fmt.Printf("\nНайдено узлов в зоне %s: %d\n", maintenanceZone, len(nodes.Items))
		fmt.Println(strings.Repeat("-", 80))

		table := tablewriter.NewWriter(os.Stdout)
		table.Header([]string{"NAME", "STATUS", "UNSCHEDULABLE", "PODS"})

		for _, node := range nodes.Items {
			podCount := getNodePodCount(clientset, node.Name)
			table.Append([]string{
				node.Name,
				getNodeStatus(node),
				fmt.Sprintf("%v", node.Spec.Unschedulable),
				fmt.Sprintf("%d", podCount),
			})
		}
		table.Render()

		if !maintenanceForce {
			fmt.Printf("\nВы уверены, что хотите включить режим обслуживания для зоны %s? (yes/no): ", maintenanceZone)
			var confirm string
			fmt.Scanln(&confirm)

			if strings.ToLower(confirm) != "yes" {
				fmt.Println("Операция отменена")
				return
			}
		}

		var successCount int
		var failedCount int

		for _, node := range nodes.Items {
			fmt.Printf("\n[%s] Обработка узла...\n", node.Name)

			if !node.Spec.Unschedulable {
				node.Spec.Unschedulable = true
				_, err := clientset.CoreV1().Nodes().Update(context.TODO(), &node, metav1.UpdateOptions{})
				if err != nil {
					fmt.Printf("  Ошибка пометки узла как unschedulable: %v\n", err)
					failedCount++
					continue
				}
				fmt.Printf("  Узел помечен как unschedulable\n")
			} else {
				fmt.Printf("  Узел уже unschedulable\n")
			}

			evicted, failed := evictPodsFromNode(clientset, node.Name, maintenanceIgnoreDaemonSets, maintenanceDeleteEmptyDir, maintenanceTimeout)
			successCount += evicted
			failedCount += failed

			cleaned := cleanTerminatingPods(clientset, node.Name, maintenanceTimeout)
			fmt.Printf("  Очищено подов в статусе Terminating: %d\n", cleaned)
		}

		fmt.Println(strings.Repeat("-", 80))
		fmt.Printf("Режим обслуживания включен для зоны %s\n", maintenanceZone)
		fmt.Printf("  Вытеснено подов: %d\n", successCount)
		if failedCount > 0 {
			fmt.Printf("  Ошибок: %d\n", failedCount)
		}
	},
}

func getNodePodCount(clientset *kubernetes.Clientset, nodeName string) int {
	pods, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{
		FieldSelector: fmt.Sprintf("spec.nodeName=%s", nodeName),
	})
	if err != nil {
		return 0
	}
	return len(pods.Items)
}

func evictPodsFromNode(clientset *kubernetes.Clientset, nodeName string, ignoreDaemonSets, deleteEmptyDir bool, timeout int) (int, int) {
	pods, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{
		FieldSelector: fmt.Sprintf("spec.nodeName=%s", nodeName),
	})
	if err != nil {
		fmt.Printf("    Ошибка получения подов: %v\n", err)
		return 0, 1
	}

	var evictedCount int
	var failedCount int

	for _, pod := range pods.Items {
		if _, ok := pod.Annotations["kubernetes.io/config.source"]; ok {
			continue
		}

		if len(pod.OwnerReferences) == 0 {
			continue
		}

		isDaemonSet := false
		for _, owner := range pod.OwnerReferences {
			if owner.Kind == "DaemonSet" {
				isDaemonSet = true
				break
			}
		}

		if isDaemonSet && ignoreDaemonSets {
			continue
		}

		hasEmptyDir := false
		for _, volume := range pod.Spec.Volumes {
			if volume.EmptyDir != nil {
				hasEmptyDir = true
				break
			}
		}

		if hasEmptyDir && !deleteEmptyDir {
			continue
		}

		err := clientset.CoreV1().Pods(pod.Namespace).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})
		if err != nil {
			fmt.Printf("    Ошибка удаления пода %s/%s: %v\n", pod.Namespace, pod.Name, err)
			failedCount++
		} else {
			fmt.Printf("    Вытеснен под %s/%s\n", pod.Namespace, pod.Name)
			evictedCount++
		}
	}

	return evictedCount, failedCount
}

// cleanTerminatingPods ожидает завершения подов в статусе Terminating
// Если под не завершается в течение timeout, выполняется force delete
func cleanTerminatingPods(clientset *kubernetes.Clientset, nodeName string, timeout int) int {
	pods, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{
		FieldSelector: fmt.Sprintf("spec.nodeName=%s", nodeName),
	})
	if err != nil {
		return 0
	}

	var cleanedCount int

	for _, pod := range pods.Items {
		if pod.DeletionTimestamp == nil {
			continue
		}

		fmt.Printf("    Ожидание завершения пода %s/%s...\n", pod.Namespace, pod.Name)

		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
		defer cancel()

		// Используем ticker для периодической проверки состояния
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				// Таймаут - force delete
				gracePeriod := int64(0)
				err := clientset.CoreV1().Pods(pod.Namespace).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{
					GracePeriodSeconds: &gracePeriod,
				})
				if err == nil {
					fmt.Printf("    Принудительно удален под %s/%s\n", pod.Namespace, pod.Name)
					cleanedCount++
				}
				return cleanedCount
			case <-ticker.C:
				// Проверяем, удалился ли под
				p, err := clientset.CoreV1().Pods(pod.Namespace).Get(context.TODO(), pod.Name, metav1.GetOptions{})
				if err != nil {
					// Под успешно удалён
					fmt.Printf("    Под %s/%s завершился\n", pod.Namespace, pod.Name)
					cleanedCount++
					return cleanedCount
				}
				if p.DeletionTimestamp == nil {
					// Под больше не в статусе Terminating
					fmt.Printf("    Под %s/%s завершился\n", pod.Namespace, pod.Name)
					cleanedCount++
					return cleanedCount
				}
			}
		}
	}

	return cleanedCount
}

func int64ptr(i int64) *int64 {
	return &i
}

func init() {
	maintenanceStartCmd.Flags().StringVarP(&maintenanceZone, "zone", "z", "", "Зона обслуживания (обязательно)")
	maintenanceStartCmd.Flags().BoolVarP(&maintenanceForce, "force", "f", false, "Пропустить подтверждение")
	maintenanceStartCmd.Flags().IntVar(&maintenanceTimeout, "timeout", 300, "Таймаут ожидания (секунды)")
	maintenanceStartCmd.Flags().BoolVar(&maintenanceIgnoreDaemonSets, "ignore-daemonsets", false, "Игнорировать поды DaemonSet")
	maintenanceStartCmd.Flags().BoolVar(&maintenanceDeleteEmptyDir, "delete-emptydir-data", false, "Удалять поды с emptyDir volumes")
	maintenanceCmd.AddCommand(maintenanceStartCmd)
}
