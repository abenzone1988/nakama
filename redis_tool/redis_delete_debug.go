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

	// 目标键
	targetKey := "user:b811ba0a-ac49-4e98-961c-9bd883cec5b8:refresh"

	fmt.Printf("=== Redis删除调试 ===\n")
	fmt.Printf("目标键: %s\n\n", targetKey)

	// 1. 检查键的详细信息
	fmt.Println("--- 步骤1: 检查键的详细信息 ---")

	exists, err := client.Exists(ctx, targetKey).Result()
	if err != nil {
		fmt.Printf("检查键存在性失败: %v\n", err)
		return
	}

	fmt.Printf("键存在: %v\n", exists > 0)

	if exists > 0 {
		// 获取键的详细信息
		keyType, _ := client.Type(ctx, targetKey).Result()
		fmt.Printf("键类型: %s\n", keyType)

		ttl, _ := client.TTL(ctx, targetKey).Result()
		fmt.Printf("TTL: %v\n", ttl)

		if keyType == "string" {
			value, err := client.Get(ctx, targetKey).Result()
			if err != nil {
				fmt.Printf("获取值失败: %v\n", err)
			} else {
				fmt.Printf("当前值: %s\n", value)
			}
		}

		memory, _ := client.MemoryUsage(ctx, targetKey).Result()
		fmt.Printf("内存使用: %d bytes\n", memory)
	}

	// 2. 尝试删除并监控
	fmt.Println("\n--- 步骤2: 执行删除操作 ---")

	if exists > 0 {
		// 记录删除前的值
		oldValue, _ := client.Get(ctx, targetKey).Result()
		fmt.Printf("删除前的值: %s\n", oldValue)

		// 执行删除
		fmt.Printf("执行 DEL %s ...\n", targetKey)
		deleted, err := client.Del(ctx, targetKey).Result()
		if err != nil {
			fmt.Printf("删除失败: %v\n", err)
			return
		}

		fmt.Printf("DEL命令返回: %d (删除的键数量)\n", deleted)

		// 立即检查
		exists1, _ := client.Exists(ctx, targetKey).Result()
		fmt.Printf("删除后立即检查: 键存在 = %v\n", exists1 > 0)

		// 等待1秒后再次检查
		fmt.Println("等待1秒...")
		time.Sleep(1 * time.Second)

		exists2, _ := client.Exists(ctx, targetKey).Result()
		fmt.Printf("1秒后检查: 键存在 = %v\n", exists2 > 0)

		if exists2 > 0 {
			newValue, _ := client.Get(ctx, targetKey).Result()
			fmt.Printf("新的值: %s\n", newValue)

			if newValue != oldValue {
				fmt.Println("⚠️  警告: 键被重新创建了！值已改变")
			} else {
				fmt.Println("⚠️  警告: 键仍然存在，值未改变")
			}
		} else {
			fmt.Println("✓ 键已成功删除")
		}

		// 继续监控5秒，看是否会重新出现
		fmt.Println("\n--- 步骤3: 监控键是否被重新创建 ---")
		for i := 1; i <= 5; i++ {
			time.Sleep(1 * time.Second)
			exists3, _ := client.Exists(ctx, targetKey).Result()
			if exists3 > 0 {
				value3, _ := client.Get(ctx, targetKey).Result()
				fmt.Printf("第%d秒: 键重新出现! 值: %s\n", i, value3)
				break
			} else {
				fmt.Printf("第%d秒: 键仍不存在\n", i)
			}
		}
	} else {
		fmt.Println("键不存在，无需删除")
	}

	// 4. 检查所有相关键
	fmt.Println("\n--- 步骤4: 检查所有相关键 ---")
	userID := "b811ba0a-ac49-4e98-961c-9bd883cec5b8"
	pattern := fmt.Sprintf("user:%s:*", userID)

	allKeys, err := client.Keys(ctx, pattern).Result()
	if err != nil {
		fmt.Printf("获取键列表失败: %v\n", err)
		return
	}

	fmt.Printf("用户相关的所有键 (%d个):\n", len(allKeys))
	for _, key := range allKeys {
		value, _ := client.Get(ctx, key).Result()
		ttl, _ := client.TTL(ctx, key).Result()
		fmt.Printf("- %s = %s (TTL: %v)\n", key, value, ttl)
	}

	// 5. 查看Redis日志信息
	fmt.Println("\n--- 步骤5: Redis服务器信息 ---")
	info, _ := client.Info(ctx, "stats").Result()
	fmt.Printf("统计信息:\n%s\n", info)
}
