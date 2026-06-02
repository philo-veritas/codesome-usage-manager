package syncer

import (
	"context"
	"encoding/json"
	"testing"

	"codesome-usage-manager/internal/config"
	"codesome-usage-manager/internal/provider"
	"codesome-usage-manager/internal/repository"
)

func TestUserFeishuImporterUsesFixedFieldNames(t *testing.T) {
	database := newTestDatabase(t)
	ctx := context.Background()
	if _, err := repository.NewTeamRepository(database).Create(ctx, "platform", "Platform"); err != nil {
		t.Fatalf("create team: %v", err)
	}

	client := &fakeFeishuBitableUserClient{
		records: []provider.FeishuBitableRecord{
			{
				RecordID: "rec1",
				Fields: map[string]json.RawMessage{
					"人员": json.RawMessage(`[{"email":"","en_name":"Alice","id":"ou_alice","name":"Alice"}]`),
					"团队": json.RawMessage(`[{"text":"platform","type":"text"}]`),
					"工号": json.RawMessage(`{"type":1,"value":[{"text":"E12345","type":"text"}]}`),
					"状态": json.RawMessage(`"生效"`),
				},
			},
		},
	}
	feishu := &config.FeishuConfig{
		Bitable: config.FeishuBitable{
			AppToken: "app_token",
			Users: config.FeishuUsersTable{
				TableID: "tbl1",
			},
		},
	}

	results, err := NewUserFeishuImporter(database, client).Import(ctx, feishu, ImportUsersOptions{})
	if err != nil {
		t.Fatalf("import feishu users: %v", err)
	}
	if len(results) != 1 || results[0].Action != "create" || results[0].FeishuOpenID != "ou_alice" {
		t.Fatalf("unexpected import results: %+v", results)
	}
	if len(client.fieldNames) != 4 || !containsString(client.fieldNames, "人员") {
		t.Fatalf("unexpected requested fields: %+v", client.fieldNames)
	}

	user, err := repository.NewUserRepository(database).GetByEmployeeNo(ctx, "E12345")
	if err != nil {
		t.Fatalf("get imported user: %v", err)
	}
	if user.FeishuOpenID != "ou_alice" {
		t.Fatalf("expected feishu open id to be stored, got %+v", user)
	}
	if user.CodesomeGroupID != nil {
		t.Fatalf("expected Feishu import to avoid user group override, got %+v", user)
	}
}

func TestUserFeishuImporterCreatesMissingTeams(t *testing.T) {
	database := newTestDatabase(t)
	ctx := context.Background()
	client := &fakeFeishuBitableUserClient{
		records: []provider.FeishuBitableRecord{
			{
				RecordID: "rec1",
				Fields: map[string]json.RawMessage{
					"人员": json.RawMessage(`[{"id":"ou_ying","name":"应晶"}]`),
					"团队": json.RawMessage(`[{"text":"数字化中心","type":"text"}]`),
					"工号": json.RawMessage(`{"type":1,"value":[{"text":"89012024","type":"text"}]}`),
					"状态": json.RawMessage(`"生效"`),
				},
			},
		},
	}
	feishu := &config.FeishuConfig{
		Bitable: config.FeishuBitable{
			AppToken: "app_token",
			Users:    config.FeishuUsersTable{TableID: "tbl1"},
		},
	}

	results, err := NewUserFeishuImporter(database, client).Import(ctx, feishu, ImportUsersOptions{DryRun: true})
	if err != nil {
		t.Fatalf("dry-run import feishu users: %v", err)
	}
	if len(results) != 1 || results[0].TeamCode != "数字化中心" {
		t.Fatalf("unexpected dry-run results: %+v", results)
	}
	if _, err := repository.NewTeamRepository(database).GetByCode(ctx, "数字化中心"); err == nil {
		t.Fatal("expected dry-run to avoid persisting created team")
	}

	results, err = NewUserFeishuImporter(database, client).Import(ctx, feishu, ImportUsersOptions{})
	if err != nil {
		t.Fatalf("import feishu users: %v", err)
	}
	if len(results) != 1 || results[0].Action != "create" {
		t.Fatalf("unexpected import results: %+v", results)
	}
	team, err := repository.NewTeamRepository(database).GetByCode(ctx, "数字化中心")
	if err != nil {
		t.Fatalf("get created team: %v", err)
	}
	if team.Name != "数字化中心" || team.Status != repository.TeamStatusActive {
		t.Fatalf("unexpected created team: %+v", team)
	}
	user, err := repository.NewUserRepository(database).GetByEmployeeNo(ctx, "89012024")
	if err != nil {
		t.Fatalf("get imported user: %v", err)
	}
	if user.TeamCode == nil || *user.TeamCode != "数字化中心" {
		t.Fatalf("expected user to belong to created team, got %+v", user)
	}
}

