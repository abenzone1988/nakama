package main

import (
	"context"
	"fmt"
	"log"

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

	// 测试连接
	if err := client.Ping(ctx).Err(); err != nil {
		log.Fatalf("连接Redis失败: %v", err)
	}

	userID := "b811ba0a-ac49-4e98-961c-9bd883cec5b8"

	fmt.Printf("=== 用户 %s 的Redis数据 ===\n", userID)

	// 检查session键
	sessionKey := fmt.Sprintf("user:%s:session", userID)
	sessionValue, err := client.Get(ctx, sessionKey).Result()
	if err != nil {
		fmt.Printf("Session键: %s - 不存在或错误: %v\n", sessionKey, err)
	} else {
		fmt.Printf("Session键: %s\n", sessionKey)
		fmt.Printf("Session值: %s\n", sessionValue)

		// 获取TTL
		ttl, _ := client.TTL(ctx, sessionKey).Result()
		fmt.Printf("Session TTL: %v\n", ttl)
	}

	fmt.Println()

	// 检查refresh键
	refreshKey := fmt.Sprintf("user:%s:refresh", userID)
	refreshValue, err := client.Get(ctx, refreshKey).Result()
	if err != nil {
		fmt.Printf("Refresh键: %s - 不存在或错误: %v\n", refreshKey, err)
	} else {
		fmt.Printf("Refresh键: %s\n", refreshKey)
		fmt.Printf("Refresh值: %s\n", refreshValue)

		// 获取TTL
		ttl, _ := client.TTL(ctx, refreshKey).Result()
		fmt.Printf("Refresh TTL: %v\n", ttl)
	}

	fmt.Println()

	// 检查ban键（如果存在）
	banKey := fmt.Sprintf("user:%s:ban", userID)
	banValue, err := client.Get(ctx, banKey).Result()
	if err != nil {
		fmt.Printf("Ban键: %s - 不存在\n", banKey)
	} else {
		fmt.Printf("Ban键: %s\n", banKey)
		fmt.Printf("Ban值: %s\n", banValue)
	}

	fmt.Println()

	// 查看所有相关键
	fmt.Println("=== 所有用户相关键 ===")
	keys, err := client.Keys(ctx, fmt.Sprintf("user:%s:*", userID)).Result()
	if err != nil {
		fmt.Printf("获取键列表失败: %v\n", err)
	} else {
		for _, key := range keys {
			fmt.Printf("键: %s\n", key)
		}
	}
}
