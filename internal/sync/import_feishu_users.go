package syncer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"codesome-usage-manager/internal/config"
	"codesome-usage-manager/internal/provider"
	"codesome-usage-manager/internal/repository"
)

type FeishuBitableUserClient interface {
	SearchBitableRecords(ctx context.Context, appToken string, tableID string, viewID string, fieldNames []string) ([]provider.FeishuBitableRecord, error)
}

func DefaultFeishuUserFields() config.FeishuUserFields {
	return config.FeishuUserFields{
		EmployeeNo: "工号",
		Name:       "人员",
		Team:       "团队",
		Status:     "状态",
		OpenID:     "人员",
	}
}

type UserFeishuImporter struct {
	database *sql.DB
	client   FeishuBitableUserClient
}

func NewUserFeishuImporter(database *sql.DB, client FeishuBitableUserClient) *UserFeishuImporter {
	return &UserFeishuImporter{database: database, client: client}
}

func (i *UserFeishuImporter) Import(ctx context.Context, feishu *config.FeishuConfig, options ImportUsersOptions) ([]ImportUsersResult, error) {
	if i.database == nil {
		return nil, fmt.Errorf("导入 user 需要数据库连接")
	}
	if i.client == nil {
		return nil, fmt.Errorf("feishu bitable client is nil")
	}
	source, fields, err := resolveFeishuUserSource(feishu)
	if err != nil {
		return nil, err
	}

	records, err := i.client.SearchBitableRecords(ctx, source.appToken, source.tableID, source.viewID, feishuUserFieldNames(fields))
	if err != nil {
		return nil, err
	}
	rows, err := parseFeishuUserRecords(records, fields)
	if err != nil {
		return nil, err
	}
	return i.importRowsWithTeams(ctx, rows, options)
}

type feishuUserSource struct {
	appToken string
	tableID  string
	viewID   string
}

func resolveFeishuUserSource(feishu *config.FeishuConfig) (feishuUserSource, config.FeishuUserFields, error) {
	if feishu == nil {
		return feishuUserSource{}, config.FeishuUserFields{}, fmt.Errorf("未找到 feishu 配置")
	}
	source := feishuUserSource{
		appToken: feishu.Bitable.AppToken,
		tableID:  feishu.Bitable.Users.TableID,
		viewID:   feishu.Bitable.Users.ViewID,
	}
	if source.appToken == "" {
		return feishuUserSource{}, config.FeishuUserFields{}, fmt.Errorf("feishu.bitable.app_token is required")
	}
	if source.tableID == "" {
		return feishuUserSource{}, config.FeishuUserFields{}, fmt.Errorf("feishu.bitable.users.table_id is required")
	}
	return source, DefaultFeishuUserFields(), nil
}

func (i *UserFeishuImporter) importRowsWithTeams(ctx context.Context, rows []userCSVRow, options ImportUsersOptions) ([]ImportUsersResult, error) {
	tx, err := i.database.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("start feishu user import transaction: %w", err)
	}
	defer tx.Rollback()

	if err := ensureFeishuTeams(ctx, repository.NewTeamRepositoryTx(tx), rows); err != nil {
		return nil, err
	}

	txImporter := &UserCSVImporter{users: repository.NewUserRepositoryTx(tx)}
	results := make([]ImportUsersResult, 0, len(rows))
	for _, row := range rows {
		result, err := txImporter.importOne(ctx, row, ImportUsersOptions{DryRun: true})
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	if options.DryRun {
		return results, nil
	}

	for _, row := range rows {
		if _, err := txImporter.importOne(ctx, row, ImportUsersOptions{}); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit feishu user import transaction: %w", err)
	}
	return results, nil
}

func ensureFeishuTeams(ctx context.Context, teams *repository.TeamRepository, rows []userCSVRow) error {
	seen := map[string]bool{}
	for _, row := range rows {
		if row.teamCode == "" || seen[row.teamCode] {
			continue
		}
		seen[row.teamCode] = true
		if _, err := teams.GetByCode(ctx, row.teamCode); err == nil {
			continue
		} else if !isRepositoryNotFound(err) {
			return err
		}
		if _, err := teams.Create(ctx, row.teamCode, row.teamCode); err != nil {
			return err
		}
	}
	return nil
}

