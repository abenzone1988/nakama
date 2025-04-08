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
CREATE TABLE IF NOT EXISTS announcement (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title VARCHAR(255) NOT NULL,
    content TEXT NOT NULL,
    img VARCHAR(1024),
    status SMALLINT NOT NULL DEFAULT 0,
    create_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    update_time TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS announcement_status_create_time_idx ON announcement(status, create_time DESC);
CREATE INDEX IF NOT EXISTS announcement_create_time_idx ON announcement(create_time DESC);

-- +migrate Down
DROP TABLE IF EXISTS announcement;
