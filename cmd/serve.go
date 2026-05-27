package cmd

import (
	"fmt"
	"log"
	"net/http"

	"github.com/spf13/cobra"

	"usage-cli/internal/config"
	"usage-cli/internal/server"
)

var (
	host string
	port int
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "启动 HTTP API 服务",
	Long:  "启动 HTTP API 服务，提供 /api/cost 端点查询剩余额度",
	RunE:  runServer,
}

func init() {
	serveCmd.Flags().StringVar(&host, "host", "127.0.0.1", "服务监听地址")
	serveCmd.Flags().IntVarP(&port, "port", "p", 8080, "服务端口号")
	rootCmd.AddCommand(serveCmd)
}

func runServer(cmd *cobra.Command, args []string) error {
	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}

	// Register routes
	http.HandleFunc("/api/cost", server.CostHandler(cfg))
	http.HandleFunc("/api/codesome/usage", server.UsageHandler(cfg))
	http.HandleFunc("/api/codesome/usage-stats", server.UsageStatsHandler(cfg))
	http.HandleFunc("/api/codesome/keys", server.KeysHandler(cfg))
	http.HandleFunc("/api/codesome/daily-usage", server.DailyUsageHandler(cfg))
	http.HandleFunc("/api/codesome/reset-quota", server.ResetQuotaHandler(cfg))
	http.HandleFunc("/api/codesome/reset-all-quotas", server.ResetAllQuotasHandler(cfg))
	http.HandleFunc("/api/codesome/switch-group", server.SwitchGroupHandler(cfg))
	http.HandleFunc("/api/codesome/switch-on-exhausted", server.SwitchOnExhaustedHandler(cfg))

	addr := fmt.Sprintf("%s:%d", host, port)
	log.Printf("服务启动在 %s", addr)
	return http.ListenAndServe(addr, nil)
}
