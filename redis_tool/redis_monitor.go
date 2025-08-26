package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

func main() {
	client := redis.NewClient(&redis.Options{
		Addr:     "192.168.102.223:6379",
		Password: "",
		DB:       0,
	})
	defer client.Close()

	ctx := context.Background()

	if err := client.Ping(ctx).Err(); err != nil {
		log.Fatalf("连接Redis失败: %v", err)
	}

	userID := "b811ba0a-ac49-4e98-961c-9bd883cec5b8"
	pattern := fmt.Sprintf("user:%s:*", userID)

	fmt.Printf("=== 实时监控用户键的变化 ===\n")
	fmt.Printf("监控模式: %s\n", pattern)
	fmt.Printf("按 Ctrl+C 停止监控\n\n")

	lastKeys := make(map[string]string)

	for {
		// 获取当前所有键
		currentKeys, err := client.Keys(ctx, pattern).Result()
		if err != nil {
			fmt.Printf("获取键失败: %v\n", err)
			time.Sleep(1 * time.Second)
			continue
		}

		// 构建当前键值映射
		currentMap := make(map[string]string)
		for _, key := range currentKeys {
			value, err := client.Get(ctx, key).Result()
			if err != nil {
				value = "<获取失败>"
			}
			currentMap[key] = value
		}

		// 检查变化
		hasChanges := false

		// 检查新增的键
		for key, value := range currentMap {
			if lastValue, exists := lastKeys[key]; !exists {
				fmt.Printf("[%s] 🆕 新增键: %s = %s\n",
					time.Now().Format("15:04:05"), key, value)
				hasChanges = true
			} else if lastValue != value {
				fmt.Printf("[%s] 🔄 键值变化: %s\n",
					time.Now().Format("15:04:05"), key)
				fmt.Printf("    旧值: %s\n", lastValue)
				fmt.Printf("    新值: %s\n", value)
				hasChanges = true
			}
		}

		// 检查删除的键
		for key, value := range lastKeys {
			if _, exists := currentMap[key]; !exists {
				fmt.Printf("[%s] 🗑️  删除键: %s (旧值: %s)\n",
					time.Now().Format("15:04:05"), key, value)
				hasChanges = true
			}
		}

		// 如果没有变化，显示当前状态
		if !hasChanges && len(currentMap) > 0 {
			fmt.Printf("[%s] 📊 当前状态: %d个键存在\n",
				time.Now().Format("15:04:05"), len(currentMap))
			for key, value := range currentMap {
				ttl, _ := client.TTL(ctx, key).Result()
				fmt.Printf("    %s = %s (TTL: %v)\n", key, value, ttl)
			}
		} else if len(currentMap) == 0 && len(lastKeys) == 0 {
			fmt.Printf("[%s] 💤 无相关键存在\n", time.Now().Format("15:04:05"))
		}

		// 更新上次状态
		lastKeys = currentMap

		// 等待1秒
		time.Sleep(1 * time.Second)
	}
}
