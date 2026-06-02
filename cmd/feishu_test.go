package cmd

import "testing"

func TestFeishuSendKeysHasDatabasePathFlag(t *testing.T) {
	if flag := feishuSendKeysCmd.Flags().Lookup("path"); flag == nil {
		t.Fatal("expected feishu send-keys to expose --path")
	}
}
