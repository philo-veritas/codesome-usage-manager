package syncer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"codesome-usage-manager/internal/provider"
	"codesome-usage-manager/internal/repository"
)

type UserKeyService interface {
	CreateKey(ctx context.Context, name string, groupID int) (*provider.CodesomeApiKeyWithSecret, error)
	UpdateKey(ctx context.Context, keyID int, update provider.CodesomeKeyUpdate) (*provider.CodesomeApiKey, error)
}

type DefaultGroupIDResolver func(ctx context.Context) (int, error)

type UserSyncer struct {
	users                  *repository.UserRepository
	keys                   *repository.APIKeyRepository
	service                UserKeyService
	defaultGroupID         int
	defaultGroupIDResolver DefaultGroupIDResolver
	resolvedDefaultGroupID *int
	planRuntimeGroup       bool
}

type UserSyncOptions struct {
	DryRun     bool
	EmployeeNo string
	Full       bool
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

func (s *UserSyncer) WithDefaultGroupIDResolver(resolver DefaultGroupIDResolver) *UserSyncer {
	s.defaultGroupIDResolver = resolver
	return s
}

func (s *UserSyncer) WithRuntimeGroupSelectionPlan() *UserSyncer {
	s.planRuntimeGroup = true
	return s
}

func (s *UserSyncer) SyncUsers(ctx context.Context, options UserSyncOptions) ([]UserSyncResult, error) {
	users, err := s.resolveUsers(ctx, options.EmployeeNo)
	if err != nil {
		return nil, err
	}

	results := make([]UserSyncResult, 0, len(users))
	for _, user := range users {
		result, err := s.syncUser(ctx, user, options.DryRun, options.Full)
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

func (s *UserSyncer) syncUser(ctx context.Context, user repository.User, dryRun bool, full bool) (UserSyncResult, error) {
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
	return s.syncUserWithKey(ctx, user, key, dryRun, full, result)
}

func (s *UserSyncer) syncUserWithoutKey(ctx context.Context, user repository.User, dryRun bool, result UserSyncResult) (UserSyncResult, error) {
	if user.Status != repository.UserStatusActive {
		result.Action = "noop"
		result.Message = "非 active 用户且没有本地 key"
		return result, nil
	}

	if dryRun && (s.usesRuntimeGroupSelection(user) || s.canPlanMissingDefaultGroup(user)) {
		name := desiredKeyName(user)
		result.Action = "create"
		result.Message = fmt.Sprintf("创建 Codesome key: name=%s group_id=<真实运行时选择可用余额最多的 group>", name)
		return result, nil
	}

	groupID, err := s.desiredGroupID(ctx, user)
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

func (s *UserSyncer) syncUserWithKey(ctx context.Context, user repository.User, key *repository.APIKey, dryRun bool, full bool, result UserSyncResult) (UserSyncResult, error) {
	desiredStatus := desiredKeyStatus(user)
	desiredName := desiredKeyName(user)

	desiredGroupID := s.localDesiredGroupIDForChangeCheck(user, key)
	if user.Status == repository.UserStatusActive && !(dryRun && s.usesRuntimeGroupSelection(user)) {
		resolvedGroupID, err := s.desiredExistingGroupID(ctx, user, key)
		if err != nil {
			return result, err
		}
		desiredGroupID = resolvedGroupID
	}
	localUpdate := desiredKeyUpdate(user, key, desiredName, desiredStatus, desiredGroupID)

	result.CodesomeKeyID = key.CodesomeKeyID
	result.GroupID = desiredGroupID
	if dryRun && user.Status == repository.UserStatusActive && s.usesRuntimeGroupSelection(user) {
		return s.planUserWithRuntimeGroupSelection(key, desiredName, desiredStatus, result)
	}
	needsSync := shouldSyncExistingUser(user, key, localUpdate, full)
	if !needsSync {
		result.Action = "noop"
		result.Message = "本地 key 状态已匹配，未检测到本地变更"
		return result, nil
	}
	if localUpdate.Status == nil && localUpdate.GroupID == nil && localUpdate.Name == nil {
		result.Action = "sync"
		result.Message = "重新应用期望状态到 Codesome key"
		if dryRun {
			return result, nil
		}
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

func (s *UserSyncer) planUserWithRuntimeGroupSelection(
	key *repository.APIKey,
	desiredName string,
	desiredStatus string,
	result UserSyncResult,
) (UserSyncResult, error) {
	update := provider.CodesomeKeyUpdate{}
	if key.Status != desiredStatus {
		update.Status = &desiredStatus
	}
	if key.Name != desiredName {
		update.Name = &desiredName
	}

	result.CodesomeKeyID = key.CodesomeKeyID
	result.GroupID = 0
	if update.Status == nil && update.Name == nil {
		result.Action = "sync"
		result.Message = "真实运行时按可用余额最多的 group 评估 Codesome key"
		return result, nil
	}
	result.Action = "update"
	result.Message = describeKeyUpdate(key, update) + " group_id=<真实运行时选择可用余额最多的 group>"
	return result, nil
}

func (s *UserSyncer) desiredExistingGroupID(ctx context.Context, user repository.User, key *repository.APIKey) (int, error) {
	if user.CodesomeGroupID != nil {
		return *user.CodesomeGroupID, nil
	}
	groupID, err := s.resolveDefaultGroupID(ctx)
	if err == nil && groupID > 0 {
		return groupID, nil
	}
	return key.GroupID, nil
}

func (s *UserSyncer) desiredGroupID(ctx context.Context, user repository.User) (int, error) {
	if user.CodesomeGroupID != nil {
		return *user.CodesomeGroupID, nil
	}
	groupID, err := s.resolveDefaultGroupID(ctx)
	if err != nil {
		return 0, err
	}
	if groupID <= 0 {
		return 0, fmt.Errorf("user %s 缺少 Codesome group：请配置 codesome.default_group_id 或 user --group-id", user.EmployeeNo)
	}
	return groupID, nil
}

func (s *UserSyncer) resolveDefaultGroupID(ctx context.Context) (int, error) {
	if s.defaultGroupIDResolver != nil {
		groupID, err := s.resolveRuntimeDefaultGroupID(ctx)
		if err == nil {
			return groupID, nil
		}
		if s.defaultGroupID <= 0 {
			return 0, err
		}
	}
	return s.defaultGroupID, nil
}

func (s *UserSyncer) resolveRuntimeDefaultGroupID(ctx context.Context) (int, error) {
	if s.resolvedDefaultGroupID != nil {
		return *s.resolvedDefaultGroupID, nil
	}
	groupID, err := s.defaultGroupIDResolver(ctx)
	if err != nil {
		return 0, err
	}
	if groupID <= 0 {
		return 0, fmt.Errorf("运行时 Codesome group 选择返回无效 group_id: %d", groupID)
	}
	s.resolvedDefaultGroupID = &groupID
	return groupID, nil
}

func (s *UserSyncer) usesRuntimeGroupSelection(user repository.User) bool {
	return user.CodesomeGroupID == nil && s.planRuntimeGroup
}

func (s *UserSyncer) canPlanMissingDefaultGroup(user repository.User) bool {
	return user.CodesomeGroupID == nil && s.defaultGroupID <= 0 && s.defaultGroupIDResolver == nil
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

func (s *UserSyncer) localDesiredGroupIDForChangeCheck(user repository.User, key *repository.APIKey) int {
	if user.Status != repository.UserStatusActive {
		return key.GroupID
	}
	if user.CodesomeGroupID != nil {
		return *user.CodesomeGroupID
	}
	if s.defaultGroupID > 0 {
		return s.defaultGroupID
	}
	return key.GroupID
}

func desiredKeyUpdate(user repository.User, key *repository.APIKey, desiredName string, desiredStatus string, desiredGroupID int) provider.CodesomeKeyUpdate {
	update := provider.CodesomeKeyUpdate{}
	if key.Status != desiredStatus {
		update.Status = &desiredStatus
	}
	if user.Status == repository.UserStatusActive && key.GroupID != desiredGroupID {
		update.GroupID = &desiredGroupID
	}
	if user.Status == repository.UserStatusActive && key.Name != desiredName {
		update.Name = &desiredName
	}
	return update
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

func shouldSyncExistingUser(user repository.User, key *repository.APIKey, update provider.CodesomeKeyUpdate, full bool) bool {
	if full || hasKeyUpdate(update) || key.LastSyncedAt == nil {
		return true
	}
	return userChangedAfterLastSync(user.UpdatedAt, *key.LastSyncedAt)
}

func hasKeyUpdate(update provider.CodesomeKeyUpdate) bool {
	return update.Status != nil || update.GroupID != nil || update.Name != nil
}

func userChangedAfterLastSync(userUpdatedAt string, lastSyncedAt string) bool {
	userUpdated, err := time.Parse(time.RFC3339, userUpdatedAt)
	if err != nil {
		return true
	}
	lastSynced, err := time.Parse(time.RFC3339, lastSyncedAt)
	if err != nil {
		return true
	}
	return userUpdated.After(lastSynced)
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
