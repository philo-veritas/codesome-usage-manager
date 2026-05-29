package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"codesome-usage-manager/internal/config"
	"codesome-usage-manager/internal/provider"
)

var (
	switchKeyID       int
	switchKeyAlias    string
	switchGroupID     int
	exhaustedKeyID    int
	exhaustedKeyAlias string
	exhaustedAll      bool
	minRemainingUSD   float64
)

var switchGroupCmd = &cobra.Command{
	Use:   "switch-group",
	Short: "切换 Codesome API Key 绑定的 group",
	Long: `切换指定 Codesome API Key 绑定的 group，从而切换该 Key 使用的 subscription 额度。

示例:
  codesome switch-group --key main --group-id 60
  codesome switch-group --key-id 6732 --group-id 60`,
	RunE: runSwitchGroup,
}

var switchOnExhaustedCmd = &cobra.Command{
	Use:   "switch-on-exhausted",
	Short: "当前 subscription 额度低于阈值后自动切换 Codesome group",
	Long: `检查指定 Codesome API Key 当前绑定 group 对应的 active subscription。
如果该 subscription 今日剩余额度低于阈值，则自动切换到剩余额度最多的 active subscription group。
默认阈值为 0，保持“用尽后切换”的行为。

示例:
  codesome switch-on-exhausted --key main
  codesome switch-on-exhausted --key-id 6732
  codesome switch-on-exhausted --all --min-remaining 10`,
	RunE: runSwitchOnExhausted,
}

func init() {
	switchGroupCmd.Flags().IntVar(&switchKeyID, "key-id", 0, "要切换 group 的 API Key ID")
	switchGroupCmd.Flags().StringVar(&switchKeyAlias, "key", "", "legacy api_key_ids 中的 key 别名")
	switchGroupCmd.Flags().IntVar(&switchGroupID, "group-id", 0, "目标 group ID")
	switchGroupCmd.MarkFlagsOneRequired("key-id", "key")
	switchGroupCmd.MarkFlagsMutuallyExclusive("key-id", "key")
	switchGroupCmd.MarkFlagRequired("group-id")
	rootCmd.AddCommand(switchGroupCmd)

	switchOnExhaustedCmd.Flags().IntVar(&exhaustedKeyID, "key-id", 0, "要检查的 API Key ID")
	switchOnExhaustedCmd.Flags().StringVar(&exhaustedKeyAlias, "key", "", "legacy api_key_ids 中的 key 别名")
	switchOnExhaustedCmd.Flags().BoolVar(&exhaustedAll, "all", false, "检查 legacy api_key_ids 中的所有 API Key")
	switchOnExhaustedCmd.Flags().Float64Var(&minRemainingUSD, "min-remaining", 0, "当前 group 剩余额度低于该 USD 阈值时切换")
	switchOnExhaustedCmd.MarkFlagsOneRequired("key-id", "key", "all")
	switchOnExhaustedCmd.MarkFlagsMutuallyExclusive("key-id", "key", "all")
	rootCmd.AddCommand(switchOnExhaustedCmd)
}

func resolveCodesomeKeyFlag(cfg *config.Config, keyID int, keyAlias string) (int, error) {
	if keyAlias == "" {
		return keyID, nil
	}
	return cfg.ResolveCodesomeKeyID(keyAlias)
}

func loadCodesomeConfig() (*config.Config, error) {
	cfg, err := config.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("加载配置失败: %w", err)
	}
	if cfg.GetCodesomeConfig() == nil {
		return nil, fmt.Errorf("未找到 Codesome 配置")
	}
	return cfg, nil
}

func runSwitchGroup(cmd *cobra.Command, args []string) error {
	cfg, err := loadCodesomeConfig()
	if err != nil {
		return err
	}

	resolvedID, err := resolveCodesomeKeyFlag(cfg, switchKeyID, switchKeyAlias)
	if err != nil {
		return err
	}

	result, err := provider.SwitchCodesomeKeyGroup(cfg, resolvedID, switchGroupID)
	if err != nil {
		return fmt.Errorf("切换 group 失败: %w", err)
	}
	printGroupSwitchResult(result)
	return nil
}