func feishuUserFieldNames(fields config.FeishuUserFields) []string {
	seen := map[string]bool{}
	values := []string{
		fields.EmployeeNo,
		fields.Name,
		fields.Team,
		fields.Status,
		fields.OpenID,
	}
	var names []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		names = append(names, value)
	}
	return names
}

func parseFeishuUserRecords(records []provider.FeishuBitableRecord, fields config.FeishuUserFields) ([]userCSVRow, error) {
	if len(records) == 0 {
		return nil, fmt.Errorf("飞书多维表格未包含 user 数据")
	}
	seen := map[string]bool{}
	rows := make([]userCSVRow, 0, len(records))
	for index, record := range records {
		row, err := parseFeishuUserRecord(index+1, record, fields)
		if err != nil {
			return nil, err
		}
		if seen[row.employeeNo] {
			return nil, fmt.Errorf("飞书记录 %d employee_no 重复: %s", index+1, row.employeeNo)
		}
		seen[row.employeeNo] = true
		rows = append(rows, row)
	}
	return rows, nil
}

func parseFeishuUserRecord(rowNumber int, record provider.FeishuBitableRecord, fields config.FeishuUserFields) (userCSVRow, error) {
	row := userCSVRow{
		row:          rowNumber,
		employeeNo:   feishuFieldString(record.Fields, fields.EmployeeNo),
		name:         feishuFieldString(record.Fields, fields.Name),
		teamCode:     feishuFieldString(record.Fields, fields.Team),
		status:       normalizeFeishuUserStatus(feishuFieldString(record.Fields, fields.Status)),
		feishuOpenID: feishuOpenID(record.Fields[fields.OpenID]),
		teamColumn:   fields.Team != "",
		openIDColumn: feishuFieldExists(record.Fields, fields.OpenID),
	}
	if row.employeeNo == "" {
		return userCSVRow{}, fmt.Errorf("飞书记录 %d employee_no 不能为空", rowNumber)
	}
	if row.name == "" {
		return userCSVRow{}, fmt.Errorf("飞书记录 %d name 不能为空", rowNumber)
	}
	if row.status != "" {
		row.statusSet = true
	}
	if row.statusSet && (!repository.IsValidUserStatus(row.status) || row.status == repository.UserStatusDeleted) {
		return userCSVRow{}, fmt.Errorf("飞书记录 %d status 必须是 生效/禁用 或 active/inactive", rowNumber)
	}
	return row, nil
}

func feishuFieldExists(recordFields map[string]json.RawMessage, name string) bool {
	if name == "" {
		return false
	}
	_, ok := recordFields[name]
	return ok
}

func feishuFieldString(recordFields map[string]json.RawMessage, name string) string {
	if !feishuFieldExists(recordFields, name) {
		return ""
	}
	return rawFeishuString(recordFields[name])
}

func rawFeishuString(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err == nil {
		return strings.TrimSpace(value)
	}
	var number json.Number
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&number); err == nil {
		return number.String()
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err == nil {
		parts := make([]string, 0, len(items))
		for _, item := range items {
			text := rawFeishuString(item)
			if text != "" {
				parts = append(parts, text)
			}
		}
		return strings.TrimSpace(strings.Join(parts, ""))
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err == nil {
		for _, key := range []string{"text", "name", "en_name", "id", "open_id", "value"} {
			if text := rawFeishuString(object[key]); text != "" {
				return text
			}
		}
	}
	return ""
}

func normalizeFeishuUserStatus(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "":
		return ""
	case "生效", "active":
		return repository.UserStatusActive
	case "禁用", "inactive":
		return repository.UserStatusInactive
	default:
		return value
	}
}

func feishuOpenID(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err == nil {
		return strings.TrimSpace(value)
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err == nil {
		for _, item := range items {
			if id := feishuOpenID(item); id != "" {
				return id
			}
		}
		return ""
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err == nil {
		for _, key := range []string{"open_id", "id", "user_id"} {
			if id := rawFeishuString(object[key]); id != "" {
				return id
			}
		}
	}
	return ""
}
