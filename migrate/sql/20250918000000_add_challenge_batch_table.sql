/*
 * Copyright 2024 The Nakama Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

-- +migrate Up
-- 创建挑战赛批次号管理表（兼容PostgreSQL和CockroachDB）
-- 使用INTEGER类型确保两个数据库的兼容性
CREATE TABLE challenge_batch (
    challenge_id INTEGER PRIMARY KEY,
    current_batch INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 创建索引以提高查询性能
CREATE INDEX idx_challenge_batch_challenge_id ON challenge_batch(challenge_id);

-- +migrate Down
-- 删除表
DROP TABLE IF EXISTS challenge_batch;
