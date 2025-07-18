-- +migrate Up
-- 修改 system_notification 表，确保 effective_time 不为空
ALTER TABLE system_notification 
    ALTER COLUMN effective_time SET NOT NULL;

-- 为现有的空 effective_time 记录设置默认值（使用 create_time）
UPDATE system_notification 
SET effective_time = create_time 
WHERE effective_time IS NULL;

-- 添加约束确保 effective_time 不能为空
ALTER TABLE system_notification 
    ADD CONSTRAINT system_notification_effective_time_not_null 
    CHECK (effective_time IS NOT NULL);

-- 添加索引优化按 effective_time 排序的查询
CREATE INDEX IF NOT EXISTS idx_system_notification_effective_time_sort 
ON system_notification(effective_time DESC);

-- 添加约束确保 effective_time 不能小于当前时间（对于新插入的记录）
-- 注意：这个约束只对新记录生效，现有记录不受影响
ALTER TABLE system_notification 
    ADD CONSTRAINT system_notification_effective_time_future 
    CHECK (effective_time >= CURRENT_TIMESTAMP);

-- +migrate Down
-- 回滚修改
ALTER TABLE system_notification 
    DROP CONSTRAINT IF EXISTS system_notification_effective_time_future;

DROP INDEX IF EXISTS idx_system_notification_effective_time_sort;

ALTER TABLE system_notification 
    DROP CONSTRAINT IF EXISTS system_notification_effective_time_not_null;

ALTER TABLE system_notification 
    ALTER COLUMN effective_time DROP NOT NULL; 