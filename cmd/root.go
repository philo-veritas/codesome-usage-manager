package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"usage-cli/internal/config"
	"usage-cli/internal/provider"
)

var (
	forceUpdate  bool
	debug        bool
	providerFlag string
)

var rootCmd = &cobra.Command{
	Use:   "usage",
	Short: "显示 Claude API 使用量",
	Long:  "Claude Code Usage Helper - 显示 API 使用量",
	RunE:  runUsage,
}

func init() {
	rootCmd.Flags().BoolVarP(&forceUpdate, "force-update", "f", false, "强制刷新远程数据")
	rootCmd.Flags().BoolVarP(&debug, "debug", "d", false, "Debug 模式，打印原始 JSON 数据")
	rootCmd.Flags().StringVar(&providerFlag, "provider", "",
		"指定 provider: claude-buddy, codesome（默认自动检测）")
}

func resolveProvider(cfg *config.Config) (string, error) {
	if providerFlag != "" {
		return providerFlag, nil
	}

	// Auto-detect: try env-based first
	isClaudeBuddy, err := cfg.IsClaudeBuddyProvider()
	if err == nil && isClaudeBuddy {
		return "claude-buddy", nil
	}

	// Check if Codesome is configured
	if cfg.GetCodesomeConfig() != nil {
		return "codesome", nil
	}

	return "", fmt.Errorf("未检测到有效的 provider，请使用 --provider 指定")
}

func runUsage(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}

	selected, err := resolveProvider(cfg)
	if err != nil {
		return err
	}

	switch selected {
	case "claude-buddy":
		return runClaudeBuddy(cfg)
	case "codesome":
		return runCodesome(cfg)
	default:
		return fmt.Errorf("不支持的 provider: %s", selected)
	}
}

func runClaudeBuddy(cfg *config.Config) error {
	apiID, err := cfg.GetApiIDByEnv()
	if err != nil {
		return fmt.Errorf("获取 API ID 失败: %w", err)
	}
	usageData, err := provider.FetchUsage(apiID, forceUpdate)
	if err != nil {
		return fmt.Errorf("获取使用量失败: %w", err)
	}
	provider.DisplayUsage(usageData, debug)
	return nil
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
