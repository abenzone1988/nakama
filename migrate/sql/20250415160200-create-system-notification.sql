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

-- +migrate Down
DROP TABLE IF EXISTS system_notification;


