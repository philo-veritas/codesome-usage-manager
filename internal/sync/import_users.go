package syncer

import (
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"

	"codesome-usage-manager/internal/repository"
)

type ImportUsersOptions struct {
	DryRun bool
}

type ImportUsersResult struct {
	Row             int
	Action          string
	EmployeeNo      string
	Name            string
	TeamCode        string
	Status          string
	CodesomeGroupID *int
}

type UserCSVImporter struct {
	database *sql.DB
	users    *repository.UserRepository
}

type userCSVRow struct {
	row             int
	employeeNo      string
	name            string
	teamCode        string
	status          string
	codesomeGroupID *int
	groupColumn     bool
	teamColumn      bool
	statusSet       bool
}

func NewUserCSVImporter(database *sql.DB) *UserCSVImporter {
	if database == nil {
		return &UserCSVImporter{}
	}
	return &UserCSVImporter{
		database: database,
		users:    repository.NewUserRepository(database),
	}
}

func (i *UserCSVImporter) ImportCSV(ctx context.Context, reader io.Reader, options ImportUsersOptions) ([]ImportUsersResult, error) {
	rows, err := parseUserCSV(reader)
	if err != nil {
		return nil, err
	}
	if i.users == nil {
		return nil, fmt.Errorf("导入 user 需要数据库连接")
	}

	results := make([]ImportUsersResult, 0, len(rows))
	for _, row := range rows {
		result, err := i.importOne(ctx, row, ImportUsersOptions{DryRun: true})
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	if options.DryRun {
		return results, nil
	}

	tx, err := i.database.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("start user import transaction: %w", err)
	}
	defer tx.Rollback()

	txImporter := &UserCSVImporter{users: repository.NewUserRepositoryTx(tx)}
	for _, row := range rows {
		if _, err := txImporter.importOne(ctx, row, ImportUsersOptions{}); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit user import transaction: %w", err)
	}
	return results, nil
}

func (i *UserCSVImporter) importOne(ctx context.Context, row userCSVRow, options ImportUsersOptions) (ImportUsersResult, error) {
	result := ImportUsersResult{
		Row:             row.row,
		EmployeeNo:      row.employeeNo,
		Name:            row.name,
		TeamCode:        row.teamCode,
		Status:          row.status,
		CodesomeGroupID: row.codesomeGroupID,
	}

	if i.users == nil {
		result.Action = "create"
		if result.Status == "" {
			result.Status = repository.UserStatusActive
		}
		return result, nil
	}

	user, err := i.users.GetByEmployeeNo(ctx, row.employeeNo)
	if err != nil {
		if !isRepositoryNotFound(err) {
			return ImportUsersResult{}, err
		}
		result.Action = "create"
		if result.Status == "" {
			result.Status = repository.UserStatusActive
		}
		if options.DryRun {
			if i.users != nil {
				if err := i.users.ValidateCreate(ctx, createUserParamsFromCSV(row)); err != nil {
					return ImportUsersResult{}, err
				}
			}
			return result, nil
		}
		_, err := i.users.Create(ctx, createUserParamsFromCSV(row))
		return result, err
	}
	if user.Status == repository.UserStatusDeleted {
		return ImportUsersResult{}, fmt.Errorf("CSV 第 %d 行 employee_no 已是 deleted 用户，不能导入: %s", row.row, row.employeeNo)
	}

	if userMatchesCSV(user, row) {
		result.Action = "skip"
		return result, nil
	}
	result.Action = "update"
	params := updateUserParamsFromCSV(row)
	if options.DryRun {
		if err := i.users.ValidateUpdate(ctx, row.employeeNo, params); err != nil {
			return ImportUsersResult{}, err
		}
		return result, nil
	}
	_, err = i.users.Update(ctx, row.employeeNo, params)
	return result, err
}

func createUserParamsFromCSV(row userCSVRow) repository.CreateUserParams {
	return repository.CreateUserParams{
		EmployeeNo:      row.employeeNo,
		Name:            row.name,
		TeamCode:        row.teamCode,
		Status:          row.status,
		CodesomeGroupID: row.codesomeGroupID,
	}
}

