package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"codesome-usage-manager/internal/repository"
)

var (
	userAddEmployeeNo string
	userAddName       string
	userAddTeam       string
	userAddGroupID    int

	userUpdateEmployeeNo string
	userUpdateName       string
	userUpdateTeam       string
	userUpdateStatus     string
	userUpdateGroupID    int
	userUpdateClearGroup bool

	userDeleteEmployeeNo string
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
	userAddCmd.MarkFlagRequired("employee-no")
	userAddCmd.MarkFlagRequired("name")
	userCmd.AddCommand(userAddCmd)

	userUpdateCmd.Flags().StringVar(&userUpdateEmployeeNo, "employee-no", "", "员工稳定标识")
	userUpdateCmd.Flags().StringVar(&userUpdateName, "name", "", "新的用户展示名称")
	userUpdateCmd.Flags().StringVar(&userUpdateTeam, "team", "", "新的团队 code")
	userUpdateCmd.Flags().StringVar(&userUpdateStatus, "status", "", "新的状态: active 或 inactive")
	userUpdateCmd.Flags().IntVar(&userUpdateGroupID, "group-id", 0, "新的个人级 Codesome group ID")
	userUpdateCmd.Flags().BoolVar(&userUpdateClearGroup, "clear-group-id", false, "清除个人级 Codesome group 覆盖")
	userUpdateCmd.MarkFlagRequired("employee-no")
	userUpdateCmd.MarkFlagsOneRequired("name", "team", "status", "group-id", "clear-group-id")
	userUpdateCmd.MarkFlagsMutuallyExclusive("group-id", "clear-group-id")
	userCmd.AddCommand(userUpdateCmd)

	userDeleteCmd.Flags().StringVar(&userDeleteEmployeeNo, "employee-no", "", "员工稳定标识")
	userDeleteCmd.MarkFlagRequired("employee-no")
	userCmd.AddCommand(userDeleteCmd)

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
	fmt.Fprintln(w, "EMPLOYEE_NO\tNAME\tTEAM\tSTATUS\tGROUP_ID")
	for _, user := range users {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			user.EmployeeNo,
			user.Name,
			stringValue(user.TeamCode),
			user.Status,
			intValue(user.CodesomeGroupID),
		)
	}
	return w.Flush()
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
