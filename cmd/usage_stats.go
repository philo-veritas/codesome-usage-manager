package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"usage-cli/internal/provider"
)

var (
	usageStatsKeyID       int
	usageStatsKeyAlias    string
	usageStatsStartDate   string
	usageStatsEndDate     string
	usageStatsForceUpdate bool
)

var usageStatsCmd = &cobra.Command{
	Use:   "usage-stats",
	Short: "查询 Codesome API Key 日期范围用量",
	Long: `查询指定 Codesome API Key 在日期范围内的聚合用量。
start-date 和 end-date 为左右包含，格式为 YYYY-MM-DD，timezone 固定为 Asia/Shanghai。

示例:
  usage-cli usage-stats --key main --start-date 2026-05-26 --end-date 2026-05-26
  usage-cli usage-stats --key-id 6732 --start-date 2026-05-26 --end-date 2026-05-27`,
	RunE: runUsageStats,
}

func init() {
	usageStatsCmd.Flags().IntVar(&usageStatsKeyID, "key-id", 0, "要查询的 API Key ID")
	usageStatsCmd.Flags().StringVar(&usageStatsKeyAlias, "key", "", "配置文件中的 key 别名")
	usageStatsCmd.Flags().StringVar(&usageStatsStartDate, "start-date", "", "开始日期 YYYY-MM-DD")
	usageStatsCmd.Flags().StringVar(&usageStatsEndDate, "end-date", "", "结束日期 YYYY-MM-DD")
	usageStatsCmd.Flags().BoolVar(&usageStatsForceUpdate, "force-update", false, "强制刷新远程数据")
	usageStatsCmd.MarkFlagsOneRequired("key-id", "key")
	usageStatsCmd.MarkFlagsMutuallyExclusive("key-id", "key")
	usageStatsCmd.MarkFlagRequired("start-date")
	usageStatsCmd.MarkFlagRequired("end-date")
	rootCmd.AddCommand(usageStatsCmd)
}

func runUsageStats(cmd *cobra.Command, args []string) error {
	cfg, err := loadCodesomeConfig()
	if err != nil {
		return err
	}

	resolvedID, err := resolveCodesomeKeyFlag(cfg, usageStatsKeyID, usageStatsKeyAlias)
	if err != nil {
		return err
	}

	stats, err := provider.GetCodesomeKeyUsageStats(cfg, resolvedID, usageStatsStartDate, usageStatsEndDate, usageStatsForceUpdate)
	if err != nil {
		return fmt.Errorf("查询日期范围用量失败: %w", err)
	}

	fmt.Printf("API Key %d 用量（%s 至 %s，含首尾）：\n", resolvedID, usageStatsStartDate, usageStatsEndDate)
	fmt.Printf("请求数: %d\n", stats.TotalRequests)
	fmt.Printf("输入 tokens: %d\n", stats.TotalInputTokens)
	fmt.Printf("输出 tokens: %d\n", stats.TotalOutputTokens)
	fmt.Printf("缓存 tokens: %d\n", stats.TotalCacheTokens)
	fmt.Printf("总 tokens: %d\n", stats.TotalTokens)
	fmt.Printf("成本: $%.6f\n", stats.TotalCost)
	fmt.Printf("实际成本: $%.6f\n", stats.TotalActualCost)
	fmt.Printf("平均耗时: %.2f ms\n", stats.AverageDurationMS)
	return nil
}
