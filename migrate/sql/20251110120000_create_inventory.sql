-- Copyright 2024 The Nakama Authors
--
-- Licensed under the Apache License, Version 2.0 (the "License");
-- you may not use this file except in compliance with the License.
-- You may obtain a copy of the License at
--
-- http://www.apache.org/licenses/LICENSE-2.0
--
-- Unless required by applicable law or agreed to in writing, software
-- distributed under the License is distributed on an "AS IS" BASIS,
-- WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
-- See the License for the specific language governing permissions and
-- limitations under the License.

-- +migrate Up
-- 添加 inventory 字段到 users 表
ALTER TABLE users ADD COLUMN IF NOT EXISTS inventory JSONB NOT NULL DEFAULT '{}';

-- 创建 inventory_ledger 表用于记录背包变化
CREATE TABLE IF NOT EXISTS inventory_ledger (
    PRIMARY KEY (user_id, create_time, id),
    FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,

    id          UUID        NOT NULL UNIQUE,
    user_id     UUID        NOT NULL,
    changeset   JSONB       NOT NULL,
    metadata    JSONB       NOT NULL,
    create_time TIMESTAMPTZ NOT NULL DEFAULT now(),
    update_time TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 创建索引以优化查询性能
CREATE INDEX IF NOT EXISTS inventory_ledger_user_id_create_time_idx ON inventory_ledger(user_id, create_time DESC);

-- +migrate Down
DROP INDEX IF EXISTS inventory_ledger_user_id_create_time_idx;
DROP TABLE IF EXISTS inventory_ledger;
ALTER TABLE users DROP COLUMN IF EXISTS inventory;

