package syncer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"codesome-usage-manager/internal/config"
	"codesome-usage-manager/internal/provider"
)

const (
	feishuUsageFieldID         = "ID"
	feishuUsageFieldDate       = "日期"
	feishuUsageFieldPerson     = "人员"
	feishuUsageFieldTokens     = "总Tokens"
	feishuUsageFieldActualCost = "实际成本USD"
)

type FeishuBitableUsageClient interface {
	SearchBitableRecords(ctx context.Context, appToken string, tableID string, viewID string, fieldNames []string) ([]provider.FeishuBitableRecord, error)
	CreateBitableRecord(ctx context.Context, appToken string, tableID string, fields map[string]any) (*provider.FeishuBitableRecord, error)
	UpdateBitableRecord(ctx context.Context, appToken string, tableID string, recordID string, fields map[string]any) (*provider.FeishuBitableRecord, error)
}

type FeishuUsageRecordStore interface {
	GetRecordID(ctx context.Context, appToken string, tableID string, syncID string) (string, error)
	Upsert(ctx context.Context, appToken string, tableID string, syncID string, recordID string) error
}

type FeishuUsageSyncResult struct {
	ID       string
	Action   string
	RecordID string
	Message  string
}

func SyncUsageToFeishu(ctx context.Context, feishu *config.FeishuConfig, client FeishuBitableUsageClient, store FeishuUsageRecordStore, rows []UsageSyncResult) ([]FeishuUsageSyncResult, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	if client == nil {
		return nil, fmt.Errorf("feishu bitable client is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("feishu usage record store is nil")
	}
	source, err := resolveFeishuUsageSource(feishu)
	if err != nil {
		return nil, err
	}

	results := make([]FeishuUsageSyncResult, 0, len(rows))
	var existing map[string]string
	loadExisting := func() (map[string]string, error) {
		if existing != nil {
			return existing, nil
		}
		var err error
		existing, err = existingFeishuUsageRecords(ctx, client, source)
		return existing, err
	}

	for _, row := range rows {
		id := FeishuUsageID(row)
		if message := skipFeishuUsageMessage(row); message != "" {
			results = append(results, FeishuUsageSyncResult{ID: id, Action: "skipped", Message: message})
			continue
		}
		fields, err := FeishuUsageFields(row)
		if err != nil {
			return results, err
		}
		if recordID, err := store.GetRecordID(ctx, source.appToken, source.tableID, id); err != nil {
			return results, err
		} else if recordID != "" {
			record, err := client.UpdateBitableRecord(ctx, source.appToken, source.tableID, recordID, fields)
			if err == nil {
				if err := store.Upsert(ctx, source.appToken, source.tableID, id, record.RecordID); err != nil {
					return results, err
				}
				results = append(results, FeishuUsageSyncResult{ID: id, Action: "updated", RecordID: record.RecordID})
				continue
			}
			if !isFeishuRecordIDNotFound(err) {
				return results, fmt.Errorf("update feishu usage %s: %w", id, err)
			}
		}

		existingRecords, err := loadExisting()
		if err != nil {
			return results, err
		}
		if recordID := existingRecords[id]; recordID != "" {
			record, err := client.UpdateBitableRecord(ctx, source.appToken, source.tableID, recordID, fields)
			if err != nil {
				return results, fmt.Errorf("update feishu usage %s: %w", id, err)
			}
			if err := store.Upsert(ctx, source.appToken, source.tableID, id, record.RecordID); err != nil {
				return results, err
			}
			results = append(results, FeishuUsageSyncResult{ID: id, Action: "updated", RecordID: record.RecordID, Message: "record_id_repaired"})
			continue
		}
		record, err := client.CreateBitableRecord(ctx, source.appToken, source.tableID, fields)
		if err != nil {
			return results, fmt.Errorf("create feishu usage %s: %w", id, err)
		}
		if err := store.Upsert(ctx, source.appToken, source.tableID, id, record.RecordID); err != nil {
			return results, err
		}
		results = append(results, FeishuUsageSyncResult{ID: id, Action: "created", RecordID: record.RecordID})
		existingRecords[id] = record.RecordID
	}
	return results, nil
}

func HasFeishuUsageConfig(feishu *config.FeishuConfig) bool {
	if feishu == nil {
		return false
	}
	return feishu.Bitable.Usage.TableID != ""
}

func FeishuUsageID(row UsageSyncResult) string {
	return fmt.Sprintf("%s#%d", row.UsageDate, row.CodesomeKeyID)
}

func skipFeishuUsageMessage(row UsageSyncResult) string {
	if row.FeishuOpenID == "" {
		return "missing_feishu_open_id"
	}
	if row.TotalTokens <= 0 {
		return "zero_tokens"
	}
	return ""
}

func FeishuUsageFields(row UsageSyncResult) (map[string]any, error) {
	dateMillis, err := feishuUsageDateMillis(row.UsageDate)
	if err != nil {
		return nil, err
	}
	fields := map[string]any{
		feishuUsageFieldID:         FeishuUsageID(row),
		feishuUsageFieldDate:       dateMillis,
		feishuUsageFieldTokens:     row.TotalTokens,
		feishuUsageFieldActualCost: row.ActualCost,
	}
	if row.FeishuOpenID != "" {
		fields[feishuUsageFieldPerson] = []map[string]string{{"id": row.FeishuOpenID}}
	}
	return fields, nil
}

type feishuUsageSource struct {
	appToken string
	tableID  string
	viewID   string
}

func resolveFeishuUsageSource(feishu *config.FeishuConfig) (feishuUsageSource, error) {
	if feishu == nil {
		return feishuUsageSource{}, fmt.Errorf("未找到 feishu 配置")
	}
	source := feishuUsageSource{
		appToken: feishu.Bitable.AppToken,
		tableID:  feishu.Bitable.Usage.TableID,
		viewID:   feishu.Bitable.Usage.ViewID,
	}
	if source.appToken == "" {
		return feishuUsageSource{}, fmt.Errorf("feishu.bitable.app_token is required")
	}
	if source.tableID == "" {
		return feishuUsageSource{}, fmt.Errorf("feishu.bitable.usage.table_id is required")
	}
	return source, nil
}

func existingFeishuUsageRecords(ctx context.Context, client FeishuBitableUsageClient, source feishuUsageSource) (map[string]string, error) {
	records, err := client.SearchBitableRecords(ctx, source.appToken, source.tableID, source.viewID, []string{feishuUsageFieldID})
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(records))
	for _, record := range records {
		id := feishuUsageRecordID(record)
		if id == "" {
			continue
		}
		result[id] = record.RecordID
	}
	return result, nil
}

func feishuUsageRecordID(record provider.FeishuBitableRecord) string {
	raw, ok := record.Fields[feishuUsageFieldID]
	if !ok {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var richText []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &richText); err == nil && len(richText) > 0 {
		return richText[0].Text
	}
	var richTextObject struct {
		Value []struct {
			Text string `json:"text"`
		} `json:"value"`
	}
	if err := json.Unmarshal(raw, &richTextObject); err == nil && len(richTextObject.Value) > 0 {
		return richTextObject.Value[0].Text
	}
	return ""
}

func feishuUsageDateMillis(usageDate string) (int64, error) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	date, err := time.ParseInLocation("2006-01-02", usageDate, loc)
	if err != nil {
		return 0, fmt.Errorf("usage date must be YYYY-MM-DD: %s", usageDate)
	}
	return date.UnixMilli(), nil
}

func isFeishuRecordIDNotFound(err error) bool {
	if err == nil {
		return false
	}
	text := err.Error()
	return strings.Contains(text, "1254043") || strings.Contains(text, "RecordIdNotFound")
}
