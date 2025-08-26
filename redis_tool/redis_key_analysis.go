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

	fmt.Printf("=== Redis键值分析 ===\n")
	fmt.Printf("用户ID: %s\n\n", userID)

	// 1. 查找所有相关键
	pattern := fmt.Sprintf("user:%s:*", userID)
	keys, err := client.Keys(ctx, pattern).Result()
	if err != nil {
		fmt.Printf("获取键列表失败: %v\n", err)
		return
	}

	fmt.Printf("找到 %d 个相关键:\n", len(keys))

	for i, key := range keys {
		fmt.Printf("\n--- 键 %d ---\n", i+1)
		fmt.Printf("键名(Key): %s\n", key)

		// 获取键的类型
		keyType, _ := client.Type(ctx, key).Result()
		fmt.Printf("数据类型: %s\n", keyType)

		// 获取值
		if keyType == "string" {
			value, err := client.Get(ctx, key).Result()
			if err != nil {
				fmt.Printf("获取值失败: %v\n", err)
			} else {
				fmt.Printf("值(String): %s\n", value)
			}
		}

		// 获取TTL
		ttl, _ := client.TTL(ctx, key).Result()
		if ttl == -1 {
			fmt.Printf("过期时间: 永不过期\n")
		} else if ttl == -2 {
			fmt.Printf("过期时间: 键不存在\n")
		} else {
			fmt.Printf("过期时间: %v\n", ttl)
		}

		// 获取内存使用
		memory, _ := client.MemoryUsage(ctx, key).Result()
		fmt.Printf("内存使用: %d bytes\n", memory)
	}

	// 2. 测试删除操作
	fmt.Printf("\n=== 测试删除操作 ===\n")

	if len(keys) > 0 {
		testKey := keys[0]
		fmt.Printf("准备删除键: %s\n", testKey)

		// 删除前检查
		exists1, _ := client.Exists(ctx, testKey).Result()
		fmt.Printf("删除前键存在: %v\n", exists1 > 0)

		// 执行删除
		deleted, err := client.Del(ctx, testKey).Result()
		if err != nil {
			fmt.Printf("删除失败: %v\n", err)
		} else {
			fmt.Printf("删除成功，删除了 %d 个键\n", deleted)
		}

		// 删除后检查
		exists2, _ := client.Exists(ctx, testKey).Result()
		fmt.Printf("删除后键存在: %v\n", exists2 > 0)

		// 等待一秒后再次检查（防止延迟）
		time.Sleep(1 * time.Second)
		exists3, _ := client.Exists(ctx, testKey).Result()
		fmt.Printf("1秒后键存在: %v\n", exists3 > 0)
	}

	// 3. 查看Redis信息
	fmt.Printf("\n=== Redis服务器信息 ===\n")
	info, _ := client.Info(ctx, "memory").Result()
	fmt.Printf("内存信息:\n%s\n", info)

	// 4. 查看所有匹配的键（再次确认）
	fmt.Printf("\n=== 最终键列表 ===\n")
	finalKeys, _ := client.Keys(ctx, pattern).Result()
	fmt.Printf("当前存在 %d 个键:\n", len(finalKeys))
	for _, key := range finalKeys {
		fmt.Printf("- %s\n", key)
	}
}
