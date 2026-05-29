package syncer

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"codesome-usage-manager/internal/provider"
	"codesome-usage-manager/internal/repository"
)

type ImportRemoteKeysOptions struct {
	DryRun bool
}

type ImportRemoteKeysResult struct {
	EmployeeNo    string
	UserName      string
	CodesomeKeyID int
	KeyName       string
	GroupID       int
	Action        string
}

type RemoteKeyImporter struct {
	users *repository.UserRepository
	keys  *repository.APIKeyRepository
}

func NewRemoteKeyImporter(database *sql.DB) *RemoteKeyImporter {
	if database == nil {
		return &RemoteKeyImporter{}
	}
	return &RemoteKeyImporter{
		users: repository.NewUserRepository(database),
		keys:  repository.NewAPIKeyRepository(database),
	}
}

func (i *RemoteKeyImporter) Import(ctx context.Context, remoteKeys []provider.CodesomeApiKey, options ImportRemoteKeysOptions) ([]ImportRemoteKeysResult, error) {
	if !options.DryRun && (i.users == nil || i.keys == nil) {
		return nil, fmt.Errorf("导入远程 key 需要数据库连接")
	}
	if len(remoteKeys) == 0 {
		return nil, fmt.Errorf("未找到远程 Codesome API Key")
	}

	results := make([]ImportRemoteKeysResult, 0, len(remoteKeys))
	for _, key := range remoteKeys {
		result, err := i.importOne(ctx, key, options)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func (i *RemoteKeyImporter) importOne(ctx context.Context, key provider.CodesomeApiKey, options ImportRemoteKeysOptions) (ImportRemoteKeysResult, error) {
	if key.ID <= 0 {
		return ImportRemoteKeysResult{}, fmt.Errorf("远程 API Key 包含非法 id: %d", key.ID)
	}
	groupID := remoteKeyGroupID(key)
	if groupID <= 0 {
		return ImportRemoteKeysResult{}, fmt.Errorf("远程 API Key %d 未绑定有效 group", key.ID)
	}
	status, err := normalizeRemoteKeyStatus(key.Status)
	if err != nil {
		return ImportRemoteKeysResult{}, fmt.Errorf("远程 API Key %d: %w", key.ID, err)
	}
	name := remoteKeyName(key)
	employeeNo := remoteKeyEmployeeNo(key)
	result := ImportRemoteKeysResult{
		EmployeeNo:    employeeNo,
		UserName:      name,
		CodesomeKeyID: key.ID,
		KeyName:       name,
		GroupID:       groupID,
	}

	if i.keys != nil {
		if _, err := i.keys.GetByCodesomeKeyID(ctx, key.ID); err == nil {
			result.Action = "skip"
			return result, nil
		} else if !isRepositoryNotFound(err) {
			return ImportRemoteKeysResult{}, err
		}
	}

	result.Action = "create"
	if options.DryRun {
		return result, nil
	}

	user, err := i.users.GetByEmployeeNo(ctx, employeeNo)
	if err != nil {
		if !isRepositoryNotFound(err) {
			return ImportRemoteKeysResult{}, err
		}
		codesomeGroupID := groupID
		user, err = i.users.Create(ctx, repository.CreateUserParams{
			EmployeeNo:      employeeNo,
			Name:            name,
			CodesomeGroupID: &codesomeGroupID,
		})
		if err != nil {
			return ImportRemoteKeysResult{}, err
		}
	}
	if _, err := i.keys.Create(ctx, repository.CreateAPIKeyParams{
		UserID:        user.ID,
		CodesomeKeyID: key.ID,
		Name:          name,
		Status:        status,
		GroupID:       groupID,
	}); err != nil {
		return ImportRemoteKeysResult{}, err
	}
	return result, nil
}

func remoteKeyGroupID(key provider.CodesomeApiKey) int {
	if key.GroupID > 0 {
		return key.GroupID
	}
	if key.Group != nil {
		return key.Group.ID
	}
	return 0
}

func remoteKeyName(key provider.CodesomeApiKey) string {
	name := strings.TrimSpace(key.Name)
	if name != "" {
		return name
	}
	return fmt.Sprintf("codesome-key-%d", key.ID)
}

func remoteKeyEmployeeNo(key provider.CodesomeApiKey) string {
	return fmt.Sprintf("codesome-key:%d", key.ID)
}

func normalizeRemoteKeyStatus(status string) (string, error) {
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "" {
		return repository.APIKeyStatusActive, nil
	}
	if repository.IsValidAPIKeyStatus(status) {
		return status, nil
	}
	return "", fmt.Errorf("unsupported status %q", status)
}