func runSwitchOnExhausted(cmd *cobra.Command, args []string) error {
	cfg, err := loadCodesomeConfig()
	if err != nil {
		return err
	}
	if minRemainingUSD < 0 {
		return fmt.Errorf("min-remaining 必须大于等于 0")
	}

	if exhaustedAll {
		codesome := cfg.GetCodesomeConfig()
		if len(codesome.ApiKeyIDs) == 0 {
			return fmt.Errorf("未配置 legacy api_key_ids")
		}
		results, summary, err := provider.SwitchCodesomeKeysGroupOnExhaustedWithSummary(cfg, codesome.ApiKeyIDs, minRemainingUSD)
		if err != nil {
			return fmt.Errorf("批量自动切换 group 失败: %w", err)
		}
		printSubscriptionUsageSummary(summary)
		if hasErrors := printGroupSwitchBatchResults(results); hasErrors {
			return fmt.Errorf("部分 API Key 自动切换 group 失败")
		}
		return nil
	}

	resolvedID, err := resolveCodesomeKeyFlag(cfg, exhaustedKeyID, exhaustedKeyAlias)
	if err != nil {
		return err
	}

	result, err := provider.SwitchCodesomeKeyGroupOnExhausted(cfg, resolvedID, minRemainingUSD)
	if err != nil {
		return fmt.Errorf("自动切换 group 失败: %w", err)
	}
	printGroupSwitchResult(result)
	return nil
}

func printGroupSwitchResult(result *provider.CodesomeGroupSwitchResult) {
	if result == nil {
		return
	}
	printGroupSwitchResultWithLabel(fmt.Sprintf("%d", result.KeyID), result)
}

func printSubscriptionUsageSummary(summary provider.CodesomeSubscriptionUsageSummary) {
	printSubscriptionUsageSummaryWith(summary, stdoutPrintf)
}

func printSubscriptionUsageSummaryWith(summary provider.CodesomeSubscriptionUsageSummary, printf func(string, ...any)) {
	printf("今日总余额：$%.2f / $%.2f\n", summary.RemainingUSD, summary.LimitUSD)
}

func printGroupSwitchBatchResults(results []provider.CodesomeGroupSwitchBatchResult) bool {
	return printGroupSwitchBatchResultsWith(results, stdoutPrintf)
}

func printGroupSwitchBatchResultsWith(results []provider.CodesomeGroupSwitchBatchResult, printf func(string, ...any)) bool {
	hasErrors := false
	for _, item := range results {
		keyLabel := formatKeyLabel(item.KeyID, item.Name)
		if item.Error != "" {
			hasErrors = true
			printf("API Key %s 执行失败：%s\n", keyLabel, item.Error)
			continue
		}
		if item.Result == nil {
			hasErrors = true
			printf("API Key %s 执行失败：未返回执行结果\n", keyLabel)
			continue
		}
		printGroupSwitchResultWithLabelWith(keyLabel, item.Result, printf)
	}
	return hasErrors
}

func printGroupSwitchResultWithLabel(keyLabel string, result *provider.CodesomeGroupSwitchResult) {
	printGroupSwitchResultWithLabelWith(keyLabel, result, stdoutPrintf)
}

func printGroupSwitchResultWithLabelWith(keyLabel string, result *provider.CodesomeGroupSwitchResult, printf func(string, ...any)) {
	if result.Switched {
		printf(
			"API Key %s 已切换 group: %s -> %s，当前剩余额度 $%.2f，目标剩余额度 $%.2f\n",
			keyLabel,
			formatGroupLabel(result.FromGroupID, result.FromGroupName),
			formatGroupLabel(result.ToGroupID, result.ToGroupName),
			result.CurrentRemainingUSD,
			result.TargetRemainingUSD,
		)
		return
	}

	printf(
		"API Key %s 未切换：%s，当前 group %s 剩余额度 $%.2f\n",
		keyLabel,
		result.Message,
		formatGroupLabel(result.FromGroupID, result.FromGroupName),
		result.CurrentRemainingUSD,
	)
}

func stdoutPrintf(format string, args ...any) {
	fmt.Printf(format, args...)
}

func formatKeyLabel(id int, name string) string {
	if name == "" {
		return fmt.Sprintf("%d", id)
	}
	return fmt.Sprintf("%d(%s)", id, name)
}

func formatGroupLabel(id int, name string) string {
	if name == "" {
		return fmt.Sprintf("%d", id)
	}
	return fmt.Sprintf("%d(%s)", id, name)
}
