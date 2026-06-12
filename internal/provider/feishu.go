package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"codesome-usage-manager/internal/config"
)

var feishuHTTPClient = &http.Client{Timeout: 20 * time.Second}

type FeishuClient struct {
	cfg *config.FeishuConfig
}

type FeishuBitableField struct {
	FieldID   string          `json:"field_id"`
	FieldName string          `json:"field_name"`
	Type      int             `json:"type"`
	UIType    string          `json:"ui_type"`
	IsPrimary bool            `json:"is_primary"`
	Property  json.RawMessage `json:"property"`
}

type FeishuBitableRecord struct {
	RecordID string                     `json:"record_id"`
	Fields   map[string]json.RawMessage `json:"fields"`
}

type feishuBitableRecordResponse struct {
	Record FeishuBitableRecord `json:"record"`
}

type FeishuMessageResult struct {
	MessageID string `json:"message_id"`
}

type feishuBaseResponse struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

func NewFeishuClient(cfg *config.Config) (*FeishuClient, error) {
	feishu := cfg.GetFeishuConfig()
	if feishu == nil {
		return nil, fmt.Errorf("未找到 feishu 配置")
	}
	if feishu.AppID == "" {
		return nil, fmt.Errorf("feishu.app_id is required")
	}
	if feishu.AppSecret == "" {
		return nil, fmt.Errorf("feishu.app_secret is required")
	}
	return &FeishuClient{cfg: feishu}, nil
}

func (c *FeishuClient) ListBitableFields(ctx context.Context, appToken string, tableID string, viewID string) ([]FeishuBitableField, error) {
	if appToken == "" || tableID == "" {
		return nil, fmt.Errorf("feishu bitable app_token and table_id are required")
	}
	token, err := c.tenantAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	var fields []FeishuBitableField
	pageToken := ""
	for {
		query := url.Values{}
		query.Set("page_size", "100")
		if pageToken != "" {
			query.Set("page_token", pageToken)
		}
		if viewID != "" {
			query.Set("view_id", viewID)
		}
		path := fmt.Sprintf("/bitable/v1/apps/%s/tables/%s/fields?%s", url.PathEscape(appToken), url.PathEscape(tableID), query.Encode())
		var data struct {
			HasMore   bool                 `json:"has_more"`
			PageToken string               `json:"page_token"`
			Items     []FeishuBitableField `json:"items"`
		}
		if err := c.request(ctx, http.MethodGet, path, token, nil, &data); err != nil {
			return nil, err
		}
		fields = append(fields, data.Items...)
		if !data.HasMore {
			return fields, nil
		}
		pageToken = data.PageToken
	}
}

func (c *FeishuClient) SearchBitableRecords(ctx context.Context, appToken string, tableID string, viewID string, fieldNames []string) ([]FeishuBitableRecord, error) {
	if appToken == "" || tableID == "" {
		return nil, fmt.Errorf("feishu bitable app_token and table_id are required")
	}
	token, err := c.tenantAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	body := map[string]any{}
	if viewID != "" {
		body["view_id"] = viewID
	}
	if len(fieldNames) > 0 {
		body["field_names"] = fieldNames
	}

	var records []FeishuBitableRecord
	pageToken := ""
	for {
		query := url.Values{}
		query.Set("page_size", "500")
		query.Set("user_id_type", "open_id")
		if pageToken != "" {
			query.Set("page_token", pageToken)
		}
		path := fmt.Sprintf("/bitable/v1/apps/%s/tables/%s/records/search?%s", url.PathEscape(appToken), url.PathEscape(tableID), query.Encode())
		var data struct {
			HasMore   bool                  `json:"has_more"`
			PageToken string                `json:"page_token"`
			Items     []FeishuBitableRecord `json:"items"`
		}
		if err := c.request(ctx, http.MethodPost, path, token, body, &data); err != nil {
			return nil, err
		}
		records = append(records, data.Items...)
		if !data.HasMore {
			return records, nil
		}
		pageToken = data.PageToken
	}
}

