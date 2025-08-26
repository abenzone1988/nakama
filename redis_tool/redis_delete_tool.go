package main

import (
	"context"
	"fmt"
	"log"
	"os"

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

	if len(os.Args) < 2 {
		fmt.Println("使用方法:")
		fmt.Println("  go run redis_delete_tool.go list [pattern]     - 列出匹配的键")
		fmt.Println("  go run redis_delete_tool.go del <key>          - 删除指定键")
		fmt.Println("  go run redis_delete_tool.go delpattern <pattern> - 删除匹配模式的所有键")
		fmt.Println("  go run redis_delete_tool.go flush              - 清空当前数据库")
		fmt.Println("")
		fmt.Println("示例:")
		fmt.Println("  go run redis_delete_tool.go list user:*")
		fmt.Println("  go run redis_delete_tool.go del user:123:session")
		fmt.Println("  go run redis_delete_tool.go delpattern user:b811ba0a*")
		return
	}

	command := os.Args[1]

	switch command {
	case "list":
		pattern := "*"
		if len(os.Args) > 2 {
			pattern = os.Args[2]
		}
		listKeys(client, ctx, pattern)

	case "del":
		if len(os.Args) < 3 {
			fmt.Println("请指定要删除的键名")
			return
		}
		key := os.Args[2]
		deleteKey(client, ctx, key)

	case "delpattern":
		if len(os.Args) < 3 {
			fmt.Println("请指定要删除的键模式")
			return
		}
		pattern := os.Args[2]
		deletePattern(client, ctx, pattern)

	case "flush":
		flushDB(client, ctx)

	default:
		fmt.Printf("未知命令: %s\n", command)
	}
}

func listKeys(client *redis.Client, ctx context.Context, pattern string) {
	fmt.Printf("查找匹配模式 '%s' 的键...\n", pattern)

	keys, err := client.Keys(ctx, pattern).Result()
	if err != nil {
		fmt.Printf("获取键列表失败: %v\n", err)
		return
	}

	if len(keys) == 0 {
		fmt.Println("没有找到匹配的键")
		return
	}

	fmt.Printf("找到 %d 个键:\n", len(keys))
	for i, key := range keys {
		// 获取键的类型和TTL
		keyType, _ := client.Type(ctx, key).Result()
		ttl, _ := client.TTL(ctx, key).Result()

		fmt.Printf("%d. %s (类型: %s, TTL: %v)\n", i+1, key, keyType, ttl)

		// 如果是string类型，显示值的前50个字符
		if keyType == "string" {
			value, err := client.Get(ctx, key).Result()
			if err == nil {
				if len(value) > 50 {
					fmt.Printf("   值: %s...\n", value[:50])
				} else {
					fmt.Printf("   值: %s\n", value)
				}
			}
		}
	}
}

func deleteKey(client *redis.Client, ctx context.Context, key string) {
	fmt.Printf("准备删除键: %s\n", key)

	// 检查键是否存在
	exists, err := client.Exists(ctx, key).Result()
	if err != nil {
		fmt.Printf("检查键存在性失败: %v\n", err)
		return
	}

	if exists == 0 {
		fmt.Println("键不存在")
		return
	}

	// 删除键
	deleted, err := client.Del(ctx, key).Result()
	if err != nil {
		fmt.Printf("删除失败: %v\n", err)
		return
	}

	if deleted > 0 {
		fmt.Printf("✓ 成功删除键: %s\n", key)
	} else {
		fmt.Printf("✗ 删除失败，键可能不存在: %s\n", key)
	}
}

func deletePattern(client *redis.Client, ctx context.Context, pattern string) {
	fmt.Printf("查找并删除匹配模式 '%s' 的所有键...\n", pattern)

	keys, err := client.Keys(ctx, pattern).Result()
	if err != nil {
		fmt.Printf("获取键列表失败: %v\n", err)
		return
	}

	if len(keys) == 0 {
		fmt.Println("没有找到匹配的键")
		return
	}

	fmt.Printf("找到 %d 个键，准备删除...\n", len(keys))

	// 批量删除
	deleted, err := client.Del(ctx, keys...).Result()
	if err != nil {
		fmt.Printf("批量删除失败: %v\n", err)
		return
	}

	fmt.Printf("✓ 成功删除 %d 个键\n", deleted)
	for _, key := range keys {
		fmt.Printf("  - %s\n", key)
	}
}

func flushDB(client *redis.Client, ctx context.Context) {
	fmt.Println("警告: 这将删除当前数据库的所有键!")
	fmt.Print("确认删除? (输入 'YES' 确认): ")

	var confirm string
	fmt.Scanln(&confirm)

	if confirm != "YES" {
		fmt.Println("操作已取消")
		return
	}

	err := client.FlushDB(ctx).Err()
	if err != nil {
		fmt.Printf("清空数据库失败: %v\n", err)
		return
	}

	fmt.Println("✓ 成功清空当前数据库的所有键")
}
