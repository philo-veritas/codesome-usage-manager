package cmd

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"codesome-usage-manager/internal/repository"
)

var (
	reportMonthlyMonth  string
	reportMonthlyTeam   string
	reportMonthlyOutput string
)

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "生成本地用量报表",
}

var reportMonthlyCmd = &cobra.Command{
	Use:   "monthly",
	Short: "生成月度用量报表",
	RunE:  runReportMonthly,
}

func init() {
	reportCmd.PersistentFlags().StringVar(&dbPath, "path", "", "SQLite 数据库路径")
	reportMonthlyCmd.Flags().StringVar(&reportMonthlyMonth, "month", "", "月份 YYYY-MM")
	reportMonthlyCmd.Flags().StringVar(&reportMonthlyTeam, "team", "", "只输出指定团队")
	reportMonthlyCmd.Flags().StringVar(&reportMonthlyOutput, "output", "", "CSV 输出路径；为空时输出表格到 stdout")
	reportMonthlyCmd.MarkFlagRequired("month")
	reportCmd.AddCommand(reportMonthlyCmd)
	rootCmd.AddCommand(reportCmd)
}

func runReportMonthly(cmd *cobra.Command, args []string) error {
	month, err := resolveReportMonth(reportMonthlyMonth)
	if err != nil {
		return err
	}

	database, err := openLocalDatabase(context.Background())
	if err != nil {
		return err
	}
	defer database.Close()

	rows, err := repository.NewUsageDailyRepository(database).MonthlyReport(context.Background(), month, reportMonthlyTeam)
	if err != nil {
		return err
	}

	if reportMonthlyOutput == "" {
		printMonthlyReport(rows)
		return nil
	}

	outputFile, err := createReportOutput(reportMonthlyOutput)
	if err != nil {
		return err
	}
	if err := writeMonthlyReportCSV(outputFile, rows); err != nil {
		outputFile.Close()
		return err
	}
	if err := outputFile.Close(); err != nil {
		return fmt.Errorf("关闭报表文件失败: %w", err)
	}
	fmt.Fprintf(os.Stderr, "已导出 %d 条月报：%s\n", len(rows), reportMonthlyOutput)
	return nil
}

func resolveReportMonth(value string) (string, error) {
	if _, err := time.Parse("2006-01", value); err != nil {
		return "", fmt.Errorf("month 必须是 YYYY-MM: %s", value)
	}
	return value, nil
}

func createReportOutput(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return nil, fmt.Errorf("创建报表文件失败: %w", err)
	}
	if err := file.Chmod(0600); err != nil {
		file.Close()
		return nil, fmt.Errorf("设置报表文件权限失败: %w", err)
	}
	return file, nil
}

func printMonthlyReport(rows []repository.MonthlyReportRow) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "MONTH\tTEAM\tUSER\tEMPLOYEE_NO\tREQUESTS\tTOKENS\tACTUAL_COST")
	for _, row := range rows {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%d\t%.6f\n",
			row.Month,
			stringValue(row.TeamCode),
			row.UserName,
			row.EmployeeNo,
			row.TotalRequests,
			row.TotalTokens,
			row.TotalActualCost,
		)
	}
	w.Flush()
}

func writeMonthlyReportCSV(writer io.Writer, rows []repository.MonthlyReportRow) error {
	csvWriter := csv.NewWriter(writer)
	if err := csvWriter.Write([]string{
		"month",
		"team",
		"user",
		"employee_no",
		"total_requests",
		"total_tokens",
		"total_actual_cost",
	}); err != nil {
		return fmt.Errorf("写入 CSV 表头失败: %w", err)
	}
	for _, row := range rows {
		if err := csvWriter.Write([]string{
			row.Month,
			stringValue(row.TeamCode),
			row.UserName,
			row.EmployeeNo,
			strconv.FormatInt(row.TotalRequests, 10),
			strconv.FormatInt(row.TotalTokens, 10),
			strconv.FormatFloat(row.TotalActualCost, 'f', 6, 64),
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
