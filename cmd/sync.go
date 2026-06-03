package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"codesome-usage-manager/internal/config"
	"codesome-usage-manager/internal/provider"
	usersync "codesome-usage-manager/internal/sync"
)

var (
	syncUsersDryRun     bool
	syncUsersEmployeeNo string
	syncUsersFull       bool

	syncUsageDate         string
	syncUsageFromDate     string
	syncUsageToDate       string
	syncUsageYesterday    bool
	syncUsageIncludeToday bool
	syncUsageForceUpdate  bool
)

var syncUsageNow = func() time.Time {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	return time.Now().In(loc)
}

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "同步本地管理数据",
}

var syncUsersCmd = &cobra.Command{
	Use:   "users",
	Short: "同步本地用户与 Codesome API Key",
	RunE:  runSyncUsers,
}

var syncUsageCmd = &cobra.Command{
	Use:   "usage",
	Short: "同步本地 API Key 历史用量",
	RunE:  runSyncUsage,
}

func init() {
	syncCmd.PersistentFlags().StringVar(&dbPath, "path", "", "SQLite 数据库路径")

	syncUsersCmd.Flags().BoolVar(&syncUsersDryRun, "dry-run", false, "只输出同步计划，不创建或更新 Codesome key")
	syncUsersCmd.Flags().StringVar(&syncUsersEmployeeNo, "employee-no", "", "只同步指定员工")
	syncUsersCmd.Flags().BoolVar(&syncUsersFull, "full", false, "全量收敛所有匹配的本地用户，重新应用期望状态")
	syncCmd.AddCommand(syncUsersCmd)

	syncUsageCmd.Flags().StringVar(&syncUsageDate, "date", "", "同步指定日期 YYYY-MM-DD")
	syncUsageCmd.Flags().StringVar(&syncUsageFromDate, "from", "", "同步开始日期 YYYY-MM-DD")
	syncUsageCmd.Flags().StringVar(&syncUsageToDate, "to", "", "同步结束日期 YYYY-MM-DD")
	syncUsageCmd.Flags().BoolVar(&syncUsageYesterday, "yesterday", false, "同步昨天")
	syncUsageCmd.Flags().BoolVar(&syncUsageIncludeToday, "include-today", false, "允许同步今天")
	syncUsageCmd.Flags().BoolVar(&syncUsageForceUpdate, "force-update", false, "强制刷新远程数据")
	syncCmd.AddCommand(syncUsageCmd)

	rootCmd.AddCommand(syncCmd)
}

func runSyncUsers(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	database, err := openLocalDatabase(ctx)
	if err != nil {
		return err
	}
	defer database.Close()

	cfg, err := loadSyncUsersConfig(syncUsersDryRun)
	if err != nil {
		return err
	}

	var service usersync.UserKeyService
	if !syncUsersDryRun {
		service = codesomeUserKeyService{cfg: cfg}
	}

	defaultGroupID := 0
	if codesome := cfg.GetCodesomeConfig(); codesome != nil {
		defaultGroupID = codesome.DefaultGroupID
	}

	syncer := usersync.NewUserSyncer(database, service, defaultGroupID)
	if cfg.GetCodesomeConfig() != nil {
		syncer.WithDefaultGroupIDResolver(func(ctx context.Context) (int, error) {
			return provider.BestCodesomeGroupID(cfg)
		})
	}

	results, err := syncer.SyncUsers(ctx, usersync.UserSyncOptions{
		DryRun:     syncUsersDryRun,
		EmployeeNo: syncUsersEmployeeNo,
		Full:       syncUsersFull,
	})
	if err != nil {
		return err
	}
	printSyncUserResults(results)
	return nil
}

func loadSyncUsersConfig(dryRun bool) (*config.Config, error) {
	if !dryRun {
		return loadCodesomeConfig()
	}

	cfg, err := config.LoadConfig()
	if err == nil {
		return cfg, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return &config.Config{}, nil
	}
	return nil, fmt.Errorf("加载配置失败: %w", err)
}

type codesomeUserKeyService struct {
	cfg *config.Config
}

func (s codesomeUserKeyService) CreateKey(ctx context.Context, name string, groupID int) (*provider.CodesomeApiKeyWithSecret, error) {
	return provider.CreateCodesomeKey(s.cfg, name, groupID)
}

func (s codesomeUserKeyService) UpdateKey(ctx context.Context, keyID int, update provider.CodesomeKeyUpdate) (*provider.CodesomeApiKey, error) {
	return provider.UpdateCodesomeKey(s.cfg, keyID, update)
}