func (c *FeishuClient) CreateBitableRecord(ctx context.Context, appToken string, tableID string, fields map[string]any) (*FeishuBitableRecord, error) {
	if appToken == "" || tableID == "" {
		return nil, fmt.Errorf("feishu bitable app_token and table_id are required")
	}
	token, err := c.tenantAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	body := map[string]any{"fields": fields}
	path := fmt.Sprintf("/bitable/v1/apps/%s/tables/%s/records?user_id_type=open_id", url.PathEscape(appToken), url.PathEscape(tableID))
	var data feishuBitableRecordResponse
	if err := c.request(ctx, http.MethodPost, path, token, body, &data); err != nil {
		return nil, err
	}
	return &data.Record, nil
}

func (c *FeishuClient) UpdateBitableRecord(ctx context.Context, appToken string, tableID string, recordID string, fields map[string]any) (*FeishuBitableRecord, error) {
	if appToken == "" || tableID == "" || recordID == "" {
		return nil, fmt.Errorf("feishu bitable app_token, table_id, and record_id are required")
	}
	token, err := c.tenantAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	body := map[string]any{"fields": fields}
	path := fmt.Sprintf("/bitable/v1/apps/%s/tables/%s/records/%s?user_id_type=open_id", url.PathEscape(appToken), url.PathEscape(tableID), url.PathEscape(recordID))
	var data feishuBitableRecordResponse
	if err := c.request(ctx, http.MethodPut, path, token, body, &data); err != nil {
		return nil, err
	}
	return &data.Record, nil
}

func (c *FeishuClient) SendTextMessage(ctx context.Context, openID string, text string) (*FeishuMessageResult, error) {
	if openID == "" {
		return nil, fmt.Errorf("feishu open_id is required")
	}
	if text == "" {
		return nil, fmt.Errorf("message text is required")
	}
	token, err := c.tenantAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	content, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return nil, fmt.Errorf("encode feishu message content: %w", err)
	}
	body := map[string]any{
		"receive_id": openID,
		"msg_type":   "text",
		"content":    string(content),
	}
	var data FeishuMessageResult
	if err := c.request(ctx, http.MethodPost, "/im/v1/messages?receive_id_type=open_id", token, body, &data); err != nil {
		return nil, err
	}
	return &data, nil
}

func (c *FeishuClient) tenantAccessToken(ctx context.Context) (string, error) {
	body := map[string]string{
		"app_id":     c.cfg.AppID,
		"app_secret": c.cfg.AppSecret,
	}
	var resp struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
	}
	if err := c.requestRaw(ctx, http.MethodPost, "/auth/v3/tenant_access_token/internal", "", body, &resp); err != nil {
		return "", err
	}
	if resp.Code != 0 {
		return "", fmt.Errorf("feishu auth failed: code=%d msg=%s", resp.Code, resp.Msg)
	}
	if resp.TenantAccessToken == "" {
		return "", fmt.Errorf("feishu auth returned empty tenant_access_token")
	}
	return resp.TenantAccessToken, nil
}

func (c *FeishuClient) request(ctx context.Context, method string, path string, token string, body any, data any) error {
	var resp feishuBaseResponse
	if err := c.requestRaw(ctx, method, path, token, body, &resp); err != nil {
		return err
	}
	if resp.Code != 0 {
		return fmt.Errorf("feishu api failed: code=%d msg=%s", resp.Code, resp.Msg)
	}
	if data == nil || len(resp.Data) == 0 {
		return nil
	}
	if err := json.Unmarshal(resp.Data, data); err != nil {
		return fmt.Errorf("decode feishu data: %w", err)
	}
	return nil
}

func (c *FeishuClient) requestRaw(ctx context.Context, method string, path string, token string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode feishu request: %w", err)
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.cfg.BaseURL+path, reader)
	if err != nil {
		return fmt.Errorf("build feishu request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := feishuHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("call feishu api: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read feishu response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("feishu api status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("decode feishu response: %w", err)
	}
	return nil
}
