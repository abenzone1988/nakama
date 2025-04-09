// Copyright 2024 The Nakama Authors
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
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama/v3/console"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	ErrAnnouncementNotFound = errors.New("announcement not found")
)

func AnnouncementCreate(ctx context.Context, logger *zap.Logger, db *sql.DB, announcement *console.CreateAnnouncementRequest) (*console.Announcement, error) {
	id := uuid.Must(uuid.NewV4())
	now := time.Now().UTC()

	query := `
INSERT INTO announcement (id, title, content, img, status, create_time, update_time)
VALUES ($1, $2, $3, $4, $5, $6, $6)
RETURNING id, title, content, img, status, create_time, update_time`

	var dbAnnouncement console.Announcement
	var createTime, updateTime time.Time
	err := db.QueryRowContext(ctx, query,
		id,
		announcement.Title,
		announcement.Content,
		announcement.Img,
		announcement.Status,
		now,
	).Scan(&dbAnnouncement.Id,
		&dbAnnouncement.Title,
		&dbAnnouncement.Content,
		&dbAnnouncement.Img,
		&dbAnnouncement.Status,
		&createTime,
		&updateTime)

	if err != nil {
		logger.Error("Error creating announcement", zap.Error(err))
		return nil, err
	}

	dbAnnouncement.CreateTime = timestamppb.New(createTime)
	dbAnnouncement.UpdateTime = timestamppb.New(updateTime)

	return &dbAnnouncement, nil
}

func AnnouncementUpdate(ctx context.Context, logger *zap.Logger, db *sql.DB, announcement *console.UpdateAnnouncementRequest) (*console.Announcement, error) {
	now := time.Now().UTC()

	query := `
UPDATE announcement
SET title = $2, content = $3, img = $4, status = $5, update_time = $6
WHERE id = $1
RETURNING id, title, content, img, status, create_time, update_time`

	var dbAnnouncement console.Announcement
	var createTime, updateTime time.Time
	err := db.QueryRowContext(ctx, query,
		announcement.Id,
		announcement.Title,
		announcement.Content,
		announcement.Img,
		announcement.Status,
		now,
	).Scan(&dbAnnouncement.Id,
		&dbAnnouncement.Title,
		&dbAnnouncement.Content,
		&dbAnnouncement.Img,
		&dbAnnouncement.Status,
		&createTime,
		&updateTime)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrAnnouncementNotFound
		}
		logger.Error("Error updating announcement", zap.Error(err))
		return nil, err
	}

	dbAnnouncement.CreateTime = timestamppb.New(createTime)
	dbAnnouncement.UpdateTime = timestamppb.New(updateTime)

	return &dbAnnouncement, nil
}

func AnnouncementDelete(ctx context.Context, logger *zap.Logger, db *sql.DB, id string) error {
	query := "DELETE FROM announcement WHERE id = $1"
	result, err := db.ExecContext(ctx, query, id)
	if err != nil {
		logger.Error("Error deleting announcement", zap.Error(err))
		return err
	}

	if rowsAffected, _ := result.RowsAffected(); rowsAffected == 0 {
		return ErrAnnouncementNotFound
	}

	return nil
}

func AnnouncementGet(ctx context.Context, logger *zap.Logger, db *sql.DB, id string) (*console.Announcement, error) {
	query := `
SELECT id, title, content, img, status, create_time, update_time
FROM announcement
WHERE id = $1`

	var announcement console.Announcement
	var createTime, updateTime time.Time
	err := db.QueryRowContext(ctx, query, id).Scan(
		&announcement.Id,
		&announcement.Title,
		&announcement.Content,
		&announcement.Img,
		&announcement.Status,
		&createTime,
		&updateTime)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrAnnouncementNotFound
		}
		logger.Error("Error getting announcement", zap.Error(err))
		return nil, err
	}

	announcement.CreateTime = timestamppb.New(createTime)
	announcement.UpdateTime = timestamppb.New(updateTime)

	return &announcement, nil
}

func AnnouncementList(ctx context.Context, logger *zap.Logger, db *sql.DB, status int32, limit int32, cursor string) (*console.AnnouncementList, error) {
	// 先构建条件
	conditions := "WHERE 1=1"
	params := make([]interface{}, 0, 3)
	paramCount := 0

	if status >= 0 {
		paramCount++
		params = append(params, status)
		conditions += " AND status = $" + strconv.Itoa(paramCount)
	}

	// 先查询总数
	countQuery := "SELECT COUNT(*) FROM announcement " + conditions
	var totalCount int32
	err := db.QueryRowContext(ctx, countQuery, params...).Scan(&totalCount)
	if err != nil {
		logger.Error("Error counting announcements", zap.Error(err))
		return nil, err
	}

	// 构建分页查询
	query := `
SELECT id, title, content, img, status, create_time, update_time
FROM announcement
` + conditions

	if cursor != "" {
		paramCount++
		params = append(params, cursor)
		query += " AND create_time < (SELECT create_time FROM announcement WHERE id = $" + strconv.Itoa(paramCount) + ")"
	}

	paramCount++
	params = append(params, limit)
	query += " ORDER BY create_time DESC LIMIT $" + strconv.Itoa(paramCount)

	rows, err := db.QueryContext(ctx, query, params...)
	if err != nil {
		logger.Error("Error listing announcements", zap.Error(err))
		return nil, err
	}
	defer rows.Close()

	announcements := make([]*console.Announcement, 0, limit)
	var nextCursor string

	for rows.Next() {
		var announcement console.Announcement
		var createTime, updateTime time.Time
		err = rows.Scan(
			&announcement.Id,
			&announcement.Title,
			&announcement.Content,
			&announcement.Img,
			&announcement.Status,
			&createTime,
			&updateTime)

		if err != nil {
			logger.Error("Error scanning announcement row", zap.Error(err))
			return nil, err
		}

		announcement.CreateTime = timestamppb.New(createTime)
		announcement.UpdateTime = timestamppb.New(updateTime)
		announcements = append(announcements, &announcement)
		nextCursor = announcement.Id
	}

	if err = rows.Err(); err != nil {
		logger.Error("Error iterating announcement rows", zap.Error(err))
		return nil, err
	}

	result := &console.AnnouncementList{
		Announcements: announcements,
		TotalCount:    totalCount,
	}

	if len(announcements) == int(limit) {
		result.NextCursor = nextCursor
	}

	return result, nil
}
