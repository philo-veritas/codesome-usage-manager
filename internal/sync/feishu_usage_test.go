package syncer

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"codesome-usage-manager/internal/config"
	"codesome-usage-manager/internal/provider"
)

func TestFeishuUsageFieldsUsesConfiguredFieldNames(t *testing.T) {
	fields, err := FeishuUsageFields(UsageSyncResult{
		UsageDate:     "2026-06-11",
		CodesomeKeyID: 6732,
		FeishuOpenID:  "ou_alice",
		TotalTokens:   20099584,
		ActualCost:    30.6706521,
	})
	if err != nil {
		t.Fatalf("build fields: %v", err)
	}

	if fields["ID"] != "2026-06-11#6732" {
		t.Fatalf("unexpected ID: %+v", fields["ID"])
	}
	if fields["日期"] != int64(1781107200000) {
		t.Fatalf("unexpected date millis: %+v", fields["日期"])
	}
	if fields["总Tokens"] != int64(20099584) {
		t.Fatalf("unexpected tokens: %+v", fields["总Tokens"])
	}
	if fields["实际成本USD"] != 30.6706521 {
		t.Fatalf("unexpected cost: %+v", fields["实际成本USD"])
	}
	person, ok := fields["人员"].([]map[string]string)
	if !ok || len(person) != 1 || person[0]["id"] != "ou_alice" {
		t.Fatalf("unexpected person field: %+v", fields["人员"])
	}
}

func TestHasFeishuUsageConfigUsesTableIDAsOptIn(t *testing.T) {
	if HasFeishuUsageConfig(&config.FeishuConfig{}) {
		t.Fatal("expected missing usage table to disable feishu usage sync")
	}
	if !HasFeishuUsageConfig(&config.FeishuConfig{
		Bitable: config.FeishuBitable{
			Usage: config.FeishuUsageTable{TableID: "tbl_usage"},
		},
	}) {
		t.Fatal("expected usage table id to opt in even when app_token is missing")
	}
}

func TestSyncUsageToFeishuUsesStoredRecordID(t *testing.T) {
	ctx := context.Background()
	client := &fakeFeishuUsageClient{}
	store := fakeFeishuUsageRecordStore{
		records: map[string]string{
			"app_token|tbl_usage|2026-06-10#6732": "rec_stored",
		},
	}
	feishu := testFeishuUsageConfig()

	results, err := SyncUsageToFeishu(ctx, feishu, client, store, []UsageSyncResult{
		{UsageDate: "2026-06-10", CodesomeKeyID: 6732, FeishuOpenID: "ou_alice", TotalTokens: 100, ActualCost: 1.25},
	})
	if err != nil {
		t.Fatalf("sync feishu usage: %v", err)
	}
	if len(results) != 1 || results[0].Action != "updated" || results[0].RecordID != "rec_stored" {
		t.Fatalf("unexpected results: %+v", results)
	}
	if client.searchCalls != 0 {
		t.Fatalf("expected stored record id path to skip search, got %d search calls", client.searchCalls)
	}
	if len(client.updated) != 1 || client.updated[0].recordID != "rec_stored" {
		t.Fatalf("unexpected updates: %+v", client.updated)
	}
}

func TestSyncUsageToFeishuCreatesAndStoresRecordID(t *testing.T) {
	ctx := context.Background()
	client := &fakeFeishuUsageClient{}
	store := fakeFeishuUsageRecordStore{records: map[string]string{}}
	feishu := testFeishuUsageConfig()

	results, err := SyncUsageToFeishu(ctx, feishu, client, store, []UsageSyncResult{
		{UsageDate: "2026-06-11", CodesomeKeyID: 6732, FeishuOpenID: "ou_alice", TotalTokens: 200, ActualCost: 2.5},
	})
	if err != nil {
		t.Fatalf("sync feishu usage: %v", err)
	}
	if len(results) != 1 || results[0].Action != "created" || results[0].RecordID != "rec_created" {
		t.Fatalf("unexpected results: %+v", results)
	}
	if client.searchViewID != "vew_usage" {
		t.Fatalf("unexpected view id: %s", client.searchViewID)
	}
	if len(client.created) != 1 || client.created[0]["ID"] != "2026-06-11#6732" {
		t.Fatalf("unexpected creates: %+v", client.created)
	}
	if got := store.records["app_token|tbl_usage|2026-06-11#6732"]; got != "rec_created" {
		t.Fatalf("expected stored record id, got %q", got)
	}
}

