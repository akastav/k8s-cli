package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "k8s-cli",
	Short: "CLI утилита для работы с Kubernetes",
	Long:  `Утилита для управления ресурсами Kubernetes`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