func TestUserFeishuImporterClearsMissingTeamCell(t *testing.T) {
	database := newTestDatabase(t)
	ctx := context.Background()
	if _, err := repository.NewTeamRepository(database).Create(ctx, "platform", "Platform"); err != nil {
		t.Fatalf("create team: %v", err)
	}
	userRepo := repository.NewUserRepository(database)
	if _, err := userRepo.Create(ctx, repository.CreateUserParams{
		EmployeeNo: "E12345",
		Name:       "Alice",
		TeamCode:   "platform",
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	client := &fakeFeishuBitableUserClient{
		records: []provider.FeishuBitableRecord{
			{
				RecordID: "rec1",
				Fields: map[string]json.RawMessage{
					"人员": json.RawMessage(`[{"id":"ou_alice","name":"Alice"}]`),
					"工号": json.RawMessage(`{"type":1,"value":[{"text":"E12345","type":"text"}]}`),
					"状态": json.RawMessage(`"生效"`),
				},
			},
		},
	}

	results, err := NewUserFeishuImporter(database, client).Import(ctx, &config.FeishuConfig{
		Bitable: config.FeishuBitable{
			AppToken: "app_token",
			Users:    config.FeishuUsersTable{TableID: "tbl1"},
		},
	}, ImportUsersOptions{})
	if err != nil {
		t.Fatalf("import feishu users: %v", err)
	}
	if len(results) != 1 || results[0].Action != "update" {
		t.Fatalf("unexpected import results: %+v", results)
	}
	user, err := userRepo.GetByEmployeeNo(ctx, "E12345")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if user.TeamCode != nil {
		t.Fatalf("expected missing Feishu team cell to clear local team, got %+v", user.TeamCode)
	}
}

func TestUserFeishuImporterDoesNotRequireFieldMapping(t *testing.T) {
	database := newTestDatabase(t)
	client := &fakeFeishuBitableUserClient{
		records: []provider.FeishuBitableRecord{
			{
				RecordID: "rec1",
				Fields: map[string]json.RawMessage{
					"人员": json.RawMessage(`[{"id":"ou_bob","name":"Bob"}]`),
					"工号": json.RawMessage(`{"type":1,"value":[{"text":"E12346","type":"text"}]}`),
					"状态": json.RawMessage(`"禁用"`),
				},
			},
		},
	}
	results, err := NewUserFeishuImporter(database, client).Import(context.Background(), &config.FeishuConfig{
		Bitable: config.FeishuBitable{
			AppToken: "app_token",
			Users:    config.FeishuUsersTable{TableID: "tbl1"},
		},
	}, ImportUsersOptions{DryRun: true})
	if err != nil {
		t.Fatalf("import without field mapping: %v", err)
	}
	if len(results) != 1 || results[0].Status != repository.UserStatusInactive {
		t.Fatalf("unexpected import results: %+v", results)
	}
}

func TestParseFeishuUserRecordMapsDisabledStatus(t *testing.T) {
	row, err := parseFeishuUserRecord(1, provider.FeishuBitableRecord{
		RecordID: "rec1",
		Fields: map[string]json.RawMessage{
			"人员": json.RawMessage(`[{"id":"ou_bob","name":"Bob"}]`),
			"工号": json.RawMessage(`{"type":1,"value":[{"text":"E12346","type":"text"}]}`),
			"状态": json.RawMessage(`"禁用"`),
		},
	}, config.FeishuUserFields{
		EmployeeNo: "工号",
		Name:       "人员",
		Status:     "状态",
		OpenID:     "人员",
	})
	if err != nil {
		t.Fatalf("parse feishu record: %v", err)
	}
	if row.status != repository.UserStatusInactive || row.feishuOpenID != "ou_bob" {
		t.Fatalf("unexpected row: %+v", row)
	}
}

type fakeFeishuBitableUserClient struct {
	fieldNames []string
	records    []provider.FeishuBitableRecord
}

func (c *fakeFeishuBitableUserClient) SearchBitableRecords(ctx context.Context, appToken string, tableID string, viewID string, fieldNames []string) ([]provider.FeishuBitableRecord, error) {
	c.fieldNames = fieldNames
	return c.records, nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