func TestSyncUsageToFeishuRepairsStaleRecordID(t *testing.T) {
	ctx := context.Background()
	client := &fakeFeishuUsageClient{
		records: []provider.FeishuBitableRecord{
			{
				RecordID: "rec_repaired",
				Fields: map[string]json.RawMessage{
					"ID": json.RawMessage(`"2026-06-10#6732"`),
				},
			},
		},
		updateErrors: map[string]error{
			"rec_stale": errors.New("feishu api failed: code=1254043 msg=RecordIdNotFound"),
		},
	}
	store := fakeFeishuUsageRecordStore{
		records: map[string]string{
			"app_token|tbl_usage|2026-06-10#6732": "rec_stale",
		},
	}
	feishu := testFeishuUsageConfig()

	results, err := SyncUsageToFeishu(ctx, feishu, client, store, []UsageSyncResult{
		{UsageDate: "2026-06-10", CodesomeKeyID: 6732, FeishuOpenID: "ou_alice", TotalTokens: 100, ActualCost: 1.25},
	})
	if err != nil {
		t.Fatalf("sync feishu usage: %v", err)
	}
	if len(results) != 1 || results[0].Action != "updated" || results[0].Message != "record_id_repaired" {
		t.Fatalf("unexpected results: %+v", results)
	}
	if len(client.updated) != 2 || client.updated[0].recordID != "rec_stale" || client.updated[1].recordID != "rec_repaired" {
		t.Fatalf("unexpected updates: %+v", client.updated)
	}
	if got := store.records["app_token|tbl_usage|2026-06-10#6732"]; got != "rec_repaired" {
		t.Fatalf("expected repaired record id, got %q", got)
	}
}

func TestSyncUsageToFeishuFindsExistingRichTextObjectID(t *testing.T) {
	ctx := context.Background()
	client := &fakeFeishuUsageClient{
		records: []provider.FeishuBitableRecord{
			{
				RecordID: "rec_existing",
				Fields: map[string]json.RawMessage{
					"ID": json.RawMessage(`{"type":1,"value":[{"text":"2026-06-10#6732"}]}`),
				},
			},
		},
	}
	store := fakeFeishuUsageRecordStore{records: map[string]string{}}
	feishu := testFeishuUsageConfig()

	results, err := SyncUsageToFeishu(ctx, feishu, client, store, []UsageSyncResult{
		{UsageDate: "2026-06-10", CodesomeKeyID: 6732, FeishuOpenID: "ou_alice", TotalTokens: 100, ActualCost: 1.25},
	})
	if err != nil {
		t.Fatalf("sync feishu usage: %v", err)
	}
	if len(results) != 1 || results[0].Action != "updated" || results[0].RecordID != "rec_existing" {
		t.Fatalf("unexpected results: %+v", results)
	}
	if len(client.created) != 0 {
		t.Fatalf("expected existing rich text object ID not to create, got %+v", client.created)
	}
	if len(client.updated) != 1 || client.updated[0].recordID != "rec_existing" {
		t.Fatalf("unexpected updates: %+v", client.updated)
	}
	if got := store.records["app_token|tbl_usage|2026-06-10#6732"]; got != "rec_existing" {
		t.Fatalf("expected stored record id, got %q", got)
	}
}

