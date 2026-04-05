package cmd

import (
	"github.com/spf13/cobra"
)

var deleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Удалить ресурсы",
	Long:  `Удаление ресурсов Kubernetes`,
}

func init() {
	rootCmd.AddCommand(deleteCmd)
}
