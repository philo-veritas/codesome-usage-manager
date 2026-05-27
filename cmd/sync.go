package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"usage-cli/internal/config"
	"usage-cli/internal/provider"
)

var quiet bool

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "同步所有 Claude Buddy 账号的缓存",
	Long:  "遍历配置文件中所有 Claude Buddy provider 的账号，强制更新每个账号的缓存",
	RunE:  runSync,
}

func init() {
	syncCmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "仅显示汇总信息")
	rootCmd.AddCommand(syncCmd)
}

func runSync(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}

	accounts := cfg.GetAllClaudeBuddyAccounts()
	if len(accounts) == 0 {
		return fmt.Errorf("未找到 Claude Buddy 账号配置")
	}

	var successCount, failCount int

	for _, account := range accounts {
		usageData, err := provider.FetchUsage(account.ApiID, true)
		if err != nil {
			failCount++
			if !quiet {
				fmt.Printf("✗ %s 刷新失败: %v\n", account.Name, err)
			}
			continue
		}

		successCount++
		if !quiet {
			usageLine := provider.FormatUsageLine(usageData)
			fmt.Printf("✓ %s 已刷新，%s\n", account.Name, usageLine)
		}
	}

	fmt.Printf("同步完成：总计 %d 个账号，成功 %d，失败 %d\n",
		len(accounts), successCount, failCount)

	return nil
}
