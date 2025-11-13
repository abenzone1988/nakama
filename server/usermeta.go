package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sync"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama/v3/game"
	"go.uber.org/zap"
)

// userMetaContextKey 用于在 context 中存储 UserMeta 管理器
type userMetaContextKey struct{}

// userMetaLockPool 用户级别的锁池，确保同一用户的请求串行化处理
var (
	userMetaLocks   = make(map[uuid.UUID]*sync.RWMutex)
	userMetaLocksMu sync.Mutex
)

// getUserMetaLock 获取或创建用户级别的锁
func getUserMetaLock(userID uuid.UUID) *sync.RWMutex {
	userMetaLocksMu.Lock()
	defer userMetaLocksMu.Unlock()

	lock, exists := userMetaLocks[userID]
	if !exists {
		lock = &sync.RWMutex{}
		userMetaLocks[userID] = lock
	}
	return lock
}

// UserMetaManager 管理 UserMeta 的加载、修改和保存
type UserMetaManager struct {
	logger         *zap.Logger
	db             *sql.DB
	statusRegistry StatusRegistry
	userID         uuid.UUID

	// userLock 是用户级别的锁，确保同一用户的并发请求串行化
	userLock *sync.RWMutex
	// localMu 保护本实例的 loaded/dirty 状态
	localMu sync.Mutex
	loaded  bool
	dirty   bool

	userMeta *game.UserMeta
	user     *api.User
}

// newUserMetaManager 创建新的 UserMeta 管理器
func newUserMetaManager(logger *zap.Logger, db *sql.DB, statusRegistry StatusRegistry, userID uuid.UUID) *UserMetaManager {
	return &UserMetaManager{
		logger:         logger,
		db:             db,
		statusRegistry: statusRegistry,
		userID:         userID,
		userLock:       getUserMetaLock(userID), // 获取用户级别的锁
		loaded:         false,
		dirty:          false,
	}
}

// load 从数据库加载 UserMeta（延迟加载）
// 使用用户级别的写锁，确保同一用户的并发请求不会重复加载
func (m *UserMetaManager) load(ctx context.Context) error {
	m.localMu.Lock()
	if m.loaded {
		m.localMu.Unlock()
		return nil
	}
	m.localMu.Unlock()

	// 使用用户级别的写锁，确保同一用户的请求串行化
	m.userLock.Lock()
	defer m.userLock.Unlock()

	// 双重检查，避免在获取锁期间其他请求已经加载
	m.localMu.Lock()
	if m.loaded {
		m.localMu.Unlock()
		return nil
	}
	m.localMu.Unlock()

	ids := []string{m.userID.String()}
	users, err := GetUsers(ctx, m.logger, m.db, m.statusRegistry, ids, nil, nil)
	if err != nil {
		return err
	}

	m.user = users.Users[0]
	m.userMeta = &game.UserMeta{}

	// 如果 metadata 不为空，则解析
	if m.user.Metadata != "" {
		if err := json.Unmarshal([]byte(m.user.Metadata), m.userMeta); err != nil {
			m.logger.Error("json.Unmarshal user.Metadata",
				zap.Error(err),
				zap.String("metadata", m.user.Metadata))
			return err
		}
	}

	m.localMu.Lock()
	m.loaded = true
	m.localMu.Unlock()
	return nil
}

// Get 获取 UserMeta（如果未加载则自动加载）
func (m *UserMetaManager) Get(ctx context.Context) (*game.UserMeta, *api.User, error) {
	if err := m.load(ctx); err != nil {
		return nil, nil, err
	}

	// 使用用户级别的读锁，允许并发读取
	m.userLock.RLock()
	defer m.userLock.RUnlock()
	return m.userMeta, m.user, nil
}

// MarkDirty 标记 UserMeta 已被修改，需要保存
func (m *UserMetaManager) MarkDirty() {
	m.localMu.Lock()
	defer m.localMu.Unlock()
	m.dirty = true
}

