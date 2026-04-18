package cmd

import (
	"context"
	"fmt"
	"k8s-cli/pkg/k8s"
	"os"
	"strings"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var stopMaintenanceZone string
var stopMaintenanceForce bool

var maintenanceStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Выключить режим обслуживания",
	Long:  `Выключает режим обслуживания для всех узлов в указанной зоне.`,
	Run: func(cmd *cobra.Command, args []string) {
		if stopMaintenanceZone == "" {
			fmt.Println("Ошибка: необходимо указать зону (--zone)")
			return
		}

		clientset, err := k8s.GetClientset()
		if err != nil {
			fmt.Printf("Ошибка подключения: %v\n", err)
			return
		}

		labelSelector := fmt.Sprintf("topology.kubernetes.io/zone=%s", stopMaintenanceZone)
		nodes, err := clientset.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{
			LabelSelector: labelSelector,
		})
		if err != nil {
			fmt.Printf("Ошибка получения узлов в зоне %s: %v\n", stopMaintenanceZone, err)
			return
		}

		if len(nodes.Items) == 0 {
			fmt.Printf("Узлы в зоне %s не найдены\n", stopMaintenanceZone)
			return
		}

		fmt.Printf("\nНайдено узлов в зоне %s: %d\n", stopMaintenanceZone, len(nodes.Items))
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

		if !stopMaintenanceForce {
			fmt.Printf("\nВы уверены, что хотите выключить режим обслуживания для зоны %s? (yes/no): ", stopMaintenanceZone)
			var confirm string
			fmt.Scanln(&confirm)
			if strings.ToLower(confirm) != "yes" {
				fmt.Println("Операция отменена")
				return
			}
		}

		var successCount int
		var failedCount int
		var alreadyReadyCount int

		for _, node := range nodes.Items {
			fmt.Printf("\n[%s] Возврат узла в нормальный режим...\n", node.Name)

			if !node.Spec.Unschedulable {
				fmt.Printf("  Узел уже в нормальном режиме\n")
				alreadyReadyCount++
				continue
			}

			node.Spec.Unschedulable = false
			_, err := clientset.CoreV1().Nodes().Update(context.TODO(), &node, metav1.UpdateOptions{})
			if err != nil {
				fmt.Printf("  Ошибка возврата узла: %v\n", err)
				failedCount++
				continue
			}

			fmt.Printf("  Узел возвращен в нормальный режим (schedulable)\n")
			successCount++
		}

		fmt.Println(strings.Repeat("-", 80))
		fmt.Printf("Режим обслуживания выключен для зоны %s\n", stopMaintenanceZone)
		fmt.Printf("  Возвращено узлов: %d\n", successCount)
		fmt.Printf("  Уже в нормальном режиме: %d\n", alreadyReadyCount)
		if failedCount > 0 {
			fmt.Printf("  Ошибок: %d\n", failedCount)
		}
	},
}

func init() {
	maintenanceStopCmd.Flags().StringVarP(&stopMaintenanceZone, "zone", "z", "", "Зона обслуживания (обязательно)")
	maintenanceStopCmd.Flags().BoolVarP(&stopMaintenanceForce, "force", "f", false, "Пропустить подтверждение")
	maintenanceCmd.AddCommand(maintenanceStopCmd)
}