func TestSyncUsageToFeishuSkipsRowsWithoutPersonOrTokens(t *testing.T) {
	ctx := context.Background()
	client := &fakeFeishuUsageClient{}
	store := fakeFeishuUsageRecordStore{records: map[string]string{}}
	feishu := testFeishuUsageConfig()

	results, err := SyncUsageToFeishu(ctx, feishu, client, store, []UsageSyncResult{
		{UsageDate: "2026-06-10", CodesomeKeyID: 6732, TotalTokens: 100, ActualCost: 1.25},
		{UsageDate: "2026-06-11", CodesomeKeyID: 6732, FeishuOpenID: "ou_alice", TotalTokens: 0, ActualCost: 0},
	})
	if err != nil {
		t.Fatalf("sync feishu usage: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("unexpected results: %+v", results)
	}
	if results[0].Action != "skipped" || results[0].Message != "missing_feishu_open_id" {
		t.Fatalf("unexpected missing open id result: %+v", results[0])
	}
	if results[1].Action != "skipped" || results[1].Message != "zero_tokens" {
		t.Fatalf("unexpected zero token result: %+v", results[1])
	}
	if client.searchCalls != 0 || len(client.created) != 0 || len(client.updated) != 0 {
		t.Fatalf("expected skipped rows not to touch feishu, search=%d created=%+v updated=%+v", client.searchCalls, client.created, client.updated)
	}
	if len(store.records) != 0 {
		t.Fatalf("expected skipped rows not to update store, got %+v", store.records)
	}
}

func testFeishuUsageConfig() *config.FeishuConfig {
	return &config.FeishuConfig{
		Bitable: config.FeishuBitable{
			AppToken: "app_token",
			Usage: config.FeishuUsageTable{
				TableID: "tbl_usage",
				ViewID:  "vew_usage",
			},
		},
	}
}

type fakeFeishuUsageClient struct {
	records      []provider.FeishuBitableRecord
	searchViewID string
	searchCalls  int
	created      []map[string]any
	updated      []fakeFeishuUsageUpdate
	updateErrors map[string]error
}

type fakeFeishuUsageUpdate struct {
	recordID string
	fields   map[string]any
}

func (c *fakeFeishuUsageClient) SearchBitableRecords(ctx context.Context, appToken string, tableID string, viewID string, fieldNames []string) ([]provider.FeishuBitableRecord, error) {
	c.searchCalls++
	c.searchViewID = viewID
	return c.records, nil
}

func (c *fakeFeishuUsageClient) CreateBitableRecord(ctx context.Context, appToken string, tableID string, fields map[string]any) (*provider.FeishuBitableRecord, error) {
	c.created = append(c.created, fields)
	return &provider.FeishuBitableRecord{RecordID: "rec_created"}, nil
}

func (c *fakeFeishuUsageClient) UpdateBitableRecord(ctx context.Context, appToken string, tableID string, recordID string, fields map[string]any) (*provider.FeishuBitableRecord, error) {
	c.updated = append(c.updated, fakeFeishuUsageUpdate{recordID: recordID, fields: fields})
	if err := c.updateErrors[recordID]; err != nil {
		return nil, err
	}
	return &provider.FeishuBitableRecord{RecordID: recordID}, nil
}

type fakeFeishuUsageRecordStore struct {
	records map[string]string
}

func (s fakeFeishuUsageRecordStore) GetRecordID(ctx context.Context, appToken string, tableID string, syncID string) (string, error) {
	return s.records[feishuUsageRecordStoreKey(appToken, tableID, syncID)], nil
}

func (s fakeFeishuUsageRecordStore) Upsert(ctx context.Context, appToken string, tableID string, syncID string, recordID string) error {
	s.records[feishuUsageRecordStoreKey(appToken, tableID, syncID)] = recordID
	return nil
}

func feishuUsageRecordStoreKey(appToken string, tableID string, syncID string) string {
	return appToken + "|" + tableID + "|" + syncID
}
