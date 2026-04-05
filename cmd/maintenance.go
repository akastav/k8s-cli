package cmd

import (
	"github.com/spf13/cobra"
)

var maintenanceCmd = &cobra.Command{
	Use:   "maintenance",
	Short: "Управление режимом обслуживания",
	Long:  `Включение и выключение режима обслуживания для узлов в указанной зоне.`,
}

func init() {
	rootCmd.AddCommand(maintenanceCmd)
}
