package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"codesome-usage-manager/internal/config"
	"codesome-usage-manager/internal/provider"
	"codesome-usage-manager/internal/repository"
	importsync "codesome-usage-manager/internal/sync"
)

var (
	feishuExploreAppToken string
	feishuExploreTableID  string
	feishuExploreViewID   string

	feishuSendKeysEmployeeNo      string
	feishuSendKeysTeam            string
	feishuSendKeysAll             bool
	feishuSendKeysIncludeInactive bool
	feishuSendKeysDryRun          bool
)

var feishuCmd = &cobra.Command{
	Use:   "feishu",
	Short: "管理飞书多维表格和消息发送",
}

var feishuBitableCmd = &cobra.Command{
	Use:   "bitable",
	Short: "管理飞书多维表格",
}

var feishuBitableExploreCmd = &cobra.Command{
	Use:   "explore",
	Short: "列出飞书多维表格字段并输出 user 字段建议",
	RunE:  runFeishuBitableExplore,
}

var feishuSendKeysCmd = &cobra.Command{
	Use:   "send-keys",
	Short: "通过飞书私聊批量发送本地 Codesome API Key",
	RunE:  runFeishuSendKeys,
}

func init() {
	feishuBitableExploreCmd.Flags().StringVar(&feishuExploreAppToken, "app-token", "", "飞书多维表格 app_token；为空时读取 config.yaml")
	feishuBitableExploreCmd.Flags().StringVar(&feishuExploreTableID, "table-id", "", "飞书多维表格 table_id；为空时读取 user 表配置")
	feishuBitableExploreCmd.Flags().StringVar(&feishuExploreViewID, "view-id", "", "飞书多维表格 view_id；为空时读取 user 表配置")
	feishuBitableCmd.AddCommand(feishuBitableExploreCmd)
	feishuCmd.AddCommand(feishuBitableCmd)

	feishuSendKeysCmd.Flags().StringVar(&feishuSendKeysEmployeeNo, "employee-no", "", "只发送指定员工")
	feishuSendKeysCmd.Flags().StringVar(&feishuSendKeysTeam, "team", "", "只发送指定团队")
	feishuSendKeysCmd.Flags().StringVar(&dbPath, "path", "", "SQLite 数据库路径")
	feishuSendKeysCmd.Flags().BoolVar(&feishuSendKeysAll, "all", false, "发送全部用户")
	feishuSendKeysCmd.Flags().BoolVar(&feishuSendKeysIncludeInactive, "include-inactive", false, "包含 inactive 用户或 key")
	feishuSendKeysCmd.Flags().BoolVar(&feishuSendKeysDryRun, "dry-run", false, "只输出发送计划，不调用飞书")
	feishuSendKeysCmd.MarkFlagsOneRequired("employee-no", "team", "all")
	feishuSendKeysCmd.MarkFlagsMutuallyExclusive("employee-no", "team", "all")
	feishuCmd.AddCommand(feishuSendKeysCmd)

	rootCmd.AddCommand(feishuCmd)
}

func runFeishuBitableExplore(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	cfg, client, err := loadFeishuClient()
	if err != nil {
		return err
	}
	feishu := cfg.GetFeishuConfig()

	appToken := firstNonEmpty(feishuExploreAppToken, feishu.Bitable.AppToken)
	tableID := firstNonEmpty(feishuExploreTableID, feishu.Bitable.Users.TableID)
	viewID := firstNonEmpty(feishuExploreViewID, feishu.Bitable.Users.ViewID)
	if appToken == "" {
		return fmt.Errorf("缺少飞书多维表格 app_token：请配置 feishu.bitable.app_token 或传 --app-token")
	}
	if tableID == "" {
		return fmt.Errorf("缺少飞书多维表格 table_id：请配置 feishu.bitable.users.table_id 或传 --table-id")
	}

	fields, err := client.ListBitableFields(ctx, appToken, tableID, viewID)
	if err != nil {
		return err
	}
	printFeishuBitableFields(fields)
	printFeishuUserFieldAdvice(fields, importsync.DefaultFeishuUserFields())
	return nil
}

func runFeishuSendKeys(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	database, err := openLocalDatabase(ctx)
	if err != nil {
		return err
	}
	defer database.Close()

	params, err := resolveFeishuSendKeyParams()
	if err != nil {
		return err
	}
	rows, err := repository.NewAPIKeyRepository(database).ListExportRows(ctx, params)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return fmt.Errorf("未找到可发送的 API Key")
	}

	var client feishuMessageClient
	if !feishuSendKeysDryRun {
		_, client, err = loadFeishuClient()
		if err != nil {
			return err
		}
	}

	results, err := sendFeishuKeys(ctx, client, rows, feishuSendKeysDryRun)
	printFeishuSendKeyResults(results)
	return err
}

func loadFeishuClient() (*config.Config, *provider.FeishuClient, error) {
	cfg, err := config.LoadConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("加载配置失败: %w", err)
	}
	client, err := provider.NewFeishuClient(cfg)
	if err != nil {
		return nil, nil, err
	}
	return cfg, client, nil
}