// Save 保存 UserMeta 到数据库（仅当标记为 dirty 时）
// 使用用户级别的写锁，确保同一用户的并发请求串行化保存
func (m *UserMetaManager) Save(ctx context.Context) error {
	m.localMu.Lock()
	needsSave := m.loaded && m.dirty
	if !needsSave {
		m.localMu.Unlock()
		return nil
	}
	m.localMu.Unlock()

	// 使用用户级别的写锁，确保同一用户的请求串行化保存
	m.userLock.Lock()
	defer m.userLock.Unlock()

	// 双重检查，避免在获取锁期间其他请求已经保存
	m.localMu.Lock()
	if !m.loaded || !m.dirty {
		m.localMu.Unlock()
		return nil
	}
	// 保存当前请求的修改（用户级别的锁已确保串行化）
	metadataJSON, err := json.Marshal(m.userMeta)
	if err != nil {
		m.localMu.Unlock()
		m.logger.Error("json.Marshal userMeta failed", zap.Error(err))
		return err
	}

	if err = UpdateAccountMetadata(ctx, m.logger, m.db, m.userID, string(metadataJSON)); err != nil {
		m.localMu.Unlock()
		m.logger.Error("update user Metadata err",
			zap.Error(err),
			zap.String("metadata", string(metadataJSON)))
		return err
	}

	m.dirty = false
	m.localMu.Unlock()
	return nil
}

// IsDirty 返回 UserMeta 是否被修改
func (m *UserMetaManager) IsDirty() bool {
	m.localMu.Lock()
	defer m.localMu.Unlock()
	return m.dirty
}

// WithUserMetaManager 在 context 中添加 UserMetaManager
func WithUserMetaManager(ctx context.Context, logger *zap.Logger, db *sql.DB, statusRegistry StatusRegistry) context.Context {
	userID, ok := ctx.Value(ctxUserIDKey{}).(uuid.UUID)
	if !ok {
		// 如果没有 userID，返回原 context
		return ctx
	}

	manager := newUserMetaManager(logger, db, statusRegistry, userID)
	return context.WithValue(ctx, userMetaContextKey{}, manager)
}

// GetUserMetaManager 从 context 获取 UserMetaManager
func GetUserMetaManager(ctx context.Context) *UserMetaManager {
	manager, ok := ctx.Value(userMetaContextKey{}).(*UserMetaManager)
	if !ok {
		return nil
	}
	return manager
}

// GetUserMeta 从 context 中获取 UserMeta（延迟加载）
// 这个函数需要 UserMetaManager 存在于 context 中（通过拦截器自动添加）
func GetUserMeta(ctx context.Context) (*game.UserMeta, *api.User, error) {
	manager := GetUserMetaManager(ctx)
	if manager == nil {
		return nil, nil, errors.New("UserMetaManager not found in context")
	}
	return manager.Get(ctx)
}

// UpdateUserMeta 更新 UserMeta 并标记为需要保存
// 这是一个便捷函数，用于修改 UserMeta
func UpdateUserMeta(ctx context.Context, updateFunc func(*game.UserMeta) error) error {
	manager := GetUserMetaManager(ctx)
	if manager == nil {
		return nil // 没有 manager，忽略更新
	}

	userMeta, _, err := manager.Get(ctx)
	if err != nil {
		return err
	}

	if err := updateFunc(userMeta); err != nil {
		return err
	}

	manager.MarkDirty()
	return nil
}

// SaveUserMeta 手动保存 UserMeta 到数据库
// 注意：通常不需要手动调用此函数，拦截器会自动保存
// 仅在特殊情况下（如需要立即保存）才使用
func SaveUserMeta(ctx context.Context) error {
	manager := GetUserMetaManager(ctx)
	if manager == nil {
		return errors.New("UserMetaManager not found in context")
	}
	return manager.Save(ctx)
}
