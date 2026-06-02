package cmd

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"codesome-usage-manager/internal/config"
	"codesome-usage-manager/internal/provider"
	"codesome-usage-manager/internal/repository"
	importsync "codesome-usage-manager/internal/sync"
)

var (
	userAddEmployeeNo   string
	userAddName         string
	userAddTeam         string
	userAddGroupID      int
	userAddFeishuOpenID string

	userUpdateEmployeeNo   string
	userUpdateName         string
	userUpdateTeam         string
	userUpdateStatus       string
	userUpdateGroupID      int
	userUpdateClearGroup   bool
	userUpdateFeishuOpenID string

	userDeleteEmployeeNo string

	userImportFile   string
	userImportDryRun bool

	userImportFeishuDryRun bool
)

var userCmd = &cobra.Command{
	Use:   "user",
	Short: "管理本地用户",
}

var userAddCmd = &cobra.Command{
	Use:   "add",
	Short: "新增本地用户",
	RunE:  runUserAdd,
}

var userUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "更新本地用户",
	RunE:  runUserUpdate,
}

var userDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "软删除本地用户",
	RunE:  runUserDelete,
}

var userImportCmd = &cobra.Command{
	Use:   "import",
	Short: "从 CSV 批量导入本地用户",
	RunE:  runUserImport,
}

var userImportFeishuCmd = &cobra.Command{
	Use:   "import-feishu",
	Short: "从飞书多维表格批量导入本地用户",
	RunE:  runUserImportFeishu,
}

var userListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出本地用户",
	RunE:  runUserList,
}

func init() {
	userCmd.PersistentFlags().StringVar(&dbPath, "path", "", "SQLite 数据库路径")

	userAddCmd.Flags().StringVar(&userAddEmployeeNo, "employee-no", "", "员工稳定标识")
	userAddCmd.Flags().StringVar(&userAddName, "name", "", "用户展示名称")
	userAddCmd.Flags().StringVar(&userAddTeam, "team", "", "团队 code")
	userAddCmd.Flags().IntVar(&userAddGroupID, "group-id", 0, "个人级 Codesome group ID")
	userAddCmd.Flags().StringVar(&userAddFeishuOpenID, "feishu-open-id", "", "飞书 open_id")
	userAddCmd.MarkFlagRequired("employee-no")
	userAddCmd.MarkFlagRequired("name")
	userCmd.AddCommand(userAddCmd)

	userUpdateCmd.Flags().StringVar(&userUpdateEmployeeNo, "employee-no", "", "员工稳定标识")
	userUpdateCmd.Flags().StringVar(&userUpdateName, "name", "", "新的用户展示名称")
	userUpdateCmd.Flags().StringVar(&userUpdateTeam, "team", "", "新的团队 code")
	userUpdateCmd.Flags().StringVar(&userUpdateStatus, "status", "", "新的状态: active 或 inactive")
	userUpdateCmd.Flags().IntVar(&userUpdateGroupID, "group-id", 0, "新的个人级 Codesome group ID")
	userUpdateCmd.Flags().BoolVar(&userUpdateClearGroup, "clear-group-id", false, "清除个人级 Codesome group 覆盖")
	userUpdateCmd.Flags().StringVar(&userUpdateFeishuOpenID, "feishu-open-id", "", "新的飞书 open_id；传空字符串可清除")
	userUpdateCmd.MarkFlagRequired("employee-no")
	userUpdateCmd.MarkFlagsOneRequired("name", "team", "status", "group-id", "clear-group-id", "feishu-open-id")
	userUpdateCmd.MarkFlagsMutuallyExclusive("group-id", "clear-group-id")
	userCmd.AddCommand(userUpdateCmd)

	userDeleteCmd.Flags().StringVar(&userDeleteEmployeeNo, "employee-no", "", "员工稳定标识")
	userDeleteCmd.MarkFlagRequired("employee-no")
	userCmd.AddCommand(userDeleteCmd)

	userImportCmd.Flags().StringVar(&userImportFile, "file", "", "CSV 文件路径")
	userImportCmd.Flags().BoolVar(&userImportDryRun, "dry-run", false, "只输出导入计划，不写入数据库")
	userImportCmd.MarkFlagRequired("file")
	userCmd.AddCommand(userImportCmd)

	userImportFeishuCmd.Flags().BoolVar(&userImportFeishuDryRun, "dry-run", false, "只输出导入计划，不写入数据库")
	userCmd.AddCommand(userImportFeishuCmd)

	userCmd.AddCommand(userListCmd)
	rootCmd.AddCommand(userCmd)
}

func runUserAdd(cmd *cobra.Command, args []string) error {
	database, err := openLocalDatabase(context.Background())
	if err != nil {
		return err
	}
	defer database.Close()

	params := repository.CreateUserParams{
		EmployeeNo: userAddEmployeeNo,
		Name:       userAddName,
		TeamCode:   userAddTeam,
	}
	if cmd.Flags().Changed("group-id") {
		params.CodesomeGroupID = &userAddGroupID
	}
	if cmd.Flags().Changed("feishu-open-id") {
		params.FeishuOpenID = userAddFeishuOpenID
	}

	user, err := repository.NewUserRepository(database).Create(context.Background(), params)
	if err != nil {
		return err
	}
	fmt.Printf("用户已创建：employee_no=%s name=%s status=%s\n", user.EmployeeNo, user.Name, user.Status)
	return nil
}

