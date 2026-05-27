package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"usage-cli/internal/config"
	"usage-cli/internal/provider"
)

var (
	keyID    int
	keyAlias string
)

var resetQuotaCmd = &cobra.Command{
	Use:   "reset-quota",
	Short: "重置 Codesome API Key 的配额",
	Long:  "调用 Codesome API 重置指定 API Key 的配额，并清除本地缓存\n\n示例:\n  usage-cli reset-quota --key main\n  usage-cli reset-quota --key-id 2085",
	RunE:  runResetQuota,
}

func init() {
	resetQuotaCmd.Flags().IntVar(&keyID, "key-id", 0, "要重置配额的 API Key ID")
	resetQuotaCmd.Flags().StringVar(&keyAlias, "key", "", "配置文件中的 key 别名")
	resetQuotaCmd.MarkFlagsOneRequired("key-id", "key")
	resetQuotaCmd.MarkFlagsMutuallyExclusive("key-id", "key")
	rootCmd.AddCommand(resetQuotaCmd)
}

func runResetQuota(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}

	if cfg.GetCodesomeConfig() == nil {
		return fmt.Errorf("未找到 Codesome 配置")
	}

	resolvedID := keyID
	if keyAlias != "" {
		resolvedID, err = cfg.ResolveCodesomeKeyID(keyAlias)
		if err != nil {
			return err
		}
	}

	if err := provider.ResetCodesomeQuota(cfg, resolvedID); err != nil {
		return fmt.Errorf("重置配额失败: %w", err)
	}

	fmt.Printf("API Key %d 配额已重置，本地缓存已清除\n", resolvedID)
	return nil
}
