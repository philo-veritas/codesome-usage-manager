package cmd

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"codesome-usage-manager/internal/config"
	"codesome-usage-manager/internal/provider"
	"codesome-usage-manager/internal/repository"
	usersync "codesome-usage-manager/internal/sync"
)

var (
	usageImportCodexEmployeeNo string
	usageImportCodexDate       string
	usageImportCodexFromDate   string
	usageImportCodexToDate     string
	usageImportCodexHome       string
	usageImportCodexOffline    bool
	usageImportCodexOverwrite  bool
	usageImportCodexDryRun     bool
)

var usageImportCmd = &cobra.Command{
	Use:   "import",
	Short: "导入外部用量",
}

var usageImportCodexCmd = &cobra.Command{
	Use:   "codex",
	Short: "导入官方 Codex 本地用量",
	RunE:  runUsageImportCodex,
}

func init() {
	usageImportCodexCmd.Flags().StringVar(&usageImportCodexEmployeeNo, "employee-no", "", "导入到指定员工")
	usageImportCodexCmd.Flags().StringVar(&usageImportCodexDate, "date", "", "导入指定日期 YYYY-MM-DD")
	usageImportCodexCmd.Flags().StringVar(&usageImportCodexFromDate, "from", "", "导入开始日期 YYYY-MM-DD")
	usageImportCodexCmd.Flags().StringVar(&usageImportCodexToDate, "to", "", "导入结束日期 YYYY-MM-DD")
	usageImportCodexCmd.Flags().StringVar(&usageImportCodexHome, "codex-home", "", "Codex home 路径；为空时使用 ccusage 默认值")
	usageImportCodexCmd.Flags().BoolVar(&usageImportCodexOffline, "offline", false, "传递 --offline 给 ccusage")
	usageImportCodexCmd.Flags().BoolVar(&usageImportCodexOverwrite, "overwrite", false, "覆盖已存在日期")
	usageImportCodexCmd.Flags().BoolVar(&usageImportCodexDryRun, "dry-run", false, "只解析并输出导入计划，不写入数据库")
	usageImportCmd.AddCommand(usageImportCodexCmd)
	usageCmd.AddCommand(usageImportCmd)
}

func runUsageImportCodex(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	dates, err := resolveUsageImportCodexDates()
	if err != nil {
		return err
	}

	database, cleanup, err := openLocalDatabaseForImport(ctx, usageImportCodexDryRun)
	if err != nil {
		return err
	}
	defer cleanup()
	if database != nil {
		defer database.Close()
	}
	if database == nil {
		return fmt.Errorf("usage import codex --dry-run 需要已初始化数据库，请先运行 codesome db init")
	}

	source := ccusageCodexDailySource{
		CodexHome: usageImportCodexHome,
		Offline:   usageImportCodexOffline,
	}
	results, err := usersync.NewCodexUsageImporter(database, source).Import(ctx, usersync.CodexUsageImportOptions{
		EmployeeNo: usageImportCodexEmployeeNo,
		Dates:      dates,
		Overwrite:  usageImportCodexOverwrite,
		DryRun:     usageImportCodexDryRun,
	})
	if err != nil {
		return err
	}
	printUsageImportCodexResults(results)
	if !usageImportCodexDryRun {
		if err := syncCodexImportResultsToFeishu(ctx, database, results); err != nil {
			return err
		}
	}
	return nil
}

type ccusageCodexDailySource struct {
	CodexHome string
	Offline   bool
}

