package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"codesome-usage-manager/internal/provider"
	"codesome-usage-manager/internal/repository"
)

var (
	usageTodayIncludeInactive bool
	usageTodaySortByCost      bool
	getCodesomeKeysDailyUsage = provider.GetCodesomeKeysDailyUsage
)

type usageTodayResult struct {
	target     repository.APIKeyUsageTarget
	usage      provider.CodesomeKeyUsage
	usageFound bool
}

var usageCmd = &cobra.Command{
	Use:   "usage",
	Short: "查询 Codesome 用量",
}

var usageTodayCmd = &cobra.Command{
	Use:   "today",
	Short: "查询本地 API Key 今日用量",
	RunE:  runUsageToday,
}

func init() {
	usageCmd.PersistentFlags().StringVar(&dbPath, "path", "", "SQLite 数据库路径")
	usageTodayCmd.Flags().BoolVar(&usageTodayIncludeInactive, "include-inactive", false, "包含 inactive 用户或 key")
	usageTodayCmd.Flags().BoolVar(&usageTodaySortByCost, "sort-by-today-cost", false, "按 TODAY_ACTUAL_COST 倒序排列")
	usageCmd.AddCommand(usageTodayCmd)
	rootCmd.AddCommand(usageCmd)
}

func runUsageToday(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	cfg, err := loadCodesomeConfig()
	if err != nil {
		return err
	}

	database, err := openLocalDatabase(ctx)
	if err != nil {
		return err
	}
	defer database.Close()

	targets, err := repository.NewAPIKeyRepository(database).ListDailyUsageTargets(ctx, repository.ListAPIKeyDailyUsageTargetsParams{
		IncludeInactive: usageTodayIncludeInactive,
	})
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return fmt.Errorf("本地数据库未找到 API Key，请先运行 codesome db import-remote-keys 或 codesome sync users")
	}

	keyIDs := make([]int, 0, len(targets))
	for _, target := range targets {
		keyIDs = append(keyIDs, target.CodesomeKeyID)
	}
	usageMap, err := getCodesomeKeysDailyUsage(cfg, keyIDs)
	if err != nil {
		return fmt.Errorf("查询今日用量失败: %w", err)
	}

	results := buildUsageTodayResults(targets, usageMap)
	if usageTodaySortByCost {
		sortUsageTodayResultsByCost(results)
	}
	printUsageTodayResults(os.Stdout, results)
	return nil
}

func buildUsageTodayResults(targets []repository.APIKeyUsageTarget, usageMap map[int]provider.CodesomeKeyUsage) []usageTodayResult {
	results := make([]usageTodayResult, 0, len(targets))
	for _, target := range targets {
		usage, ok := usageMap[target.CodesomeKeyID]
		results = append(results, usageTodayResult{
			target:     target,
			usage:      usage,
			usageFound: ok,
		})
	}
	return results
}

func sortUsageTodayResultsByCost(results []usageTodayResult) {
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].usage.TodayCost > results[j].usage.TodayCost
	})
}

func printUsageTodayResults(writer io.Writer, results []usageTodayResult) {
	w := tabwriter.NewWriter(writer, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "KEY_ID\tKEY_NAME\tUSER_STATUS\tUSAGE_STATUS\tTODAY_ACTUAL_COST\tTOTAL_ACTUAL_COST")
	for _, result := range results {
		usageStatus := "ok"
		if !result.usageFound {
			usageStatus = "remote_missing"
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%.6f\t%.6f\n",
			result.target.CodesomeKeyID,
			result.target.Name,
			result.target.UserStatus,
			usageStatus,
			result.usage.TodayCost,
			result.usage.TotalCost,
		)
	}
	w.Flush()
}