func resolveFeishuSendKeyParams() (repository.ListAPIKeyExportRowsParams, error) {
	selectorCount := 0
	if feishuSendKeysEmployeeNo != "" {
		selectorCount++
	}
	if feishuSendKeysTeam != "" {
		selectorCount++
	}
	if feishuSendKeysAll {
		selectorCount++
	}
	if selectorCount != 1 {
		return repository.ListAPIKeyExportRowsParams{}, fmt.Errorf("必须且只能指定 --employee-no、--team、--all 之一")
	}
	return repository.ListAPIKeyExportRowsParams{
		EmployeeNo:      feishuSendKeysEmployeeNo,
		TeamCode:        feishuSendKeysTeam,
		IncludeInactive: feishuSendKeysIncludeInactive,
	}, nil
}

type feishuSendKeyResult struct {
	EmployeeNo string
	Name       string
	OpenID     string
	KeyID      int
	Action     string
	Message    string
	Error      string
}

type feishuMessageClient interface {
	SendTextMessage(ctx context.Context, openID string, text string) (*provider.FeishuMessageResult, error)
}

func sendFeishuKeys(ctx context.Context, client feishuMessageClient, rows []repository.APIKeyExportRow, dryRun bool) ([]feishuSendKeyResult, error) {
	results := make([]feishuSendKeyResult, 0, len(rows))
	var failed []feishuSendKeyResult
	for _, row := range rows {
		result := sendFeishuKey(ctx, client, row, dryRun)
		results = append(results, result)
		if result.Error != "" {
			failed = append(failed, result)
		}
	}
	if len(failed) == 0 {
		return results, nil
	}
	return results, fmt.Errorf("发送飞书消息失败: %d/%d 失败\n%s", len(failed), len(results), formatFeishuSendFailures(failed))
}

func formatFeishuSendFailures(failed []feishuSendKeyResult) string {
	lines := make([]string, 0, len(failed))
	for _, result := range failed {
		lines = append(lines, fmt.Sprintf("employee_no=%s name=%s error=%s", result.EmployeeNo, result.Name, result.Error))
	}
	return strings.Join(lines, "\n")
}

func sendFeishuKey(ctx context.Context, client feishuMessageClient, row repository.APIKeyExportRow, dryRun bool) feishuSendKeyResult {
	result := feishuSendKeyResult{
		EmployeeNo: row.EmployeeNo,
		Name:       row.UserName,
		OpenID:     row.FeishuOpenID,
		KeyID:      row.CodesomeKeyID,
	}
	if row.FeishuOpenID == "" {
		result.Action = "skip"
		result.Message = "缺少 feishu_open_id"
		return result
	}
	rawKey := stringValue(row.RawKey)
	if rawKey == "" {
		result.Action = "skip"
		result.Message = "本地没有 raw_key，无法自动发送"
		return result
	}
	if dryRun {
		result.Action = "send"
		result.Message = "dry-run"
		return result
	}
	if client == nil {
		result.Action = "error"
		result.Error = "feishu client is nil"
		return result
	}
	sent, err := client.SendTextMessage(ctx, row.FeishuOpenID, buildFeishuKeyMessage(row, rawKey))
	if err != nil {
		result.Action = "error"
		result.Error = err.Error()
		return result
	}
	result.Action = "sent"
	result.Message = "message_id=" + sent.MessageID
	return result
}

func buildFeishuKeyMessage(row repository.APIKeyExportRow, rawKey string) string {
	return fmt.Sprintf("你的 Codesome API Key 已生成。\n\n名称：%s\nKey：%s\n\n请妥善保管，不要转发给他人。", row.KeyName, rawKey)
}

func printFeishuBitableFields(fields []provider.FeishuBitableField) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "FIELD_ID\tFIELD_NAME\tTYPE\tUI_TYPE\tPRIMARY")
	for _, field := range fields {
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%t\n",
			field.FieldID,
			field.FieldName,
			field.Type,
			field.UIType,
			field.IsPrimary,
		)
	}
	w.Flush()
}

func printFeishuUserFieldAdvice(fields []provider.FeishuBitableField, mapping config.FeishuUserFields) {
	names := map[string]bool{}
	for _, field := range fields {
		names[field.FieldName] = true
	}

	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, "User table field advice:")
	printFieldMappingAdvice("employee_no", mapping.EmployeeNo, names[mapping.EmployeeNo], "required, unique; text or number")
	printFieldMappingAdvice("name", mapping.Name, names[mapping.Name], "required; text")
	printFieldMappingAdvice("open_id", mapping.OpenID, names[mapping.OpenID], "required for Feishu delivery; person field or text open_id")
	printFieldMappingAdvice("team", mapping.Team, names[mapping.Team], "optional; must match local team code when set")
	printFieldMappingAdvice("status", mapping.Status, names[mapping.Status], "optional; 生效/禁用 or active/inactive")
	fmt.Fprintln(os.Stdout, "- group_id -> <internal> Codesome group is selected by sync users, not imported from Feishu")
}

func printFieldMappingAdvice(key string, fieldName string, exists bool, note string) {
	if fieldName == "" {
		fmt.Fprintf(os.Stdout, "- %s -> <unconfigured> [missing] %s\n", key, note)
		return
	}
	status := "missing"
	if exists {
		status = "ok"
	}
	fmt.Fprintf(os.Stdout, "- %s -> %s [%s] %s\n", key, fieldName, status, note)
}

func printFeishuSendKeyResults(results []feishuSendKeyResult) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "EMPLOYEE_NO\tNAME\tOPEN_ID\tKEY_ID\tACTION\tMESSAGE\tERROR")
	for _, result := range results {
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\t%s\t%s\n",
			result.EmployeeNo,
			result.Name,
			result.OpenID,
			result.KeyID,
			result.Action,
			result.Message,
			result.Error,
		)
	}
	w.Flush()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
