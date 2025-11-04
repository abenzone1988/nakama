-- +migrate Up

-- 主要查询索引：优化简化后的TournamentFindFirstAvailable查询
-- 查询模式：WHERE duration > 0 AND category = ? AND (max_size = 0 OR size < max_size) ORDER BY create_time
-- 索引列顺序：category(高选择性) -> duration(过滤条件) -> create_time(排序) -> size, max_size(可加入判断)
CREATE INDEX IF NOT EXISTS idx_leaderboard_tournament_available 
ON leaderboard (category, duration, create_time, size, max_size)
WHERE duration > 0;

-- 针对有maxSize限制的查询优化
-- 当maxSize > 0时，size < maxSize条件会更有选择性
CREATE INDEX IF NOT EXISTS idx_leaderboard_tournament_size_limited
ON leaderboard (category, duration, size, create_time, max_size)
WHERE duration > 0 AND max_size > 0;

-- +migrate Down

-- 删除索引（按创建的逆序）
DROP INDEX IF EXISTS idx_leaderboard_tournament_size_limited;
DROP INDEX IF EXISTS idx_leaderboard_tournament_available; 