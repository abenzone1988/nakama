-- +migrate Up
CREATE TABLE IF NOT EXISTS personal_notification_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    subject VARCHAR(255) NOT NULL,
    content JSONB NOT NULL DEFAULT '{}',
    target_ids TEXT[] NOT NULL, -- 目标用户id数组，用于显示
    sender VARCHAR(255) NOT NULL, -- 发送者用户名
    send_time TIMESTAMPTZ DEFAULT current_timestamp,
    notification_count INTEGER NOT NULL DEFAULT 0 -- 实际发送的通知数量
);

-- 添加索引以优化查询
CREATE INDEX idx_personal_notification_log_send_time ON personal_notification_log(send_time);
CREATE INDEX idx_personal_notification_log_sender_id ON personal_notification_log(sender);
CREATE INDEX idx_personal_notification_log_target_user_ids ON personal_notification_log USING GIN(target_ids);

-- +migrate Down
DROP TABLE IF EXISTS personal_notification_log;
