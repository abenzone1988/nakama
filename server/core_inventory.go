// Copyright 2018 The Nakama Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"
)

type inventoryLedgerListCursor struct {
	UserId     string
	CreateTime time.Time
	Id         string
	IsNext     bool
}

// Not an API entity, only used to receive data from runtime environment.
type inventoryUpdate struct {
	UserID    uuid.UUID
	Changeset map[string]int64
	// Metadata is expected to be a valid JSON string already.
	Metadata string
	// ExternalID is the external UUID for inventory ledger record
	ExternalID *uuid.UUID
}

// Not an API entity, only used to send data to runtime environment.
type inventoryLedger struct {
	ID         string
	UserID     string
	Changeset  map[string]int64
	Metadata   map[string]interface{}
	CreateTime int64
	UpdateTime int64
}

func (i *inventoryLedger) GetID() string {
	return i.ID
}

func (i *inventoryLedger) GetUserID() string {
	return i.UserID
}

func (i *inventoryLedger) GetCreateTime() int64 {
	return i.CreateTime
}

func (i *inventoryLedger) GetUpdateTime() int64 {
	return i.UpdateTime
}

func (i *inventoryLedger) GetChangeset() map[string]int64 {
	return i.Changeset
}

func (i *inventoryLedger) GetMetadata() map[string]interface{} {
	return i.Metadata
}

// InventoryNegativeError 背包物品不足错误
type InventoryNegativeError struct {
	UserID  string
	ItemID  string
	Current int64
	Amount  int64
}

func (e *InventoryNegativeError) Error() string {
	return fmt.Sprintf("inventory cannot go negative for user_id %v item_id %v, current %v, amount %v", e.UserID, e.ItemID, e.Current, e.Amount)
}

// UpdateInventories 更新玩家背包
func UpdateInventories(ctx context.Context, logger *zap.Logger, db *sql.DB, updates []*inventoryUpdate, updateLedger bool) ([]*runtime.WalletUpdateResult, error) {
	if len(updates) == 0 {
		return nil, nil
	}

	var results []*runtime.WalletUpdateResult

	if err := ExecuteInTxPgx(ctx, db, func(tx pgx.Tx) error {
		var updateErr error
		results, updateErr = updateInventories(ctx, logger, tx, updates, updateLedger)
		if updateErr != nil {
			logger.Error("UpdateInventories: updateInventories执行失败", zap.Error(updateErr))
			return updateErr
		}
		return nil
	}); err != nil {
		if _, ok := err.(*InventoryNegativeError); !ok {
			logger.Error("Error updating inventories.", zap.Error(err))
		}
		// Ensure there are no partially updated inventories returned as results, they would not be reflected in database anyway.
		for _, result := range results {
			result.Updated = nil
		}
		return results, err
	}

	return results, nil
}

