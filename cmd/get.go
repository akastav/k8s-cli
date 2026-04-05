package cmd

import (
	"github.com/spf13/cobra"
)

var getCmd = &cobra.Command{
	Use:   "get",
	Short: "Получить информацию о ресурсах",
	Long:  `Отобразить один или несколько ресурсов Kubernetes`,
}

func init() {
	rootCmd.AddCommand(getCmd)
}
