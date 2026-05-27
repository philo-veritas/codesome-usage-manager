package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"codesome-usage-manager/internal/config"
	"codesome-usage-manager/internal/provider"
)

var (
	forceUpdate bool
	debug       bool
)

var rootCmd = &cobra.Command{
	Use:   "codesome",
	Short: "Codesome 用量和 API Key 管理工具",
	Long:  "codesome-usage-manager - Codesome 用量查询、API Key 管理、quota reset 和 group 切换工具",
	RunE:  runUsage,
}

func init() {
	rootCmd.Flags().BoolVarP(&forceUpdate, "force-update", "f", false, "强制刷新远程数据")
	rootCmd.Flags().BoolVarP(&debug, "debug", "d", false, "Debug 模式，打印原始 JSON 数据")
}

func runUsage(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}
	return runCodesome(cfg)
}

func runCodesome(cfg *config.Config) error {
	keys, subs, usageMap, tokenStatsMap, err := provider.FetchCodesomeUsage(cfg, forceUpdate)
	if err != nil {
		return fmt.Errorf("获取 Codesome 使用量失败: %w", err)
	}
	provider.DisplayCodesomeUsage(keys, subs, usageMap, tokenStatsMap, debug)
	return nil
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