func runUserUpdate(cmd *cobra.Command, args []string) error {
	database, err := openLocalDatabase(context.Background())
	if err != nil {
		return err
	}
	defer database.Close()

	params := repository.UpdateUserParams{}
	if cmd.Flags().Changed("name") {
		params.Name = &userUpdateName
	}
	if cmd.Flags().Changed("team") {
		params.TeamCode = &userUpdateTeam
	}
	if cmd.Flags().Changed("status") {
		params.Status = &userUpdateStatus
	}
	if cmd.Flags().Changed("group-id") {
		params.CodesomeGroupID = &userUpdateGroupID
	}
	if cmd.Flags().Changed("clear-group-id") {
		params.ClearGroupID = userUpdateClearGroup
	}
	if cmd.Flags().Changed("feishu-open-id") {
		params.FeishuOpenID = &userUpdateFeishuOpenID
	}

	user, err := repository.NewUserRepository(database).Update(context.Background(), userUpdateEmployeeNo, params)
	if err != nil {
		return err
	}
	fmt.Printf("用户已更新：employee_no=%s name=%s status=%s\n", user.EmployeeNo, user.Name, user.Status)
	return nil
}

func runUserDelete(cmd *cobra.Command, args []string) error {
	database, err := openLocalDatabase(context.Background())
	if err != nil {
		return err
	}
	defer database.Close()

	user, err := repository.NewUserRepository(database).SoftDelete(context.Background(), userDeleteEmployeeNo)
	if err != nil {
		return err
	}
	fmt.Printf("用户已删除：employee_no=%s status=%s\n", user.EmployeeNo, user.Status)
	return nil
}

func runUserImport(cmd *cobra.Command, args []string) error {
	file, err := os.Open(userImportFile)
	if err != nil {
		return fmt.Errorf("打开 CSV 失败: %w", err)
	}
	defer file.Close()

	database, cleanup, err := openLocalDatabaseForImport(context.Background(), userImportDryRun)
	if err != nil {
		return err
	}
	defer cleanup()
	if database == nil {
		return fmt.Errorf("user import --dry-run 需要已初始化数据库，请先运行 codesome db init")
	}
	if database != nil {
		defer database.Close()
	}

	results, err := importsync.NewUserCSVImporter(database).ImportCSV(context.Background(), file, importsync.ImportUsersOptions{
		DryRun: userImportDryRun,
	})
	if err != nil {
		return err
	}
	printUserImportResults(results)
	return nil
}

func runUserImportFeishu(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	database, cleanup, err := openLocalDatabaseForFeishuImport(ctx, userImportFeishuDryRun)
	if err != nil {
		return err
	}
	if database == nil {
		return fmt.Errorf("user import-feishu --dry-run 需要已初始化数据库，请先运行 codesome db init")
	}
	defer cleanup()
	defer database.Close()

	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}
	feishu := cfg.GetFeishuConfig()
	if feishu == nil {
		return fmt.Errorf("未找到 feishu 配置")
	}
	client, err := provider.NewFeishuClient(cfg)
	if err != nil {
		return err
	}

	results, err := importsync.NewUserFeishuImporter(database, client).Import(ctx, feishu, importsync.ImportUsersOptions{
		DryRun: userImportFeishuDryRun,
	})
	if err != nil {
		return err
	}
	printUserImportResults(results)
	return nil
}

func openLocalDatabaseForFeishuImport(ctx context.Context, dryRun bool) (*sql.DB, func(), error) {
	if !dryRun {
		database, err := openLocalDatabase(ctx)
		return database, func() {}, err
	}

	return openDryRunDatabaseCopy(ctx)
}

func copyOptionalFile(src string, dst string) error {
	if _, err := os.Stat(src); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("检查 dry-run 数据库附属文件失败: %w", err)
	}
	return copyFile(src, dst)
}

func copyFile(src string, dst string) error {
	input, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("打开 dry-run 数据库源文件失败: %w", err)
	}
	defer input.Close()

	output, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("创建 dry-run 数据库副本失败: %w", err)
	}
	if _, err := io.Copy(output, input); err != nil {
		output.Close()
		return fmt.Errorf("复制 dry-run 数据库失败: %w", err)
	}
	if err := output.Close(); err != nil {
		return fmt.Errorf("关闭 dry-run 数据库副本失败: %w", err)
	}
	return nil
}

func runUserList(cmd *cobra.Command, args []string) error {
	database, err := openLocalDatabase(context.Background())
	if err != nil {
		return err
	}
	defer database.Close()

	users, err := repository.NewUserRepository(database).List(context.Background())
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "EMPLOYEE_NO\tNAME\tTEAM\tSTATUS\tGROUP_ID\tFEISHU_OPEN_ID")
	for _, user := range users {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			user.EmployeeNo,
			user.Name,
			stringValue(user.TeamCode),
			user.Status,
			intValue(user.CodesomeGroupID),
			user.FeishuOpenID,
		)
	}
	return w.Flush()
}

func printUserImportResults(results []importsync.ImportUsersResult) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ACTION\tROW\tEMPLOYEE_NO\tNAME\tTEAM\tSTATUS\tGROUP_ID\tFEISHU_OPEN_ID")
	for _, result := range results {
		fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\t%s\t%s\t%s\n",
			result.Action,
			result.Row,
			result.EmployeeNo,
			result.Name,
			result.TeamCode,
			result.Status,
			intValue(result.CodesomeGroupID),
			result.FeishuOpenID,
		)
	}
	w.Flush()
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func intValue(value *int) string {
	if value == nil {
		return ""
	}
	return fmt.Sprintf("%d", *value)
}
