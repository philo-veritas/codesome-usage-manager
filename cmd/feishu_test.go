package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"

	"codesome-usage-manager/internal/provider"
	"codesome-usage-manager/internal/repository"
)

func TestFeishuSendKeysHasDatabasePathFlag(t *testing.T) {
	if flag := feishuSendKeysCmd.Flags().Lookup("path"); flag == nil {
		t.Fatal("expected feishu send-keys to expose --path")
	}
}

func TestSendFeishuKeysContinuesAfterSendError(t *testing.T) {
	rawKey := "sk-test"
	client := &fakeFeishuMessageClient{
		errors: map[string]error{
			"ou_alice": errors.New("rate limited"),
			"ou_bob":   errors.New("user not found"),
		},
	}
	rows := []repository.APIKeyExportRow{
		{
			EmployeeNo:    "E12345",
			UserName:      "Alice",
			FeishuOpenID:  "ou_alice",
			KeyName:       "Alice",
			CodesomeKeyID: 6732,
			RawKey:        &rawKey,
		},
		{
			EmployeeNo:    "E99999",
			UserName:      "Bob",
			FeishuOpenID:  "ou_bob",
			KeyName:       "Bob",
			CodesomeKeyID: 6733,
			RawKey:        &rawKey,
		},
	}

	results, err := sendFeishuKeys(context.Background(), client, rows, false)
	if err == nil {
		t.Fatal("expected summarized error")
	}
	errorText := err.Error()
	for _, want := range []string{
		"2/2 失败",
		"employee_no=E12345 name=Alice error=rate limited",
		"employee_no=E99999 name=Bob error=user not found",
	} {
		if !strings.Contains(errorText, want) {
			t.Fatalf("expected error to contain %q, got %v", want, err)
		}
	}
	if len(client.openIDs) != 2 || client.openIDs[0] != "ou_alice" || client.openIDs[1] != "ou_bob" {
		t.Fatalf("expected both messages to be attempted, got %+v", client.openIDs)
	}
	if len(results) != 2 || results[0].Action != "error" || results[1].Action != "error" {
		t.Fatalf("unexpected results: %+v", results)
	}
}

type fakeFeishuMessageClient struct {
	errors  map[string]error
	openIDs []string
}

func (c *fakeFeishuMessageClient) SendTextMessage(ctx context.Context, openID string, text string) (*provider.FeishuMessageResult, error) {
	c.openIDs = append(c.openIDs, openID)
	if err := c.errors[openID]; err != nil {
		return nil, err
	}
	return &provider.FeishuMessageResult{MessageID: "msg_" + openID}, nil
}
