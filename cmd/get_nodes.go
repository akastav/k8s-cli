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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var nodeStatusFilter string
var nodeLabelSelector string
var nodeZoneFilter string

// Проверка соответствия статуса узла фильтру
func matchNodeStatus(node v1.Node, filter string) bool {
	if filter == "" {
		return true
	}

	// Получаем статус узла
	nodeStatus := getNodeStatus(node)

	// Проверка на инверсию
	invert := strings.HasPrefix(filter, "not:")
	if invert {
		filter = strings.TrimPrefix(filter, "not:")
	}

	match := strings.EqualFold(nodeStatus, filter)

	if invert {
		return !match
	}
	return match
}

// Получение статуса узла
func getNodeStatus(node v1.Node) string {
	for _, condition := range node.Status.Conditions {
		if condition.Type == v1.NodeReady {
			if condition.Status == v1.ConditionTrue {
				return "Ready"
			}
			return "NotReady"
		}
	}
	return "Unknown"
}

// Получение ролей узла
func getNodeRoles(node v1.Node) string {
	var roles []string

	for key := range node.Labels {
		if strings.HasPrefix(key, "node-role.kubernetes.io/") {
			role := strings.TrimPrefix(key, "node-role.kubernetes.io/")
			if role == "" {
				role = strings.TrimSuffix(key, "/")
				role = strings.TrimPrefix(role, "node-role.kubernetes.io/")
			}
			roles = append(roles, role)
		}
	}

	if len(roles) == 0 {
		return "<none>"
	}

	return strings.Join(roles, ",")
}

// Получение возраста узла
func getNodeAge(creationTime metav1.Time) string {
	duration := time.Since(creationTime.Time)

	if duration < time.Minute {
		return fmt.Sprintf("%ds", int(duration.Seconds()))
	} else if duration < time.Hour {
		return fmt.Sprintf("%dm", int(duration.Minutes()))
	} else if duration < 24*time.Hour {
		return fmt.Sprintf("%dh", int(duration.Hours()))
	} else if duration < 30*24*time.Hour {
		return fmt.Sprintf("%dd", int(duration.Hours()/24))
	} else {
		return fmt.Sprintf("%dM", int(duration.Hours()/(24*30)))
	}
}

// Получение внутреннего IP узла
func getNodeInternalIP(node v1.Node) string {
	for _, addr := range node.Status.Addresses {
		if addr.Type == v1.NodeInternalIP {
			return addr.Address
		}
	}
	return "<none>"
}

// Получение зоны узла
func getNodeZone(node v1.Node) string {
	// Проверка стандартных лейблов зоны
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

// Проверка соответствия узла лейблам
func matchNodeLabels(node v1.Node, labelSelector string) bool {
	if labelSelector == "" {
		return true
	}

	// Простая проверка лейблов (key=value или key)
	labelPairs := strings.Split(labelSelector, ",")

	for _, pair := range labelPairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}

		parts := strings.Split(pair, "=")
		key := parts[0]

		if len(parts) == 2 {
			// Проверка key=value
			value := parts[1]
			if nodeValue, ok := node.Labels[key]; !ok || nodeValue != value {
				return false
			}
		} else {
			// Проверка existence (key существует)
			if _, ok := node.Labels[key]; !ok {
				return false
			}
		}
	}

	return true
}

var getNodesCmd = &cobra.Command{
	Use:     "nodes",
	Short:   "Получить список узлов",
	Aliases: []string{"node", "no"},
	Run: func(cmd *cobra.Command, args []string) {
		clientset, err := k8s.GetClientset()
		if err != nil {
			fmt.Printf("Ошибка подключения: %v\n", err)
			return
		}

		// Подготовка опций списка
		listOptions := metav1.ListOptions{}
		if nodeLabelSelector != "" {
			listOptions.LabelSelector = nodeLabelSelector
		}

		nodes, err := clientset.CoreV1().Nodes().List(context.TODO(), listOptions)
		if err != nil {
			fmt.Printf("Ошибка получения узлов: %v\n", err)
			return
		}

		// Фильтрация по статусу, лейблам и зоне
		var filteredNodes []v1.Node

		for _, node := range nodes.Items {
			// Проверка зоны
			if nodeZoneFilter != "" {
				nodeZone := getNodeZone(node)
				if nodeZone != nodeZoneFilter {
					continue
				}
			}

			// Проверка статуса
			if !matchNodeStatus(node, nodeStatusFilter) {
				continue
			}

			// Проверка лейблов (дополнительная фильтрация)
			if !matchNodeLabels(node, nodeLabelSelector) {
				continue
			}

			filteredNodes = append(filteredNodes, node)
		}

		if len(filteredNodes) == 0 {
			fmt.Println("Узлы не найдены")
			return
		}

		// Вывод результатов
		table := tablewriter.NewWriter(os.Stdout)
		table.Header([]string{"NAME", "STATUS", "ROLES", "ZONE", "AGE", "VERSION", "INTERNAL-IP"})

		for _, node := range filteredNodes {
			nodeStatus := getNodeStatus(node)
			roles := getNodeRoles(node)
			zone := getNodeZone(node)
			age := getNodeAge(node.CreationTimestamp)
			version := node.Status.NodeInfo.KubeletVersion
			internalIP := getNodeInternalIP(node)

			// Сокращение версии
			if len(version) > 20 {
				version = version[:20] + "..."
			}

			table.Append([]string{
				node.Name,
				nodeStatus,
				roles,
				zone,
				age,
				version,
				internalIP,
			})
		}
		table.Render()

		// Информация о фильтрах
		filterInfo := ""
		if nodeStatusFilter != "" {
			if strings.HasPrefix(nodeStatusFilter, "not:") {
				filterInfo = fmt.Sprintf(" (фильтр статуса: все кроме %s)", strings.TrimPrefix(nodeStatusFilter, "not:"))
			} else {
				filterInfo = fmt.Sprintf(" (фильтр статуса: %s)", nodeStatusFilter)
			}
		}

		zoneInfo := ""
		if nodeZoneFilter != "" {
			zoneInfo = fmt.Sprintf(" (зона: %s)", nodeZoneFilter)
		}

		labelInfo := ""
		if nodeLabelSelector != "" {
			labelInfo = fmt.Sprintf(" (фильтр лейблов: %s)", nodeLabelSelector)
		}

		fmt.Printf("\nВсего узлов: %d (отфильтровано: %d)%s%s%s\n", len(nodes.Items), len(filteredNodes), filterInfo, zoneInfo, labelInfo)
	},
}

func init() {
	getNodesCmd.Flags().StringVarP(&nodeStatusFilter, "status", "s", "", "Фильтр по статусу (Ready, NotReady, Unknown, используйте not: для инверсии)")
	getNodesCmd.Flags().StringVarP(&nodeZoneFilter, "zone", "z", "", "Фильтр по зоне (например: us-east-1, eu-west-1)")
	getNodesCmd.Flags().StringVarP(&nodeLabelSelector, "selector", "l", "", "Селектор лейблов (например: node-role.kubernetes.io/worker=)")
	getCmd.AddCommand(getNodesCmd)
}
