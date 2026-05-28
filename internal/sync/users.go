package syncer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"codesome-usage-manager/internal/provider"
	"codesome-usage-manager/internal/repository"
)

type UserKeyService interface {
	CreateKey(ctx context.Context, name string, groupID int) (*provider.CodesomeApiKeyWithSecret, error)
	UpdateKey(ctx context.Context, keyID int, update provider.CodesomeKeyUpdate) (*provider.CodesomeApiKey, error)
}

type UserSyncer struct {
	users          *repository.UserRepository
	keys           *repository.APIKeyRepository
	service        UserKeyService
	defaultGroupID int
}

type UserSyncOptions struct {
	DryRun     bool
	EmployeeNo string
}

type UserSyncResult struct {
	EmployeeNo    string
	UserName      string
	Action        string
	CodesomeKeyID int
	GroupID       int
	Message       string
	RawKey        string
}

func NewUserSyncer(database *sql.DB, service UserKeyService, defaultGroupID int) *UserSyncer {
	return &UserSyncer{
		users:          repository.NewUserRepository(database),
		keys:           repository.NewAPIKeyRepository(database),
		service:        service,
		defaultGroupID: defaultGroupID,
	}
}

func (s *UserSyncer) SyncUsers(ctx context.Context, options UserSyncOptions) ([]UserSyncResult, error) {
	users, err := s.resolveUsers(ctx, options.EmployeeNo)
	if err != nil {
		return nil, err
	}

	results := make([]UserSyncResult, 0, len(users))
	for _, user := range users {
		result, err := s.syncUser(ctx, user, options.DryRun)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func (s *UserSyncer) resolveUsers(ctx context.Context, employeeNo string) ([]repository.User, error) {
	if employeeNo == "" {
		return s.users.List(ctx)
	}
	user, err := s.users.GetByEmployeeNo(ctx, employeeNo)
	if err != nil {
		return nil, err
	}
	return []repository.User{*user}, nil
}

func (s *UserSyncer) syncUser(ctx context.Context, user repository.User, dryRun bool) (UserSyncResult, error) {
	result := UserSyncResult{
		EmployeeNo: user.EmployeeNo,
		UserName:   user.Name,
	}

	key, err := s.keys.GetLatestByUserID(ctx, user.ID)
	if err != nil && !isNotFound(err) {
		return result, err
	}
	if key == nil {
		return s.syncUserWithoutKey(ctx, user, dryRun, result)
	}
	return s.syncUserWithKey(ctx, user, key, dryRun, result)
}

func (s *UserSyncer) syncUserWithoutKey(ctx context.Context, user repository.User, dryRun bool, result UserSyncResult) (UserSyncResult, error) {
	if user.Status != repository.UserStatusActive {
		result.Action = "noop"
		result.Message = "非 active 用户且没有本地 key"
		return result, nil
	}

	groupID, err := s.desiredGroupID(user)
	if err != nil {
		return result, err
	}
	name := desiredKeyName(user)
	result.Action = "create"
	result.GroupID = groupID
	result.Message = fmt.Sprintf("创建 Codesome key: name=%s group_id=%d", name, groupID)
	if dryRun {
		return result, nil
	}
	if s.service == nil {
		return result, fmt.Errorf("codesome key service is nil")
	}

	created, err := s.service.CreateKey(ctx, name, groupID)
	if err != nil {
		return result, fmt.Errorf("create key for user %s: %w", user.EmployeeNo, err)
	}
	if created == nil {
		return result, fmt.Errorf("create key for user %s returned nil", user.EmployeeNo)
	}
	status := created.Status
	if status == "" {
		status = repository.APIKeyStatusActive
	}
	keyName := created.Name
	if keyName == "" {
		keyName = desiredKeyName(user)
	}
	createdGroupID := created.GroupID
	if createdGroupID == 0 {
		createdGroupID = groupID
	}
	stored, err := s.keys.Create(ctx, repository.CreateAPIKeyParams{
		UserID:        user.ID,
		CodesomeKeyID: created.ID,
		Name:          keyName,
		Status:        status,
		GroupID:       createdGroupID,
		RawKey:        created.Key,
	})
	if err != nil {
		return result, err
	}
	result.CodesomeKeyID = stored.CodesomeKeyID
	result.GroupID = stored.GroupID
	result.RawKey = created.Key
	result.Message = fmt.Sprintf("已创建 Codesome key: id=%d name=%s group_id=%d status=%s", stored.CodesomeKeyID, stored.Name, stored.GroupID, stored.Status)
	return result, nil
}

func (s *UserSyncer) syncUserWithKey(ctx context.Context, user repository.User, key *repository.APIKey, dryRun bool, result UserSyncResult) (UserSyncResult, error) {
	desiredStatus := desiredKeyStatus(user)
	desiredName := desiredKeyName(user)
	desiredGroupID := key.GroupID
	if user.Status == repository.UserStatusActive {
		desiredGroupID = s.desiredExistingGroupID(user, key)
	}

	localUpdate := provider.CodesomeKeyUpdate{}
	if key.Status != desiredStatus {
		localUpdate.Status = &desiredStatus
	}
	if user.Status == repository.UserStatusActive && key.GroupID != desiredGroupID {
		localUpdate.GroupID = &desiredGroupID
	}
	if user.Status == repository.UserStatusActive && key.Name != desiredName {
		localUpdate.Name = &desiredName
	}

	result.CodesomeKeyID = key.CodesomeKeyID
	result.GroupID = desiredGroupID
	if localUpdate.Status == nil && localUpdate.GroupID == nil && localUpdate.Name == nil {
		result.Action = "noop"
		result.Message = "本地 key 状态已匹配"
		if dryRun {
			return result, nil
		}
		result.Action = "sync"
		result.Message = "重新应用期望状态到 Codesome key"
	} else {
		result.Action = "update"
		result.Message = describeKeyUpdate(key, localUpdate)
		if dryRun {
			return result, nil
		}
	}

	if s.service == nil {
		return result, fmt.Errorf("codesome key service is nil")
	}

	remoteUpdate := desiredRemoteUpdate(user, desiredName, desiredStatus, desiredGroupID)
	expectedName, expectedStatus, expectedGroupID := expectedSyncedFields(key, remoteUpdate)

	updated, err := s.service.UpdateKey(ctx, key.CodesomeKeyID, remoteUpdate)
	if err != nil {
		return result, fmt.Errorf("update key %d for user %s: %w", key.CodesomeKeyID, user.EmployeeNo, err)
	}
	if updated == nil {
		return result, fmt.Errorf("update key %d returned nil", key.CodesomeKeyID)
	}
	stored, err := s.keys.UpdateSynced(ctx, key.ID, repository.UpdateAPIKeyParams{
		Name:    fallbackString(updated.Name, expectedName),
		Status:  fallbackString(updated.Status, expectedStatus),
		GroupID: fallbackInt(updated.GroupID, expectedGroupID),
	})
	if err != nil {
		return result, err
	}
	result.GroupID = stored.GroupID
	result.Message = fmt.Sprintf("已同步 Codesome key: id=%d name=%s group_id=%d status=%s", stored.CodesomeKeyID, stored.Name, stored.GroupID, stored.Status)
	return result, nil
}

func (s *UserSyncer) desiredExistingGroupID(user repository.User, key *repository.APIKey) int {
	if user.CodesomeGroupID != nil {
		return *user.CodesomeGroupID
	}
	if s.defaultGroupID > 0 {
		return s.defaultGroupID
	}
	return key.GroupID
}

func (s *UserSyncer) desiredGroupID(user repository.User) (int, error) {
	if user.CodesomeGroupID != nil {
		return *user.CodesomeGroupID, nil
	}
	if s.defaultGroupID <= 0 {
		return 0, fmt.Errorf("user %s 缺少 Codesome group：请配置 codesome.default_group_id 或 user --group-id", user.EmployeeNo)
	}
	return s.defaultGroupID, nil
}

func desiredKeyName(user repository.User) string {
	return user.Name
}

func desiredKeyStatus(user repository.User) string {
	if user.Status == repository.UserStatusActive {
		return repository.APIKeyStatusActive
	}
	return repository.APIKeyStatusInactive
}

func describeKeyUpdate(key *repository.APIKey, update provider.CodesomeKeyUpdate) string {
	parts := ""
	if update.Name != nil {
		parts += fmt.Sprintf(" name:%s->%s", key.Name, *update.Name)
	}
	if update.GroupID != nil {
		parts += fmt.Sprintf(" group_id:%d->%d", key.GroupID, *update.GroupID)
	}
	if update.Status != nil {
		parts += fmt.Sprintf(" status:%s->%s", key.Status, *update.Status)
	}
	return "更新 Codesome key:" + parts
}

func desiredRemoteUpdate(user repository.User, desiredName string, desiredStatus string, desiredGroupID int) provider.CodesomeKeyUpdate {
	update := provider.CodesomeKeyUpdate{
		Status: &desiredStatus,
	}
	if user.Status == repository.UserStatusActive {
		update.Name = &desiredName
		update.GroupID = &desiredGroupID
	}
	return update
}

func expectedSyncedFields(key *repository.APIKey, update provider.CodesomeKeyUpdate) (string, string, int) {
	expectedName := key.Name
	if update.Name != nil {
		expectedName = *update.Name
	}
	expectedStatus := key.Status
	if update.Status != nil {
		expectedStatus = *update.Status
	}
	expectedGroupID := key.GroupID
	if update.GroupID != nil {
		expectedGroupID = *update.GroupID
	}
	return expectedName, expectedStatus, expectedGroupID
}

func isNotFound(err error) bool {
	return err != nil && errors.Is(err, sql.ErrNoRows)
}

func fallbackString(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func fallbackInt(value int, fallback int) int {
	if value == 0 {
		return fallback
	}
	return value
}
