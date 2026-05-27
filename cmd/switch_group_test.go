package cmd

import (
	"fmt"
	"testing"

	"usage-cli/internal/provider"
)

func TestPrintSubscriptionUsageSummary(t *testing.T) {
	var got string
	printSubscriptionUsageSummaryWith(
		provider.CodesomeSubscriptionUsageSummary{
			RemainingUSD: 410,
			LimitUSD:     450,
		},
		func(format string, args ...any) {
			got += fmt.Sprintf(format, args...)
		},
	)

	want := "今日总余额：$410.00 / $450.00\n"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
