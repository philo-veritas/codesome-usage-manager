package cmd

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"codesome-usage-manager/internal/config"
	"codesome-usage-manager/internal/provider"
)

const (
	defaultP80DailyBurnUSD = 361.34
	defaultMinInterval     = 2 * time.Minute
	defaultMaxInterval     = 2 * time.Hour
)

var (
	autoSwitchAll          bool
	autoSwitchMinRemaining float64
	autoSwitchMinInterval  time.Duration
	autoSwitchMaxInterval  time.Duration
)

var p80HourlyProfile = []float64{
	1.04, 0.26, 0.15, 0.33, 0.03, 0.02,
	0.00, 0.37, 1.20, 18.87, 25.06, 24.55,
	7.17, 13.80, 27.41, 25.30, 29.20, 26.52,
	13.13, 6.27, 3.27, 2.72, 2.23, 2.16,
}

var autoSwitchCmd = &cobra.Command{
	Use:   "auto-switch",
	Short: "常驻自动切换 Codesome group",
	Long: `常驻检查 Codesome active subscription 剩余额度，并自动切换配置中的 API Key。
启动和每天开始时会优先切到剩余额度最高的 group；日内低于阈值时再切到剩余额度最高的 group。`,
	RunE: runAutoSwitch,
}

func init() {
	autoSwitchCmd.Flags().BoolVar(&autoSwitchAll, "all", false, "检查并切换 legacy api_key_ids 中的所有 API Key")
	autoSwitchCmd.Flags().Float64Var(&autoSwitchMinRemaining, "min-remaining", 10, "当前 group 剩余额度低于该 USD 阈值时切换")
	autoSwitchCmd.Flags().DurationVar(&autoSwitchMinInterval, "min-interval", defaultMinInterval, "最短检查间隔")
	autoSwitchCmd.Flags().DurationVar(&autoSwitchMaxInterval, "max-interval", defaultMaxInterval, "最长检查间隔")
	autoSwitchCmd.MarkFlagRequired("all")
	rootCmd.AddCommand(autoSwitchCmd)
}

func runAutoSwitch(cmd *cobra.Command, args []string) error {
	if !autoSwitchAll {
		return fmt.Errorf("auto-switch 当前只支持 --all")
	}
	if autoSwitchMinRemaining < 0 {
		return fmt.Errorf("min-remaining 必须大于等于 0")
	}
	if autoSwitchMinInterval <= 0 {
		return fmt.Errorf("min-interval 必须大于 0")
	}
	if autoSwitchMaxInterval < autoSwitchMinInterval {
		return fmt.Errorf("max-interval 必须大于等于 min-interval")
	}

	cfg, err := loadCodesomeConfig()
	if err != nil {
		return err
	}
	codesome := cfg.GetCodesomeConfig()
	if len(codesome.ApiKeyIDs) == 0 {
		return fmt.Errorf("未配置 legacy api_key_ids")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cst := time.FixedZone("CST", 8*3600)
	currentDate := time.Now().In(cst).Format("2006-01-02")
	logf := autoSwitchPrintf(os.Stdout, cst)
	errorf := autoSwitchPrintf(os.Stderr, cst)

	logf("auto-switch 启动：keys=%d，min-remaining=$%.2f，interval=%s..%s\n",
		len(codesome.ApiKeyIDs), autoSwitchMinRemaining, autoSwitchMinInterval, autoSwitchMaxInterval)

	remaining, err := alignKeysToBestGroup(cfg, codesome.ApiKeyIDs, logf)
	if err != nil {
		return err
	}

	var previousRemaining float64
	var previousAt time.Time
	var observedBurnUSDPerHour float64
	if remaining > 0 {
		previousRemaining = remaining
		previousAt = time.Now()
	}

	for {
		now := time.Now()
		interval := nextAutoSwitchInterval(
			remaining,
			autoSwitchMinRemaining,
			now.In(cst),
			observedBurnUSDPerHour,
			autoSwitchMinInterval,
			autoSwitchMaxInterval,
		)
		if untilNextDay := durationUntilNextDay(now.In(cst)); untilNextDay > 0 && untilNextDay < interval {
			interval = untilNextDay
		}
		logf("下次检查：%s 后（当前估算剩余 $%.2f）\n", interval, remaining)

		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			logf("auto-switch 已停止\n")
			return nil
		case <-timer.C:
		}

		now = time.Now()
		today := now.In(cst).Format("2006-01-02")
		if today != currentDate {
			currentDate = today
			logf("检测到新的一天 %s，重新切到剩余额度最高的 group\n", currentDate)
			remaining, err = alignKeysToBestGroup(cfg, codesome.ApiKeyIDs, logf)
		} else {
			remaining, err = switchKeysOnExhausted(cfg, codesome.ApiKeyIDs, autoSwitchMinRemaining, logf)
		}
		if err != nil {
			errorf("auto-switch 检查失败：%v\n", err)
			remaining = 0
		}

		if remaining > 0 && previousRemaining > remaining && !previousAt.IsZero() {
			elapsedHours := now.Sub(previousAt).Hours()
			if elapsedHours > 0 {
				observedBurnUSDPerHour = math.Max(observedBurnUSDPerHour, (previousRemaining-remaining)/elapsedHours)
			}
		}
		if remaining > 0 {
			previousRemaining = remaining
			previousAt = now
		}
	}
}

