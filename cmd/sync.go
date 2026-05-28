package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"codesome-usage-manager/internal/config"
	"codesome-usage-manager/internal/provider"
	usersync "codesome-usage-manager/internal/sync"
)

var (
	syncUsersDryRun     bool
	syncUsersEmployeeNo string
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "同步本地管理数据",
}

var syncUsersCmd = &cobra.Command{
	Use:   "users",
	Short: "同步本地用户与 Codesome API Key",
	RunE:  runSyncUsers,
}

func init() {
	syncCmd.PersistentFlags().StringVar(&dbPath, "path", "", "SQLite 数据库路径")

	syncUsersCmd.Flags().BoolVar(&syncUsersDryRun, "dry-run", false, "只输出同步计划，不调用 Codesome")
	syncUsersCmd.Flags().StringVar(&syncUsersEmployeeNo, "employee-no", "", "只同步指定员工")
	syncCmd.AddCommand(syncUsersCmd)
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

	results, err := usersync.NewUserSyncer(database, service, defaultGroupID).SyncUsers(ctx, usersync.UserSyncOptions{
		DryRun:     syncUsersDryRun,
		EmployeeNo: syncUsersEmployeeNo,
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