func runSyncUsage(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	dates, err := resolveSyncUsageDates()
	if err != nil {
		return err
	}

	database, err := openLocalDatabase(ctx)
	if err != nil {
		return err
	}
	defer database.Close()

	cfg, err := loadCodesomeConfig()
	if err != nil {
		return err
	}

	results, err := usersync.NewUsageSyncer(database, codesomeUsageStatsService{cfg: cfg}).SyncUsage(ctx, usersync.UsageSyncOptions{
		Dates:       dates,
		ForceUpdate: syncUsageForceUpdate,
	})
	if err != nil {
		return err
	}
	printSyncUsageResults(results)
	return nil
}

type codesomeUsageStatsService struct {
	cfg *config.Config
}

func (s codesomeUsageStatsService) GetUsageStats(ctx context.Context, keyID int, usageDate string, forceUpdate bool) (*provider.CodesomeUsageStats, error) {
	return provider.GetCodesomeKeyUsageStats(s.cfg, keyID, usageDate, usageDate, forceUpdate)
}

func printSyncUserResults(results []usersync.UserSyncResult) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "EMPLOYEE_NO\tNAME\tACTION\tKEY_ID\tGROUP_ID\tMESSAGE")
	for _, result := range results {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			result.EmployeeNo,
			result.UserName,
			result.Action,
			intValue(emptyZero(result.CodesomeKeyID)),
			intValue(emptyZero(result.GroupID)),
			result.Message,
		)
	}
	w.Flush()
}

func emptyZero(value int) *int {
	if value == 0 {
		return nil
	}
	return &value
}

func resolveSyncUsageDates() ([]string, error) {
	selectorCount := 0
	if syncUsageDate != "" {
		selectorCount++
	}
	if syncUsageFromDate != "" || syncUsageToDate != "" {
		if syncUsageFromDate == "" || syncUsageToDate == "" {
			return nil, fmt.Errorf("--from 和 --to 必须同时指定")
		}
		selectorCount++
	}
	if syncUsageYesterday {
		selectorCount++
	}
	if selectorCount != 1 {
		return nil, fmt.Errorf("必须且只能指定 --date、--from/--to、--yesterday 之一")
	}

	today := truncateDate(syncUsageNow())
	var dates []time.Time
	if syncUsageYesterday {
		dates = []time.Time{today.AddDate(0, 0, -1)}
	} else if syncUsageDate != "" {
		date, err := parseSyncUsageDate(syncUsageDate)
		if err != nil {
			return nil, err
		}
		dates = []time.Time{date}
	} else {
		from, err := parseSyncUsageDate(syncUsageFromDate)
		if err != nil {
			return nil, err
		}
		to, err := parseSyncUsageDate(syncUsageToDate)
		if err != nil {
			return nil, err
		}
		if to.Before(from) {
			return nil, fmt.Errorf("--to 必须大于等于 --from")
		}
		for date := from; !date.After(to); date = date.AddDate(0, 0, 1) {
			dates = append(dates, date)
		}
	}

	result := make([]string, 0, len(dates))
	for _, date := range dates {
		if date.After(today) {
			return nil, fmt.Errorf("不能同步未来日期: %s", formatSyncUsageDate(date))
		}
		if date.Equal(today) && !syncUsageIncludeToday {
			continue
		}
		result = append(result, formatSyncUsageDate(date))
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("没有可同步日期；今天默认跳过，如需同步请传 --include-today")
	}
	return result, nil
}

func parseSyncUsageDate(value string) (time.Time, error) {
	date, err := time.ParseInLocation("2006-01-02", value, syncUsageNow().Location())
	if err != nil {
		return time.Time{}, fmt.Errorf("日期必须是 YYYY-MM-DD: %s", value)
	}
	return date, nil
}

func truncateDate(value time.Time) time.Time {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, value.Location())
}

func formatSyncUsageDate(value time.Time) string {
	return value.Format("2006-01-02")
}

func printSyncUsageResults(results []usersync.UsageSyncResult) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "DATE\tKEY_ID\tKEY_NAME\tREQUESTS\tTOKENS\tACTUAL_COST")
	for _, result := range results {
		fmt.Fprintf(w, "%s\t%d\t%s\t%d\t%d\t%.6f\n",
			result.UsageDate,
			result.CodesomeKeyID,
			result.KeyName,
			result.TotalRequests,
			result.TotalTokens,
			result.ActualCost,
		)
	}
	w.Flush()
}
