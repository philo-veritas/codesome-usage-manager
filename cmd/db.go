package cmd

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"codesome-usage-manager/internal/config"
	codesomedb "codesome-usage-manager/internal/db"
	"codesome-usage-manager/internal/provider"
	importsync "codesome-usage-manager/internal/sync"
)

var dbPath string
var dbImportConfigKeysDryRun bool
var dbImportConfigKeysGroupID int
var dbImportRemoteKeysDryRun bool

var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "管理本地 SQLite 数据库",
}

var dbInitCmd = &cobra.Command{
	Use:   "init",
	Short: "初始化本地 SQLite 数据库",
	RunE:  runDBInit,
}

var dbMigrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "执行本地 SQLite 数据库迁移",
	RunE:  runDBMigrate,
}

var dbImportConfigKeysCmd = &cobra.Command{
	Use:   "import-config-keys",
	Short: "把 config.yaml 中的 API Key 清单导入本地数据库",
	RunE:  runDBImportConfigKeys,
}

var dbImportRemoteKeysCmd = &cobra.Command{
	Use:   "import-remote-keys",
	Short: "从 Codesome API 导入远程 API Key 清单到本地数据库",
	RunE:  runDBImportRemoteKeys,
}

func init() {
	dbCmd.PersistentFlags().StringVar(&dbPath, "path", "", "SQLite 数据库路径")
	dbCmd.AddCommand(dbInitCmd)
	dbCmd.AddCommand(dbMigrateCmd)
	dbImportRemoteKeysCmd.Flags().BoolVar(&dbImportRemoteKeysDryRun, "dry-run", false, "只输出导入计划，不写入数据库")
	dbCmd.AddCommand(dbImportRemoteKeysCmd)
	dbImportConfigKeysCmd.Flags().BoolVar(&dbImportConfigKeysDryRun, "dry-run", false, "只输出导入计划，不写入数据库")
	dbImportConfigKeysCmd.Flags().IntVar(&dbImportConfigKeysGroupID, "group-id", 0, "导入 legacy key 使用的 Codesome group ID")
	dbCmd.AddCommand(dbImportConfigKeysCmd)
	rootCmd.AddCommand(dbCmd)
}

func runDBInit(cmd *cobra.Command, args []string) error {
	path, err := resolveDatabasePath()
	if err != nil {
		return err
	}
	if err := codesomedb.Init(context.Background(), path); err != nil {
		return err
	}
	fmt.Printf("数据库已初始化：%s\n", path)
	return nil
}

func runDBMigrate(cmd *cobra.Command, args []string) error {
	path, err := resolveDatabasePath()
	if err != nil {
		return err
	}
	database, err := codesomedb.Open(path)
	if err != nil {
		return err
	}
	defer database.Close()

	if err := codesomedb.Migrate(context.Background(), database); err != nil {
		return err
	}
	fmt.Printf("数据库迁移完成：%s\n", path)
	return nil
}

func runDBImportConfigKeys(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}

	database, cleanup, err := openLocalDatabaseForImport(context.Background(), dbImportConfigKeysDryRun)
	if err != nil {
		return err
	}
	defer cleanup()
	if database != nil {
		defer database.Close()
	}

	results, err := importsync.NewConfigKeyImporter(database).Import(context.Background(), cfg, importsync.ImportConfigKeysOptions{
		DryRun:  dbImportConfigKeysDryRun,
		GroupID: dbImportConfigKeysGroupID,
	})
	if err != nil {
		return err
	}
	printDBImportConfigKeysResults(results)
	return nil
}

func runDBImportRemoteKeys(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}

	remoteKeys, err := provider.ListCodesomeKeys(cfg, true)
	if err != nil {
		return fmt.Errorf("获取远程 API Key 失败: %w", err)
	}

	database, cleanup, err := openLocalDatabaseForImport(context.Background(), dbImportRemoteKeysDryRun)
	if err != nil {
		return err
	}
	defer cleanup()
	if database != nil {
		defer database.Close()
	}

	results, err := importsync.NewRemoteKeyImporter(database).Import(context.Background(), remoteKeys, importsync.ImportRemoteKeysOptions{
		DryRun: dbImportRemoteKeysDryRun,
	})
	if err != nil {
		return err
	}
	printDBImportRemoteKeysResults(results)
	return nil
}

func openLocalDatabaseForImport(ctx context.Context, dryRun bool) (*sql.DB, func(), error) {
	if !dryRun {
		database, err := openLocalDatabase(ctx)
		return database, func() {}, err
	}
	return openDryRunDatabaseCopy(ctx)
}

func openDryRunDatabaseCopy(ctx context.Context) (*sql.DB, func(), error) {
	path, err := resolveDatabasePath()
	if err != nil {
		return nil, nil, err
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, func() {}, nil
		}
		return nil, nil, fmt.Errorf("检查数据库失败: %w", err)
	}

	tempDir, err := os.MkdirTemp("", "codesome-import-dry-run-*")
	if err != nil {
		return nil, nil, fmt.Errorf("创建 dry-run 临时目录失败: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tempDir) }
	tempPath := filepath.Join(tempDir, filepath.Base(path))
	if err := copyFile(path, tempPath); err != nil {
		cleanup()
		return nil, nil, err
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := copyOptionalFile(path+suffix, tempPath+suffix); err != nil {
			cleanup()
			return nil, nil, err
		}
	}

	database, err := codesomedb.Open(tempPath)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	if err := codesomedb.Migrate(ctx, database); err != nil {
		database.Close()
		cleanup()
		return nil, nil, err
	}
	return database, cleanup, nil
}

func printDBImportConfigKeysResults(results []importsync.ImportConfigKeysResult) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ACTION\tEMPLOYEE_NO\tNAME\tKEY_ID\tGROUP_ID")
	for _, result := range results {
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%d\n",
			result.Action,
			result.EmployeeNo,
			result.UserName,
			result.CodesomeKeyID,
			result.GroupID,
		)
	}
	w.Flush()
}

func printDBImportRemoteKeysResults(results []importsync.ImportRemoteKeysResult) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ACTION\tEMPLOYEE_NO\tNAME\tKEY_ID\tGROUP_ID")
	for _, result := range results {
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%d\n",
			result.Action,
			result.EmployeeNo,
			result.UserName,
			result.CodesomeKeyID,
			result.GroupID,
		)
	}
	w.Flush()
}

func resolveDatabasePath() (string, error) {
	if dbPath != "" {
		return dbPath, nil
	}
	if _, err := os.Stat("config.yaml"); err != nil {
		if os.IsNotExist(err) {
			return config.DefaultDatabasePath, nil
		}
		return "", fmt.Errorf("检查 config.yaml 失败: %w", err)
	}
	cfg, err := config.LoadConfig()
	if err != nil {
		return "", fmt.Errorf("加载配置失败: %w", err)
	}
	return cfg.DatabasePath(), nil
}