func (s ccusageCodexDailySource) DailyUsage(ctx context.Context, dates []string) ([]usersync.CodexDailyUsage, error) {
	since, until, err := usageImportCodexDateBounds(dates)
	if err != nil {
		return nil, err
	}
	args := []string{
		"ccusage@latest",
		"codex",
		"daily",
		"--json",
		"--since", compactUsageDate(since),
		"--until", compactUsageDate(until),
		"--timezone", "Asia/Shanghai",
	}
	if s.Offline {
		args = append(args, "--offline")
	}
	command := exec.CommandContext(ctx, "npx", args...)
	command.Env = os.Environ()
	if s.CodexHome != "" {
		codexHome, err := expandHomePath(s.CodexHome)
		if err != nil {
			return nil, err
		}
		command.Env = append(command.Env, "CODEX_HOME="+codexHome)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("run ccusage codex daily: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return usersync.ParseCCUsageCodexDailyJSON(output)
}

func resolveUsageImportCodexDates() ([]string, error) {
	selectorCount := 0
	if usageImportCodexDate != "" {
		selectorCount++
	}
	if usageImportCodexFromDate != "" || usageImportCodexToDate != "" {
		if usageImportCodexFromDate == "" || usageImportCodexToDate == "" {
			return nil, fmt.Errorf("--from 和 --to 必须同时指定")
		}
		selectorCount++
	}
	if selectorCount != 1 {
		return nil, fmt.Errorf("必须且只能指定 --date、--from/--to 之一")
	}

	today := truncateDate(syncUsageNow())
	if usageImportCodexDate != "" {
		date, err := parseSyncUsageDate(usageImportCodexDate)
		if err != nil {
			return nil, err
		}
		if date.After(today) {
			return nil, fmt.Errorf("不能导入未来日期: %s", formatSyncUsageDate(date))
		}
		return []string{formatSyncUsageDate(date)}, nil
	}

	from, err := parseSyncUsageDate(usageImportCodexFromDate)
	if err != nil {
		return nil, err
	}
	to, err := parseSyncUsageDate(usageImportCodexToDate)
	if err != nil {
		return nil, err
	}
	if to.Before(from) {
		return nil, fmt.Errorf("--to 必须大于等于 --from")
	}

	var dates []string
	for date := from; !date.After(to); date = date.AddDate(0, 0, 1) {
		if date.After(today) {
			return nil, fmt.Errorf("不能导入未来日期: %s", formatSyncUsageDate(date))
		}
		dates = append(dates, formatSyncUsageDate(date))
	}
	return dates, nil
}

func usageImportCodexDateBounds(dates []string) (string, string, error) {
	if len(dates) == 0 {
		return "", "", fmt.Errorf("usage dates are required")
	}
	since := dates[0]
	until := dates[0]
	for _, date := range dates[1:] {
		if date < since {
			since = date
		}
		if date > until {
			until = date
		}
	}
	return since, until, nil
}

func compactUsageDate(date string) string {
	return strings.ReplaceAll(date, "-", "")
}

func expandHomePath(path string) (string, error) {
	if path == "~" {
		return os.UserHomeDir()
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil
	}
	return path, nil
}

func printUsageImportCodexResults(results []usersync.CodexUsageImportResult) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "DATE\tEMPLOYEE_NO\tSOURCE\tTOKENS\tCOST\tACTION")
	for _, result := range results {
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%.6f\t%s\n",
			result.UsageDate,
			result.EmployeeNo,
			result.Source,
			result.Tokens,
			result.Cost,
			result.Action,
		)
	}
	w.Flush()
}

func syncCodexImportResultsToFeishu(ctx context.Context, database *sql.DB, results []usersync.CodexUsageImportResult) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("加载配置失败: %w", err)
	}
	if !usersync.HasFeishuUsageConfig(cfg.GetFeishuConfig()) {
		return nil
	}
	client, err := provider.NewFeishuClient(cfg)
	if err != nil {
		return err
	}
	rows := make([]usersync.UsageSyncResult, 0, len(results))
	for _, result := range results {
		rows = append(rows, usersync.UsageSyncResult{
			UsageDate:       result.UsageDate,
			Source:          result.Source,
			SourceAccountID: result.SourceAccountID,
			KeyName:         "Codex 官方订阅",
			FeishuOpenID:    result.FeishuOpenID,
			TotalTokens:     result.Tokens,
			ActualCost:      result.Cost,
		})
	}
	feishuResults, err := usersync.SyncUsageToFeishu(ctx, cfg.GetFeishuConfig(), client, repository.NewFeishuUsageRecordRepository(database), rows)
	printFeishuUsageResults(feishuResults)
	return err
}
