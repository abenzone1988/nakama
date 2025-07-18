-- +migrate Up
-- 修改 system_notification 表，确保 effective_time 不为空
ALTER TABLE system_notification
    ALTER COLUMN effective_time SET NOT NULL;

-- 为现有的空 effective_time 记录设置默认值（使用 create_time）
UPDATE system_notification
SET effective_time = create_time
WHERE effective_time IS NULL;

-- 添加 challenge_id 字段，用于存储挑战赛ID
ALTER TABLE system_notification
    ADD COLUMN challenge_id INTEGER;

-- 添加 notice_type 字段，用于存储通知类型
ALTER TABLE system_notification
    ADD COLUMN notice_type INTEGER DEFAULT 0;

-- 添加索引优化按 effective_time 排序的查询
CREATE INDEX IF NOT EXISTS idx_system_notification_effective_time_sort
ON system_notification(effective_time DESC);

-- 添加挑战赛ID索引，用于快速查找特定挑战赛的通知
CREATE INDEX IF NOT EXISTS idx_system_notification_challenge_id
ON system_notification(challenge_id);

-- +migrate Down
-- 回滚修改
DROP INDEX IF EXISTS idx_system_notification_challenge_id;

DROP INDEX IF EXISTS idx_system_notification_effective_time_sort;

-- 删除 challenge_id 字段
ALTER TABLE system_notification
    DROP COLUMN IF EXISTS challenge_id;

-- 删除 type 字段
ALTER TABLE system_notification
    DROP COLUMN IF EXISTS notice_type;

ALTER TABLE system_notification
    ALTER COLUMN effective_time DROP NOT NULL;