func updateInventories(ctx context.Context, logger *zap.Logger, tx pgx.Tx, updates []*inventoryUpdate, updateLedger bool) ([]*runtime.WalletUpdateResult, error) {
	if len(updates) == 0 {
		return nil, nil
	}

	ids := make([]uuid.UUID, 0, len(updates))
	for _, update := range updates {
		ids = append(ids, update.UserID)
	}

	initialQuery := "SELECT id, inventory FROM users WHERE id = ANY($1::UUID[]) FOR UPDATE"

	// Select the inventories from the DB and decode them.
	inventories := make(map[string]map[string]int64, len(updates))
	rows, err := tx.Query(ctx, initialQuery, ids)
	if err != nil {
		logger.Debug("Error retrieving user inventories.", zap.Error(err))
		return nil, err
	}
	for rows.Next() {
		var id string
		var inventory sql.NullString
		err = rows.Scan(&id, &inventory)
		if err != nil {
			rows.Close()
			logger.Debug("Error reading user inventories.", zap.Error(err))
			return nil, err
		}

		var inventoryMap map[string]int64
		err = json.Unmarshal([]byte(inventory.String), &inventoryMap)
		if err != nil {
			rows.Close()
			logger.Debug("Error converting user inventory.", zap.String("user_id", id), zap.Error(err))
			return nil, err
		}

		inventories[id] = inventoryMap
	}
	rows.Close()

	results := make([]*runtime.WalletUpdateResult, 0, len(updates))

	// Prepare the set of inventory updates and ledger updates.
	updatedInventories := make(map[string][]byte, len(updates))
	updateOrder := make([]string, 0, len(updates))

	var idParams []uuid.UUID
	var userIdParams []string
	var changesetParams [][]byte
	var metadataParams []string
	if updateLedger {
		idParams = make([]uuid.UUID, 0, len(updates))
		userIdParams = make([]string, 0, len(updates))
		changesetParams = make([][]byte, 0, len(updates))
		metadataParams = make([]string, 0, len(updates))
	}

	// Go through the changesets and attempt to calculate the new state for each inventory.
	for _, update := range updates {
		userID := update.UserID.String()
		inventoryMap, ok := inventories[userID]
		if !ok {
			// Inventory update for a user that does not exist. Skip it.
			continue
		}

		// 只记录有变化的物品的旧值
		previousMap := make(map[string]int64, len(update.Changeset))
		updatedMap := make(map[string]int64, len(update.Changeset))

		for itemID := range update.Changeset {
			previousMap[itemID] = inventoryMap[itemID]
		}

		result := &runtime.WalletUpdateResult{UserID: userID, Previous: previousMap}

		for itemID, v := range update.Changeset {
			// Existing value may be 0 or missing.
			newValue := inventoryMap[itemID] + v
			if newValue < 0 {
				// Insufficient items
				return nil, &InventoryNegativeError{
					UserID:  userID,
					ItemID:  itemID,
					Current: inventoryMap[itemID],
					Amount:  v,
				}
			}
			inventoryMap[itemID] = newValue
			updatedMap[itemID] = newValue
		}

		result.Updated = updatedMap
		results = append(results, result)

		inventoryData, err := json.Marshal(inventoryMap)
		if err != nil {
			logger.Debug("Error converting new user inventory.", zap.String("user_id", userID), zap.Error(err))
			return nil, err
		}
		updatedInventories[userID] = inventoryData
		updateOrder = append(updateOrder, userID)

		// Prepare ledger updates if needed.
		if updateLedger {
			changesetData, err := json.Marshal(update.Changeset)
			if err != nil {
				logger.Debug("Error converting new user inventory changeset.", zap.String("user_id", update.UserID.String()), zap.Error(err))
				return nil, err
			}

			// 使用外部传入的ID，如果没有则生成新的UUID
			var ledgerID uuid.UUID
			if update.ExternalID != nil {
				ledgerID = *update.ExternalID
			} else {
				ledgerID = uuid.Must(uuid.NewV4())
			}

			idParams = append(idParams, ledgerID)
			userIdParams = append(userIdParams, userID)
			changesetParams = append(changesetParams, changesetData)
			metadataParams = append(metadataParams, update.Metadata)
		}
	}

	if len(updatedInventories) > 0 {
		// Ensure updates are done in natural order of user ID.
		sort.Strings(updateOrder)

		// Write the updated inventories.
		for _, userID := range updateOrder {
			updatedInventory, ok := updatedInventories[userID]
			if !ok {
				// Should not happen.
				logger.Warn("Missing inventory update for user.", zap.String("user_id", userID))
				continue
			}
			_, err = tx.Exec(ctx, "UPDATE users SET update_time = now(), inventory = $2 WHERE id = $1", userID, updatedInventory)
			if err != nil {
				logger.Debug("Error writing user inventory.", zap.String("user_id", userID), zap.Error(err))
				return nil, err
			}
		}

		// Write the ledger updates, if any.
		if updateLedger && (len(idParams) > 0) {
			_, err = tx.Exec(ctx, `
INSERT INTO
	inventory_ledger (id, user_id, changeset, metadata)
SELECT
	unnest($1::uuid[]), unnest($2::uuid[]), unnest($3::jsonb[]), unnest($4::jsonb[]);
`, idParams, userIdParams, changesetParams, metadataParams)
			if err != nil {
				logger.Error("Error writing user inventory ledgers.", zap.Error(err))
				return nil, err
			}
		}
	}

	return results, nil
}

