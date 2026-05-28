package cmd

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	codesomedb "codesome-usage-manager/internal/db"
	"codesome-usage-manager/internal/repository"
)

var (
	teamAddCode      string
	teamAddName      string
	teamUpdateCode   string
	teamUpdateName   string
	teamUpdateStatus string
)

var teamCmd = &cobra.Command{
	Use:   "team",
	Short: "管理本地团队",
}

var teamAddCmd = &cobra.Command{
	Use:   "add",
	Short: "新增本地团队",
	RunE:  runTeamAdd,
}

var teamUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "更新本地团队",
	RunE:  runTeamUpdate,
}

var teamListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出本地团队",
	RunE:  runTeamList,
}

func init() {
	teamCmd.PersistentFlags().StringVar(&dbPath, "path", "", "SQLite 数据库路径")

	teamAddCmd.Flags().StringVar(&teamAddCode, "code", "", "团队稳定标识")
	teamAddCmd.Flags().StringVar(&teamAddName, "name", "", "团队展示名称")
	teamAddCmd.MarkFlagRequired("code")
	teamAddCmd.MarkFlagRequired("name")
	teamCmd.AddCommand(teamAddCmd)

	teamUpdateCmd.Flags().StringVar(&teamUpdateCode, "code", "", "团队稳定标识")
	teamUpdateCmd.Flags().StringVar(&teamUpdateName, "name", "", "新的团队展示名称")
	teamUpdateCmd.Flags().StringVar(&teamUpdateStatus, "status", "", "新的状态: active 或 inactive")
	teamUpdateCmd.MarkFlagRequired("code")
	teamUpdateCmd.MarkFlagsOneRequired("name", "status")
	teamCmd.AddCommand(teamUpdateCmd)

	teamCmd.AddCommand(teamListCmd)
	rootCmd.AddCommand(teamCmd)
}

func runTeamAdd(cmd *cobra.Command, args []string) error {
	database, err := openLocalDatabase(context.Background())
	if err != nil {
		return err
	}
	defer database.Close()

	team, err := repository.NewTeamRepository(database).Create(context.Background(), teamAddCode, teamAddName)
	if err != nil {
		return err
	}
	fmt.Printf("团队已创建：code=%s name=%s status=%s\n", team.Code, team.Name, team.Status)
	return nil
}

func runTeamUpdate(cmd *cobra.Command, args []string) error {
	database, err := openLocalDatabase(context.Background())
	if err != nil {
		return err
	}
	defer database.Close()

	params := repository.UpdateTeamParams{}
	if cmd.Flags().Changed("name") {
		params.Name = &teamUpdateName
	}
	if cmd.Flags().Changed("status") {
		params.Status = &teamUpdateStatus
	}

	team, err := repository.NewTeamRepository(database).Update(context.Background(), teamUpdateCode, params)
	if err != nil {
		return err
	}
	fmt.Printf("团队已更新：code=%s name=%s status=%s\n", team.Code, team.Name, team.Status)
	return nil
}

func runTeamList(cmd *cobra.Command, args []string) error {
	database, err := openLocalDatabase(context.Background())
	if err != nil {
		return err
	}
	defer database.Close()

	teams, err := repository.NewTeamRepository(database).List(context.Background())
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "CODE\tNAME\tSTATUS")
	for _, team := range teams {
		fmt.Fprintf(w, "%s\t%s\t%s\n", team.Code, team.Name, team.Status)
	}
	return w.Flush()
}

func openLocalDatabase(ctx context.Context) (*sql.DB, error) {
	path, err := resolveDatabasePath()
	if err != nil {
		return nil, err
	}
	database, err := codesomedb.Open(path)
	if err != nil {
		return nil, err
	}
	if err := codesomedb.Migrate(ctx, database); err != nil {
		database.Close()
		return nil, err
	}
	return database, nil
}
