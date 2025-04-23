-- +migrate Up
CREATE TABLE IF NOT EXISTS system_notification (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    subject VARCHAR(255) NOT NULL,
    content JSONB NOT NULL DEFAULT '{}',
    code    SMALLINT NOT NULL DEFAULT 0, -- Notification code
    create_time TIMESTAMPTZ DEFAULT current_timestamp,
    effective_time TIMESTAMPTZ,
    expiry_time TIMESTAMPTZ
);

-- 添加索引以优化时间相关的查询
CREATE INDEX idx_system_notification_create_time ON system_notification(create_time);
CREATE INDEX idx_system_notification_effective_time ON system_notification(effective_time);
CREATE INDEX idx_system_notification_expiry_time ON system_notification(expiry_time);

-- 修改 notification 表添加新字段
ALTER TABLE notification 
    ADD COLUMN IF NOT EXISTS expiry_time TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS status SMALLINT NOT NULL DEFAULT 0;

-- 添加 notification 表的状态索引，用于查询未读和未领取的通知
CREATE INDEX idx_notification_status ON notification(status);

-- 添加组合索引用于按状态和过期时间查询
CREATE INDEX idx_notification_status_expiry ON notification(status, expiry_time);

-- +migrate Down
DROP TABLE IF EXISTS system_notification;

-- 回滚 notification 表的修改
ALTER TABLE notification 
    DROP COLUMN IF EXISTS expiry_time,
    DROP COLUMN IF EXISTS status;