// UpdateInventoryLedger 更新背包变化记录的元数据
func UpdateInventoryLedger(ctx context.Context, logger *zap.Logger, db *sql.DB, id uuid.UUID, metadata string) (*inventoryLedger, error) {
	// Metadata is expected to already be a valid JSON string.
	var userID string
	var changeset sql.NullString
	var createTime pgtype.Timestamptz
	var updateTime pgtype.Timestamptz
	query := "UPDATE inventory_ledger SET update_time = now(), metadata = metadata || $2 WHERE id = $1::UUID RETURNING user_id, changeset, create_time, update_time"
	err := db.QueryRowContext(ctx, query, id, metadata).Scan(&userID, &changeset, &createTime, &updateTime)
	if err != nil {
		logger.Error("Error updating user inventory ledger.", zap.String("id", id.String()), zap.Error(err))
		return nil, err
	}

	var changesetMap map[string]int64
	err = json.Unmarshal([]byte(changeset.String), &changesetMap)
	if err != nil {
		logger.Error("Error converting user inventory ledger changeset after update.", zap.String("id", id.String()), zap.Error(err))
		return nil, err
	}

	return &inventoryLedger{
		UserID:     userID,
		Changeset:  changesetMap,
		CreateTime: createTime.Time.Unix(),
		UpdateTime: updateTime.Time.Unix(),
	}, nil
}