func autoSwitchPrintf(w io.Writer, loc *time.Location) func(string, ...any) {
	return func(format string, args ...any) {
		timestamp := time.Now().In(loc).Format("2006-01-02 15:04:05")
		allArgs := append([]any{timestamp}, args...)
		fmt.Fprintf(w, "[%s] "+format, allArgs...)
	}
}

func alignKeysToBestGroup(cfg *config.Config, keys []config.CodesomeApiKeyId, printf func(string, ...any)) (float64, error) {
	results, err := provider.SwitchCodesomeKeysToBestGroup(cfg, keys)
	if err != nil {
		return 0, err
	}
	if hasErrors := printGroupSwitchBatchResultsWith(results, printf); hasErrors {
		return remainingFromSwitchResults(results), fmt.Errorf("部分 API Key 切换到最佳 group 失败")
	}
	return remainingFromSwitchResults(results), nil
}

func switchKeysOnExhausted(cfg *config.Config, keys []config.CodesomeApiKeyId, minRemainingUSD float64, printf func(string, ...any)) (float64, error) {
	results, err := provider.SwitchCodesomeKeysGroupOnExhausted(cfg, keys, minRemainingUSD)
	if err != nil {
		return 0, err
	}
	if hasErrors := printGroupSwitchBatchResultsWith(results, printf); hasErrors {
		return remainingFromSwitchResults(results), fmt.Errorf("部分 API Key 自动切换 group 失败")
	}
	return remainingFromSwitchResults(results), nil
}

func remainingFromSwitchResults(results []provider.CodesomeGroupSwitchBatchResult) float64 {
	remaining := 0.0
	for _, item := range results {
		if item.Result == nil {
			continue
		}
		candidate := item.Result.CurrentRemainingUSD
		if item.Result.Switched && item.Result.TargetRemainingUSD > 0 {
			candidate = item.Result.TargetRemainingUSD
		}
		if candidate > remaining {
			remaining = candidate
		}
	}
	return remaining
}

func nextAutoSwitchInterval(
	remainingUSD float64,
	minRemainingUSD float64,
	now time.Time,
	observedBurnUSDPerHour float64,
	minInterval time.Duration,
	maxInterval time.Duration,
) time.Duration {
	if minInterval <= 0 {
		minInterval = defaultMinInterval
	}
	if maxInterval < minInterval {
		maxInterval = minInterval
	}
	if remainingUSD <= minRemainingUSD {
		return minInterval
	}

	burnUSDPerHour := math.Max(predictedBurnUSDPerHour(now), observedBurnUSDPerHour)
	if burnUSDPerHour <= 0 {
		return maxInterval
	}

	hoursToThreshold := (remainingUSD - minRemainingUSD) / burnUSDPerHour
	interval := time.Duration(hoursToThreshold * float64(time.Hour) / 3)
	if interval < minInterval {
		return minInterval
	}
	if interval > maxInterval {
		return maxInterval
	}
	return interval
}

func predictedBurnUSDPerHour(now time.Time) float64 {
	if len(p80HourlyProfile) != 24 {
		return defaultP80DailyBurnUSD / 24
	}
	total := 0.0
	for _, value := range p80HourlyProfile {
		total += value
	}
	if total <= 0 {
		return defaultP80DailyBurnUSD / 24
	}
	hour := now.Hour()
	if hour < 0 || hour >= len(p80HourlyProfile) {
		return defaultP80DailyBurnUSD / 24
	}
	scaled := p80HourlyProfile[hour] * defaultP80DailyBurnUSD / total
	return math.Max(scaled, defaultP80DailyBurnUSD/24)
}

func durationUntilNextDay(now time.Time) time.Duration {
	nextDay := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
	return nextDay.Sub(now)
}