func updateUserParamsFromCSV(row userCSVRow) repository.UpdateUserParams {
	params := repository.UpdateUserParams{
		Name: &row.name,
	}
	if row.teamColumn {
		params.TeamCode = &row.teamCode
	}
	if row.statusSet {
		params.Status = &row.status
	}
	if row.groupColumn {
		if row.codesomeGroupID != nil {
			params.CodesomeGroupID = row.codesomeGroupID
		} else {
			params.ClearGroupID = true
		}
	}
	return params
}

func parseUserCSV(reader io.Reader) ([]userCSVRow, error) {
	csvReader := csv.NewReader(reader)
	csvReader.TrimLeadingSpace = true

	header, err := csvReader.Read()
	if err != nil {
		return nil, fmt.Errorf("读取 CSV header 失败: %w", err)
	}
	columns := userCSVColumns(header)
	if _, ok := columns["employee_no"]; !ok {
		return nil, fmt.Errorf("CSV 缺少 employee_no 列")
	}
	if _, ok := columns["name"]; !ok {
		return nil, fmt.Errorf("CSV 缺少 name 列")
	}

	seen := map[string]bool{}
	var rows []userCSVRow
	for rowNumber := 2; ; rowNumber++ {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("读取 CSV 第 %d 行失败: %w", rowNumber, err)
		}
		if isBlankCSVRecord(record) {
			continue
		}
		row, err := parseUserCSVRecord(rowNumber, record, columns)
		if err != nil {
			return nil, err
		}
		if seen[row.employeeNo] {
			return nil, fmt.Errorf("CSV 第 %d 行 employee_no 重复: %s", rowNumber, row.employeeNo)
		}
		seen[row.employeeNo] = true
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("CSV 未包含 user 数据")
	}
	return rows, nil
}

func userCSVColumns(header []string) map[string]int {
	columns := map[string]int{}
	for i, name := range header {
		columns[strings.ToLower(strings.TrimSpace(strings.TrimPrefix(name, "\ufeff")))] = i
	}
	return columns
}

func parseUserCSVRecord(rowNumber int, record []string, columns map[string]int) (userCSVRow, error) {
	row := userCSVRow{
		row:         rowNumber,
		employeeNo:  csvCell(record, columns, "employee_no"),
		name:        csvCell(record, columns, "name"),
		teamCode:    csvCell(record, columns, "team"),
		status:      strings.ToLower(csvCell(record, columns, "status")),
		groupColumn: csvColumnExists(columns, "group_id"),
		teamColumn:  csvColumnExists(columns, "team"),
	}
	if row.employeeNo == "" {
		return userCSVRow{}, fmt.Errorf("CSV 第 %d 行 employee_no 不能为空", rowNumber)
	}
	if row.name == "" {
		return userCSVRow{}, fmt.Errorf("CSV 第 %d 行 name 不能为空", rowNumber)
	}
	if row.status != "" {
		row.statusSet = true
	}
	if row.statusSet && (!repository.IsValidUserStatus(row.status) || row.status == repository.UserStatusDeleted) {
		return userCSVRow{}, fmt.Errorf("CSV 第 %d 行 status 必须是 active 或 inactive", rowNumber)
	}
	groupID := csvCell(record, columns, "group_id")
	if groupID != "" {
		parsed, err := strconv.Atoi(groupID)
		if err != nil || parsed <= 0 {
			return userCSVRow{}, fmt.Errorf("CSV 第 %d 行 group_id 必须是正整数", rowNumber)
		}
		row.codesomeGroupID = &parsed
	}
	return row, nil
}

func csvColumnExists(columns map[string]int, name string) bool {
	_, ok := columns[name]
	return ok
}

func csvCell(record []string, columns map[string]int, name string) string {
	index, ok := columns[name]
	if !ok || index >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[index])
}

func isBlankCSVRecord(record []string) bool {
	for _, cell := range record {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
}

func userMatchesCSV(user *repository.User, row userCSVRow) bool {
	if user.Name != row.name {
		return false
	}
	if row.statusSet && user.Status != row.status {
		return false
	}
	if row.teamColumn && stringPtrValue(user.TeamCode) != row.teamCode {
		return false
	}
	if row.groupColumn && intPtrValue(user.CodesomeGroupID) != intPtrValue(row.codesomeGroupID) {
		return false
	}
	return true
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func intPtrValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}