// ListInventoryLedger 查询背包变化记录
func ListInventoryLedger(ctx context.Context, logger *zap.Logger, db *sql.DB, userID uuid.UUID, limit *int, cursor string) ([]*inventoryLedger, string, string, error) {
	var incomingCursor *inventoryLedgerListCursor
	if cursor != "" {
		cb, err := base64.URLEncoding.DecodeString(cursor)
		if err != nil {
			return nil, "", "", runtime.ErrWalletLedgerInvalidCursor
		}
		incomingCursor = &inventoryLedgerListCursor{}
		if err := gob.NewDecoder(bytes.NewReader(cb)).Decode(incomingCursor); err != nil {
			return nil, "", "", runtime.ErrWalletLedgerInvalidCursor
		}

		// Cursor and filter mismatch. Perhaps the caller has sent an old cursor with a changed filter.
		if userID.String() != incomingCursor.UserId {
			return nil, "", "", runtime.ErrWalletLedgerInvalidCursor
		}
	}

	params := []interface{}{userID, time.Now().UTC(), uuid.UUID{}}
	if incomingCursor != nil {
		params[1] = incomingCursor.CreateTime
		params[2] = incomingCursor.Id
	}

	query := `SELECT id, changeset, metadata, create_time, update_time FROM inventory_ledger WHERE user_id = $1::UUID AND (user_id, create_time, id) < ($1::UUID, $2, $3::UUID) ORDER BY create_time DESC`
	if incomingCursor != nil && !incomingCursor.IsNext {
		query = `SELECT id, changeset, metadata, create_time, update_time FROM inventory_ledger WHERE user_id = $1::UUID AND (user_id, create_time, id) > ($1::UUID, $2, $3::UUID) ORDER BY create_time ASC`
	}

	if limit != nil {
		query = fmt.Sprintf(`%s LIMIT %v`, query, *limit+1)
	}

	results := make([]*inventoryLedger, 0, 10)
	rows, err := db.QueryContext(ctx, query, params...)
	if err != nil {
		logger.Error("Error retrieving user inventory ledger.", zap.String("user_id", userID.String()), zap.Error(err))
		return nil, "", "", err
	}
	defer rows.Close()

	var id string
	var changeset sql.NullString
	var metadata sql.NullString
	var createTime pgtype.Timestamptz
	var updateTime pgtype.Timestamptz
	var nextCursor *inventoryLedgerListCursor
	var prevCursor *inventoryLedgerListCursor
	for rows.Next() {
		if limit != nil && len(results) >= *limit {
			nextCursor = &inventoryLedgerListCursor{
				UserId:     userID.String(),
				Id:         id,
				CreateTime: createTime.Time,
				IsNext:     true,
			}
			break
		}

		err = rows.Scan(&id, &changeset, &metadata, &createTime, &updateTime)
		if err != nil {
			logger.Error("Error converting user inventory ledger.", zap.String("user_id", userID.String()), zap.Error(err))
			return nil, "", "", err
		}

		var changesetMap map[string]int64
		err = json.Unmarshal([]byte(changeset.String), &changesetMap)
		if err != nil {
			logger.Error("Error converting user inventory ledger changeset.", zap.String("user_id", userID.String()), zap.Error(err))
			return nil, "", "", err
		}

		var metadataMap map[string]interface{}
		err = json.Unmarshal([]byte(metadata.String), &metadataMap)
		if err != nil {
			logger.Error("Error converting user inventory ledger metadata.", zap.String("user_id", userID.String()), zap.Error(err))
			return nil, "", "", err
		}

		results = append(results, &inventoryLedger{
			ID:         id,
			Changeset:  changesetMap,
			Metadata:   metadataMap,
			CreateTime: createTime.Time.Unix(),
			UpdateTime: updateTime.Time.Unix(),
		})

		if incomingCursor != nil && prevCursor == nil {
			prevCursor = &inventoryLedgerListCursor{
				UserId:     userID.String(),
				Id:         id,
				CreateTime: createTime.Time,
				IsNext:     false,
			}
		}
	}

	if incomingCursor != nil && !incomingCursor.IsNext {
		if nextCursor != nil && prevCursor != nil {
			nextCursor, nextCursor.IsNext, prevCursor, prevCursor.IsNext = prevCursor, prevCursor.IsNext, nextCursor, nextCursor.IsNext
		} else if nextCursor != nil {
			nextCursor, prevCursor = nil, nextCursor
			prevCursor.IsNext = !prevCursor.IsNext
		} else if prevCursor != nil {
			nextCursor, prevCursor = prevCursor, nil
			nextCursor.IsNext = !nextCursor.IsNext
		}

		for i, j := 0, len(results)-1; i < j; i, j = i+1, j-1 {
			results[i], results[j] = results[j], results[i]
		}
	}

	var nextCursorStr string
	if nextCursor != nil {
		cursorBuf := new(bytes.Buffer)
		if err := gob.NewEncoder(cursorBuf).Encode(nextCursor); err != nil {
			logger.Error("Error creating inventory ledger list cursor", zap.Error(err))
			return nil, "", "", err
		}
		nextCursorStr = base64.URLEncoding.EncodeToString(cursorBuf.Bytes())
	}

	var prevCursorStr string
	if prevCursor != nil {
		cursorBuf := new(bytes.Buffer)
		if err := gob.NewEncoder(cursorBuf).Encode(prevCursor); err != nil {
			logger.Error("Error creating inventory ledger list cursor", zap.Error(err))
			return nil, "", "", err
		}
		prevCursorStr = base64.URLEncoding.EncodeToString(cursorBuf.Bytes())
	}

	return results, nextCursorStr, prevCursorStr, nil
}

// GetInventory 获取玩家背包数据
func GetInventory(ctx context.Context, logger *zap.Logger, db *sql.DB, userID uuid.UUID) (map[string]int64, error) {
	query := `
		SELECT metadata
		FROM wallet_ledger
		WHERE user_id = $1
		AND metadata->>'type' = 'inventory'
		ORDER BY update_time DESC
		LIMIT 1
	`

	var metadataJSON string
	err := db.QueryRowContext(ctx, query, userID.String()).Scan(&metadataJSON)
	if err == sql.ErrNoRows {
		return make(map[string]int64), nil
	}
	if err != nil {
		return nil, err
	}

	var metadata struct {
		Items map[string]int64 `json:"items"`
	}

	if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
		return nil, err
	}

	return metadata.Items, nil
}
