package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"usage-cli/internal/provider"
)

var (
	createKeyName    string
	createKeyGroupID int
	updateKeyID      int
	updateKeyAlias   string
	updateKeyName    string
	updateKeyGroupID int
	updateKeyStatus  string
)

var createKeyCmd = &cobra.Command{
	Use:   "create-key",
	Short: "创建 Codesome API Key",
	Long: `创建 Codesome API Key，并输出新 key。返回的 sk-... 只会展示一次，不写入本地缓存。

示例:
  usage-cli create-key --name test --group-id 51`,
	RunE: runCreateKey,
}

var updateKeyCmd = &cobra.Command{
	Use:   "update-key",
	Short: "更新 Codesome API Key",
	Long: `更新 Codesome API Key 的名称、状态或 group。

示例:
  usage-cli update-key --key-id 9356 --status inactive
  usage-cli update-key --key main --name main-2
  usage-cli update-key --key-id 9356 --group-id 51`,
	RunE: runUpdateKey,
}

func init() {
	createKeyCmd.Flags().StringVar(&createKeyName, "name", "", "API Key 名称")
	createKeyCmd.Flags().IntVar(&createKeyGroupID, "group-id", 0, "绑定的 group ID")
	createKeyCmd.MarkFlagRequired("name")
	createKeyCmd.MarkFlagRequired("group-id")
	rootCmd.AddCommand(createKeyCmd)

	updateKeyCmd.Flags().IntVar(&updateKeyID, "key-id", 0, "要更新的 API Key ID")
	updateKeyCmd.Flags().StringVar(&updateKeyAlias, "key", "", "配置文件中的 key 别名")
	updateKeyCmd.Flags().StringVar(&updateKeyName, "name", "", "新的 API Key 名称")
	updateKeyCmd.Flags().IntVar(&updateKeyGroupID, "group-id", 0, "新的 group ID")
	updateKeyCmd.Flags().StringVar(&updateKeyStatus, "status", "", "新的状态: active 或 inactive")
	updateKeyCmd.MarkFlagsOneRequired("key-id", "key")
	updateKeyCmd.MarkFlagsMutuallyExclusive("key-id", "key")
	rootCmd.AddCommand(updateKeyCmd)
}

func runCreateKey(cmd *cobra.Command, args []string) error {
	cfg, err := loadCodesomeConfig()
	if err != nil {
		return err
	}

	key, err := provider.CreateCodesomeKey(cfg, createKeyName, createKeyGroupID)
	if err != nil {
		return fmt.Errorf("创建 API Key 失败: %w", err)
	}

	fmt.Printf("API Key 已创建：id=%d name=%s group_id=%d status=%s\n", key.ID, key.Name, key.GroupID, key.Status)
	if key.Key != "" {
		fmt.Printf("key=%s\n", key.Key)
	}
	return nil
}

func runUpdateKey(cmd *cobra.Command, args []string) error {
	cfg, err := loadCodesomeConfig()
	if err != nil {
		return err
	}

	resolvedID, err := resolveCodesomeKeyFlag(cfg, updateKeyID, updateKeyAlias)
	if err != nil {
		return err
	}

	update := provider.CodesomeKeyUpdate{}
	if cmd.Flags().Changed("name") {
		update.Name = &updateKeyName
	}
	if cmd.Flags().Changed("group-id") {
		update.GroupID = &updateKeyGroupID
	}
	if cmd.Flags().Changed("status") {
		update.Status = &updateKeyStatus
	}

	key, err := provider.UpdateCodesomeKey(cfg, resolvedID, update)
	if err != nil {
		return fmt.Errorf("更新 API Key 失败: %w", err)
	}

	fmt.Printf("API Key %d 已更新：name=%s group_id=%d status=%s\n", key.ID, key.Name, key.GroupID, key.Status)
	return nil
}
