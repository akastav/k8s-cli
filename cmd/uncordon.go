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

		if uncordonZone != "" {
			nodes, err := clientset.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
			if err != nil {
				fmt.Printf("Ошибка получения узлов: %v\n", err)
				return
			}

			for _, node := range nodes.Items {
				if k8s.GetNodeZone(node) == uncordonZone {
					nodesToUncordon = append(nodesToUncordon, node)
				}
			}

			if len(nodesToUncordon) == 0 {
				fmt.Printf("Узлы в зоне %s не найдены\n", uncordonZone)
				return
			}
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
			nodesToUncordon = append(nodesToUncordon, *node)
		}

		if !uncordonForce {
			fmt.Printf("Будет помечено узлов: %d\n", len(nodesToUncordon))
			for _, node := range nodesToUncordon {
				fmt.Printf("  - %s (zone: %s)\n", node.Name, k8s.GetNodeZone(node))
			}
			fmt.Printf("\nВы уверены? (yes/no): ")
			var confirm string
			fmt.Scanln(&confirm)
			if strings.ToLower(confirm) != "yes" {
				fmt.Println("Операция отменена")
				return
			}
		}

		var uncordonedCount int
		var failedCount int

		for _, node := range nodesToUncordon {
			if !node.Spec.Unschedulable {
				fmt.Printf("Узел %s уже доступен для планирования\n", node.Name)
				continue
			}

			currentNode, err := clientset.CoreV1().Nodes().Get(context.TODO(), node.Name, metav1.GetOptions{})
			if err != nil {
				fmt.Printf("Ошибка получения узла %s: %v\n", node.Name, err)
				failedCount++
				continue
			}

			currentNode.Spec.Unschedulable = false
			_, err = clientset.CoreV1().Nodes().Update(context.TODO(), currentNode, metav1.UpdateOptions{})
			if err != nil {
				fmt.Printf("Ошибка обновления узла %s: %v\n", node.Name, err)
				failedCount++
			} else {
				fmt.Printf("Узел %s помечен как schedulable\n", node.Name)
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
	uncordonCmd.Flags().StringVarP(&uncordonZone, "zone", "z", "", "Пометить все узлы в зоне")
	rootCmd.AddCommand(uncordonCmd)
}
