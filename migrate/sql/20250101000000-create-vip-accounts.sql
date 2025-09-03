-- +migrate Up
CREATE TABLE IF NOT EXISTS vip_accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    username VARCHAR(128) NOT NULL,
    create_time TIMESTAMPTZ NOT NULL DEFAULT now(),
    expire_time TIMESTAMPTZ NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    UNIQUE (user_id)
);

-- 添加索引以优化查询
CREATE INDEX IF NOT EXISTS idx_vip_accounts_user_id ON vip_accounts(user_id);
CREATE INDEX IF NOT EXISTS idx_vip_accounts_expire_time ON vip_accounts(expire_time);
CREATE INDEX IF NOT EXISTS idx_vip_accounts_username ON vip_accounts(username);

-- +migrate Down
DROP TABLE IF EXISTS vip_accounts;
