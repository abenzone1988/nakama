package main

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

func main() {
	ctx := context.Background()
	userID := "b811ba0a-ac49-4e98-961c-9bd883cec5b8"

	fmt.Printf("=== 检查所有Redis数据库中的用户键 ===\n")
	fmt.Printf("用户ID: %s\n\n", userID)

	// 检查DB 0-15 (Redis默认16个数据库)
	for dbNum := 0; dbNum < 16; dbNum++ {
		client := redis.NewClient(&redis.Options{
			Addr:     "192.168.102.223:6379",
			Password: "",
			DB:       dbNum,
		})

		// 测试连接
		if err := client.Ping(ctx).Err(); err != nil {
			client.Close()
			continue
		}

		// 查找用户相关的键
		pattern := fmt.Sprintf("user:%s:*", userID)
		keys, err := client.Keys(ctx, pattern).Result()

		if err != nil {
			fmt.Printf("DB %d: 查询失败 - %v\n", dbNum, err)
			client.Close()
			continue
		}

		if len(keys) > 0 {
			fmt.Printf("🔍 DB %d: 找到 %d 个键\n", dbNum, len(keys))
			for _, key := range keys {
				value, err := client.Get(ctx, key).Result()
				if err != nil {
					value = fmt.Sprintf("<获取失败: %v>", err)
				}
				ttl, _ := client.TTL(ctx, key).Result()
				fmt.Printf("   - %s = %s (TTL: %v)\n", key, value, ttl)
			}
			fmt.Println()
		} else {
			fmt.Printf("DB %d: 无相关键\n", dbNum)
		}

		client.Close()
	}

	// 现在尝试删除特定键
	fmt.Println("=== 尝试从所有数据库删除指定键 ===")
	targetKeys := []string{
		fmt.Sprintf("user:%s:session", userID),
		fmt.Sprintf("user:%s:refresh", userID),
	}

	for dbNum := 0; dbNum < 16; dbNum++ {
		client := redis.NewClient(&redis.Options{
			Addr:     "192.168.102.223:6379",
			Password: "",
			DB:       dbNum,
		})

		if err := client.Ping(ctx).Err(); err != nil {
			client.Close()
			continue
		}

		for _, key := range targetKeys {
			exists, _ := client.Exists(ctx, key).Result()
			if exists > 0 {
				deleted, err := client.Del(ctx, key).Result()
				if err != nil {
					fmt.Printf("DB %d: 删除 %s 失败 - %v\n", dbNum, key, err)
				} else if deleted > 0 {
					fmt.Printf("DB %d: ✓ 成功删除 %s\n", dbNum, key)
				}
			}
		}

		client.Close()
	}
}
