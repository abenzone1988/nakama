package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama/v3/game"
	"go.uber.org/zap"
)

func GetUserMeta(ctx context.Context, logger *zap.Logger, db *sql.DB, statusRegistry StatusRegistry) (*game.UserMeta, *api.User, error) {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)
	ids := []string{userID.String()}

	users, err := GetUsers(ctx, logger, db, statusRegistry, ids, nil, nil)
	if err != nil {
		return nil, nil, err
	}

	user := users.Users[0]
	var userMeta game.UserMeta
	if err := json.Unmarshal([]byte(user.Metadata), &userMeta); err != nil {
		logger.Error("json.Unmarshal user.Metadata", zap.Error(err), zap.String("metadata", user.Metadata))
		return nil, nil, err
	}

	return &userMeta, user, nil
}

func SaveUserMeta(ctx context.Context, logger *zap.Logger, db *sql.DB, userMeta *game.UserMeta) error {
	userID := ctx.Value(ctxUserIDKey{}).(uuid.UUID)
	metadataJSON, err := json.Marshal(userMeta)
	if err != nil {
		return err
	}
	if err = UpdateAccountMetadata(ctx, logger, db, userID, string(metadataJSON)); err != nil {
		logger.Error("update user Metadata err", zap.Error(err), zap.String("metadata", string(metadataJSON)))
		return err
	}
	return nil
}
