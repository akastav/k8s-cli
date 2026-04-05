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

var uncordonForce bool
var uncordonZone string

// Получение зоны узла
func getUncordonNodeZone(node v1.Node) string {
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

var uncordonCmd = &cobra.Command{
	Use:   "uncordon [node-name]",
	Short: "Пометить узел как доступный для планирования",
	Long:  `Помечает узел как schedulable. Новые поды смогут размещаться на этом узле.`,
	Run: func(cmd *cobra.Command, args []string) {
		clientset, err := k8s.GetClientset()
		if err != nil {
			fmt.Printf("Ошибка подключения: %v\n", err)
			return
		}

		var nodesToUncordon []v1.Node

		// Если указана зона, помечаем все узлы в зоне
		if uncordonZone != "" {
			nodes, err := clientset.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
			if err != nil {
				fmt.Printf("Ошибка получения узлов: %v\n", err)
				return
			}

			for _, node := range nodes.Items {
				if getUncordonNodeZone(node) == uncordonZone {
					nodesToUncordon = append(nodesToUncordon, node)
				}
			}

			if len(nodesToUncordon) == 0 {
				fmt.Printf("Узлы в зоне %s не найдены\n", uncordonZone)
				return
			}
		} else {
			// Помечаем конкретный узел
			if len(args) < 1 {
				fmt.Println("Ошибка: необходимо указать имя узла или использовать флаг --zone")
				fmt.Println("Пример: ./k8s-cli uncordon worker-node-1")
				fmt.Println("Пример: ./k8s-cli uncordon -z us-east-1")
				return
			}

			nodeName := args[0]
			node, err := clientset.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
			if err != nil {
				fmt.Printf("Ошибка получения узла %s: %v\n", nodeName, err)
				return
			}

			nodesToUncordon = append(nodesToUncordon, *node)
		}

		// Подтверждение
		if !uncordonForce {
			fmt.Printf("Будет помечено узлов: %d\n", len(nodesToUncordon))
			for _, node := range nodesToUncordon {
				fmt.Printf("  - %s (zone: %s)\n", node.Name, getUncordonNodeZone(node))
			}
			fmt.Printf("\nВы уверены, что хотите пометить эти узлы как schedulable? (yes/no): ")
			var confirm string
			fmt.Scanln(&confirm)

			if strings.ToLower(confirm) != "yes" {
				fmt.Println("Операция отменена")
				return
			}
		}

		// Помечаем узлы как schedulable
		var uncordonedCount int
		var failedCount int

		for _, node := range nodesToUncordon {
			// Проверяем, не помечен ли уже узел как schedulable
			if !node.Spec.Unschedulable {
				fmt.Printf("⚠ Узел %s уже доступен для планирования\n", node.Name)
				continue
			}

			// Получаем узел заново для обновления (нужен pointer)
			currentNode, err := clientset.CoreV1().Nodes().Get(context.TODO(), node.Name, metav1.GetOptions{})
			if err != nil {
				fmt.Printf("✗ Ошибка получения узла %s: %v\n", node.Name, err)
				failedCount++
				continue
			}

			// Помечаем узел как schedulable
			currentNode.Spec.Unschedulable = false

			_, err = clientset.CoreV1().Nodes().Update(context.TODO(), currentNode, metav1.UpdateOptions{})
			if err != nil {
				fmt.Printf("✗ Ошибка обновления узла %s: %v\n", node.Name, err)
				failedCount++
			} else {
				fmt.Printf("✓ Узел %s успешно помечен как schedulable\n", node.Name)
				uncordonedCount++
			}
		}

		fmt.Println(strings.Repeat("-", 60))
		fmt.Printf("Помечено узлов: %d\n", uncordonedCount)
		if failedCount > 0 {
			fmt.Printf("Ошибок: %d\n", failedCount)
		}
	},
}

func init() {
	uncordonCmd.Flags().BoolVarP(&uncordonForce, "force", "f", false, "Пропустить подтверждение")
	uncordonCmd.Flags().StringVarP(&uncordonZone, "zone", "z", "", "Пометить все узлы в зоне как schedulable")
	rootCmd.AddCommand(uncordonCmd)
}
