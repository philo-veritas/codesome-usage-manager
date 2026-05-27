package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"codesome-usage-manager/internal/config"
	"codesome-usage-manager/internal/provider"
)

var (
	dailyKeyID    int
	dailyKeyAlias string
)

var dailyUsageCmd = &cobra.Command{
	Use:   "daily-usage",
	Short: "查询 Codesome API Key 的今日用量",
	Long: `查询指定 Codesome API Key 的今日用量（TodayCost）

示例:
  codesome daily-usage --key main
  codesome daily-usage --key-id 2085`,
	RunE: runDailyUsage,
}

func init() {
	dailyUsageCmd.Flags().IntVar(&dailyKeyID, "key-id", 0, "要查询的 API Key ID")
	dailyUsageCmd.Flags().StringVar(&dailyKeyAlias, "key", "", "配置文件中的 key 别名")
	dailyUsageCmd.MarkFlagsOneRequired("key-id", "key")
	dailyUsageCmd.MarkFlagsMutuallyExclusive("key-id", "key")
	rootCmd.AddCommand(dailyUsageCmd)
}

func runDailyUsage(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}

	if cfg.GetCodesomeConfig() == nil {
		return fmt.Errorf("未找到 Codesome 配置")
	}

	resolvedID := dailyKeyID
	if dailyKeyAlias != "" {
		resolvedID, err = cfg.ResolveCodesomeKeyID(dailyKeyAlias)
		if err != nil {
			return err
		}
	}

	todayCost, err := provider.GetCodesomeKeyDailyUsage(cfg, resolvedID)
	if err != nil {
		return fmt.Errorf("查询今日用量失败: %w", err)
	}

	fmt.Printf("API Key %d 今日用量: $%.2f\n", resolvedID, todayCost)
	return nil
}
