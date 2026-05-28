package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"codesome-usage-manager/internal/config"
	codesomedb "codesome-usage-manager/internal/db"
)

var dbPath string

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

func init() {
	dbCmd.PersistentFlags().StringVar(&dbPath, "path", "", "SQLite 数据库路径")
	dbCmd.AddCommand(dbInitCmd)
	dbCmd.AddCommand(dbMigrateCmd)
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
