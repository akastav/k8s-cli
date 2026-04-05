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

var cordonForce bool
var cordonZone string

// Получение зоны узла
func getCordonNodeZone(node v1.Node) string {
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

var cordonCmd = &cobra.Command{
	Use:   "cordon [node-name]",
	Short: "Пометить узел как недоступный для планирования",
	Long:  `Помечает узел как unschedulable. Новые поды не будут размещаться на этом узле.`,
	Run: func(cmd *cobra.Command, args []string) {
		clientset, err := k8s.GetClientset()
		if err != nil {
			fmt.Printf("Ошибка подключения: %v\n", err)
			return
		}

		var nodesToCordon []v1.Node

		// Если указана зона, помечаем все узлы в зоне
		if cordonZone != "" {
			nodes, err := clientset.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
			if err != nil {
				fmt.Printf("Ошибка получения узлов: %v\n", err)
				return
			}

			for _, node := range nodes.Items {
				if getCordonNodeZone(node) == cordonZone {
					nodesToCordon = append(nodesToCordon, node)
				}
			}

			if len(nodesToCordon) == 0 {
				fmt.Printf("Узлы в зоне %s не найдены\n", cordonZone)
				return
			}
		} else {
			// Помечаем конкретный узел
			if len(args) < 1 {
				fmt.Println("Ошибка: необходимо указать имя узла или использовать флаг --zone")
				fmt.Println("Пример: ./k8s-cli cordon worker-node-1")
				fmt.Println("Пример: ./k8s-cli cordon -z us-east-1")
				return
			}

			nodeName := args[0]
			node, err := clientset.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
			if err != nil {
				fmt.Printf("Ошибка получения узла %s: %v\n", nodeName, err)
				return
			}

			nodesToCordon = append(nodesToCordon, *node)
		}

		// Подтверждение
		if !cordonForce {
			fmt.Printf("Будет помечено узлов: %d\n", len(nodesToCordon))
			for _, node := range nodesToCordon {
				fmt.Printf("  - %s (zone: %s)\n", node.Name, getCordonNodeZone(node))
			}
			fmt.Printf("\nВы уверены, что хотите пометить эти узлы как unschedulable? (yes/no): ")
			var confirm string
			fmt.Scanln(&confirm)

			if strings.ToLower(confirm) != "yes" {
				fmt.Println("Операция отменена")
				return
			}
		}

		// Помечаем узлы как unschedulable
		var cordonedCount int
		var failedCount int

		for _, node := range nodesToCordon {
			// Проверяем, не помечен ли уже узел
			if node.Spec.Unschedulable {
				fmt.Printf("⚠ Узел %s уже помечен как unschedulable\n", node.Name)
				continue
			}

			// Получаем узел заново для обновления (нужен pointer)
			currentNode, err := clientset.CoreV1().Nodes().Get(context.TODO(), node.Name, metav1.GetOptions{})
			if err != nil {
				fmt.Printf("✗ Ошибка получения узла %s: %v\n", node.Name, err)
				failedCount++
				continue
			}

			// Помечаем узел как unschedulable
			currentNode.Spec.Unschedulable = true

			_, err = clientset.CoreV1().Nodes().Update(context.TODO(), currentNode, metav1.UpdateOptions{})
			if err != nil {
				fmt.Printf("✗ Ошибка обновления узла %s: %v\n", node.Name, err)
				failedCount++
			} else {
				fmt.Printf("✓ Узел %s успешно помечен как unschedulable\n", node.Name)
				cordonedCount++
			}
		}

		fmt.Println(strings.Repeat("-", 60))
		fmt.Printf("Помечено узлов: %d\n", cordonedCount)
		if failedCount > 0 {
			fmt.Printf("Ошибок: %d\n", failedCount)
		}
	},
}

func init() {
	cordonCmd.Flags().BoolVarP(&cordonForce, "force", "f", false, "Пропустить подтверждение")
	cordonCmd.Flags().StringVarP(&cordonZone, "zone", "z", "", "Пометить все узлы в зоне")
	rootCmd.AddCommand(cordonCmd)
}
