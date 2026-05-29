package syncer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"codesome-usage-manager/internal/config"
	"codesome-usage-manager/internal/repository"
)

type ImportConfigKeysOptions struct {
	DryRun  bool
	GroupID int
}

type ImportConfigKeysResult struct {
	EmployeeNo    string
	UserName      string
	CodesomeKeyID int
	KeyName       string
	GroupID       int
	Action        string
}

type ConfigKeyImporter struct {
	users *repository.UserRepository
	keys  *repository.APIKeyRepository
}

func NewConfigKeyImporter(database *sql.DB) *ConfigKeyImporter {
	if database == nil {
		return &ConfigKeyImporter{}
	}
	return &ConfigKeyImporter{
		users: repository.NewUserRepository(database),
		keys:  repository.NewAPIKeyRepository(database),
	}
}

func (i *ConfigKeyImporter) Import(ctx context.Context, cfg *config.Config, options ImportConfigKeysOptions) ([]ImportConfigKeysResult, error) {
	if cfg == nil {
		return nil, fmt.Errorf("未找到 Codesome 配置")
	}
	codesome := cfg.GetCodesomeConfig()
	if codesome == nil {
		return nil, fmt.Errorf("未找到 Codesome 配置")
	}
	if !options.DryRun && (i.users == nil || i.keys == nil) {
		return nil, fmt.Errorf("导入 config key 需要数据库连接")
	}
	if len(codesome.ApiKeyIDs) == 0 {
		return nil, fmt.Errorf("未配置 codesome.api_key_ids")
	}

	groupID := options.GroupID
	if groupID == 0 {
		groupID = codesome.DefaultGroupID
	}
	if groupID <= 0 {
		return nil, fmt.Errorf("导入 config key 需要 --group-id 或 codesome.default_group_id")
	}

	results := make([]ImportConfigKeysResult, 0, len(codesome.ApiKeyIDs))
	for _, key := range codesome.ApiKeyIDs {
		if key.ID <= 0 {
			return nil, fmt.Errorf("api_key_ids 包含非法 id: %d", key.ID)
		}
		name := legacyKeyName(key)
		employeeNo := legacyEmployeeNo(key)
		result := ImportConfigKeysResult{
			EmployeeNo:    employeeNo,
			UserName:      name,
			CodesomeKeyID: key.ID,
			KeyName:       name,
			GroupID:       groupID,
		}

		if i.keys != nil {
			if _, err := i.keys.GetByCodesomeKeyID(ctx, key.ID); err == nil {
				result.Action = "skip"
				results = append(results, result)
				continue
			} else if !isRepositoryNotFound(err) {
				return nil, err
			}
		}

		result.Action = "create"
		if options.DryRun {
			results = append(results, result)
			continue
		}

		user, err := i.users.GetByEmployeeNo(ctx, employeeNo)
		if err != nil {
			if !isRepositoryNotFound(err) {
				return nil, err
			}
			user, err = i.users.Create(ctx, repository.CreateUserParams{
				EmployeeNo: employeeNo,
				Name:       name,
			})
			if err != nil {
				return nil, err
			}
		}
		if _, err := i.keys.Create(ctx, repository.CreateAPIKeyParams{
			UserID:        user.ID,
			CodesomeKeyID: key.ID,
			Name:          name,
			Status:        repository.APIKeyStatusActive,
			GroupID:       groupID,
		}); err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func legacyKeyName(key config.CodesomeApiKeyId) string {
	if key.Name != "" {
		return key.Name
	}
	if key.Key != "" {
		return key.Key
	}
	return fmt.Sprintf("legacy-%d", key.ID)
}

func legacyEmployeeNo(key config.CodesomeApiKeyId) string {
	if key.Key != "" {
		return "legacy:" + key.Key
	}
	return fmt.Sprintf("legacy:%d", key.ID)
}

func isRepositoryNotFound(err error) bool {
	return err != nil && (errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "not found"))
}
