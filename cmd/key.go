package cmd

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"codesome-usage-manager/internal/provider"
	"codesome-usage-manager/internal/repository"
)

var (
	createKeyName    string
	createKeyGroupID int
	updateKeyID      int
	updateKeyAlias   string
	updateKeyName    string
	updateKeyGroupID int
	updateKeyStatus  string

	keyExportEmployeeNo      string
	keyExportTeam            string
	keyExportAll             bool
	keyExportOutput          string
	keyExportIncludeInactive bool
)

var createKeyCmd = &cobra.Command{
	Use:   "create-key",
	Short: "创建 Codesome API Key",
	Long: `创建 Codesome API Key，并输出新 key。返回的 sk-... 只会展示一次，不写入本地缓存。

示例:
  codesome create-key --name test --group-id 51`,
	RunE: runCreateKey,
}

var updateKeyCmd = &cobra.Command{
	Use:   "update-key",
	Short: "更新 Codesome API Key",
	Long: `更新 Codesome API Key 的名称、状态或 group。

示例:
  codesome update-key --key-id 9356 --status inactive
  codesome update-key --key main --name main-2
  codesome update-key --key-id 9356 --group-id 51`,
	RunE: runUpdateKey,
}

var keyCmd = &cobra.Command{
	Use:   "key",
	Short: "管理本地 Codesome API Key",
}

var keyExportCmd = &cobra.Command{
	Use:   "export",
	Short: "导出本地 Codesome API Key 分发清单",
	RunE:  runKeyExport,
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

	keyCmd.PersistentFlags().StringVar(&dbPath, "path", "", "SQLite 数据库路径")
	keyExportCmd.Flags().StringVar(&keyExportEmployeeNo, "employee-no", "", "只导出指定员工")
	keyExportCmd.Flags().StringVar(&keyExportTeam, "team", "", "只导出指定团队")
	keyExportCmd.Flags().BoolVar(&keyExportAll, "all", false, "导出全部用户")
	keyExportCmd.Flags().StringVar(&keyExportOutput, "output", "", "CSV 输出路径；为空时输出到 stdout")
	keyExportCmd.Flags().BoolVar(&keyExportIncludeInactive, "include-inactive", false, "包含 inactive 用户或 key")
	keyExportCmd.MarkFlagsOneRequired("employee-no", "team", "all")
	keyExportCmd.MarkFlagsMutuallyExclusive("employee-no", "team", "all")
	keyCmd.AddCommand(keyExportCmd)
	rootCmd.AddCommand(keyCmd)
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

func runKeyExport(cmd *cobra.Command, args []string) error {
	params, err := resolveKeyExportParams()
	if err != nil {
		return err
	}

	database, err := openLocalDatabase(context.Background())
	if err != nil {
		return err
	}
	defer database.Close()

	rows, err := repository.NewAPIKeyRepository(database).ListExportRows(context.Background(), params)
	if err != nil {
		return err
	}

	var writer io.Writer = os.Stdout
	var outputFile *os.File
	if keyExportOutput != "" {
		outputFile, err = createKeyExportOutput(keyExportOutput)
		if err != nil {
			return err
		}
		writer = outputFile
	}

	if err := writeAPIKeyExportCSV(writer, rows); err != nil {
		if outputFile != nil {
			outputFile.Close()
		}
		return err
	}
	if keyExportOutput != "" {
		if err := outputFile.Close(); err != nil {
			return fmt.Errorf("关闭导出文件失败: %w", err)
		}
		fmt.Fprintf(os.Stderr, "已导出 %d 条 API Key：%s\n", len(rows), keyExportOutput)
	}
	return nil
}

func createKeyExportOutput(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return nil, fmt.Errorf("创建导出文件失败: %w", err)
	}
	if err := file.Chmod(0600); err != nil {
		file.Close()
		return nil, fmt.Errorf("设置导出文件权限失败: %w", err)
	}
	return file, nil
}

func resolveKeyExportParams() (repository.ListAPIKeyExportRowsParams, error) {
	selectorCount := 0
	if keyExportEmployeeNo != "" {
		selectorCount++
	}
	if keyExportTeam != "" {
		selectorCount++
	}
	if keyExportAll {
		selectorCount++
	}
	if selectorCount != 1 {
		return repository.ListAPIKeyExportRowsParams{}, fmt.Errorf("必须且只能指定 --employee-no、--team、--all 之一")
	}
	return repository.ListAPIKeyExportRowsParams{
		EmployeeNo:      keyExportEmployeeNo,
		TeamCode:        keyExportTeam,
		IncludeInactive: keyExportIncludeInactive,
	}, nil
}

func writeAPIKeyExportCSV(writer io.Writer, rows []repository.APIKeyExportRow) error {
	csvWriter := csv.NewWriter(writer)
	if err := csvWriter.Write([]string{
		"employee_no",
		"name",
		"team",
		"key_name",
		"codesome_key_id",
		"raw_key",
		"raw_key_missing",
		"status",
	}); err != nil {
		return fmt.Errorf("写入 CSV 表头失败: %w", err)
	}
	for _, row := range rows {
		rawKey := stringValue(row.RawKey)
		rawKeyMissing := "false"
		if rawKey == "" {
			rawKeyMissing = "true"
		}
		if err := csvWriter.Write([]string{
			row.EmployeeNo,
			row.UserName,
			stringValue(row.TeamCode),
			row.KeyName,
			strconv.Itoa(row.CodesomeKeyID),
			rawKey,
			rawKeyMissing,
			row.Status,
		}); err != nil {
			return fmt.Errorf("写入 CSV 行失败: %w", err)
		}
	}
	csvWriter.Flush()
	if err := csvWriter.Error(); err != nil {
		return fmt.Errorf("写入 CSV 失败: %w", err)
	}
	return nil
}
