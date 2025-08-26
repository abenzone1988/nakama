package main

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

func main() {
	ctx := context.Background()

	fmt.Printf("=== Redis查询状态演示 ===\n\n")

	// 1. 正常连接的Redis客户端
	normalClient := redis.NewClient(&redis.Options{
		Addr:     "192.168.102.223:6379",
		Password: "",
		DB:       0,
	})
	defer normalClient.Close()

	// 2. 连接错误地址的客户端（模拟连接失败）
	errorClient := redis.NewClient(&redis.Options{
		Addr:        "192.168.102.223:9999", // 错误端口
		Password:    "",
		DB:          0,
		DialTimeout: 1 * time.Second, // 快速超时
	})
	defer errorClient.Close()

	testKey := "test:demo:key"

	// 演示1: 查询不存在的键（正常连接）
	fmt.Println("--- 演示1: 查询不存在的键 ---")
	value1, err1 := normalClient.Get(ctx, testKey).Result()
	analyzeResult("不存在的键", value1, err1)

	// 演示2: 设置键并查询（正常连接）
	fmt.Println("\n--- 演示2: 设置键后查询 ---")
	normalClient.Set(ctx, testKey, "hello world", 10*time.Second)
	value2, err2 := normalClient.Get(ctx, testKey).Result()
	analyzeResult("存在的键", value2, err2)

	// 演示3: 删除键后查询（正常连接）
	fmt.Println("\n--- 演示3: 删除键后查询 ---")
	normalClient.Del(ctx, testKey)
	value3, err3 := normalClient.Get(ctx, testKey).Result()
	analyzeResult("删除后的键", value3, err3)

	// 演示4: 连接失败的情况
	fmt.Println("\n--- 演示4: 连接失败的情况 ---")
	value4, err4 := errorClient.Get(ctx, testKey).Result()
	analyzeResult("连接失败", value4, err4)

	// 演示5: 使用Keys命令的不同情况
	fmt.Println("\n--- 演示5: Keys命令的不同情况 ---")

	// 5.1 查询存在的键模式
	normalClient.Set(ctx, "user:123:session", "token123", 10*time.Second)
	normalClient.Set(ctx, "user:123:refresh", "refresh123", 10*time.Second)

	keys1, err5 := normalClient.Keys(ctx, "user:123:*").Result()
	analyzeKeysResult("存在的键模式", keys1, err5)

	// 5.2 查询不存在的键模式
	keys2, err6 := normalClient.Keys(ctx, "nonexistent:*").Result()
	analyzeKeysResult("不存在的键模式", keys2, err6)

	// 5.3 连接失败时的Keys查询
	keys3, err7 := errorClient.Keys(ctx, "user:*").Result()
	analyzeKeysResult("连接失败时的Keys", keys3, err7)

	// 清理
	normalClient.Del(ctx, "user:123:session", "user:123:refresh")
}

func analyzeResult(scenario string, value string, err error) {
	fmt.Printf("场景: %s\n", scenario)
	fmt.Printf("返回值: \"%s\"\n", value)

	if err == nil {
		fmt.Printf("错误: nil\n")
		fmt.Printf("结果: ✅ 查询成功，键存在\n")
		fmt.Printf("判断: err == nil\n")
	} else if err == redis.Nil {
		fmt.Printf("错误: %v\n", err)
		fmt.Printf("结果: 🔍 键不存在（正常情况）\n")
		fmt.Printf("判断: err == redis.Nil\n")
	} else {
		fmt.Printf("错误: %v\n", err)
		fmt.Printf("结果: ❌ 查询失败（连接/服务器问题）\n")
		fmt.Printf("判断: err != nil && err != redis.Nil\n")
	}
	fmt.Println()
}

func analyzeKeysResult(scenario string, keys []string, err error) {
	fmt.Printf("Keys场景: %s\n", scenario)
	fmt.Printf("返回键数量: %d\n", len(keys))
	if len(keys) > 0 {
		fmt.Printf("键列表: %v\n", keys)
	}

	if err == nil {
		if len(keys) > 0 {
			fmt.Printf("结果: ✅ 查询成功，找到匹配的键\n")
		} else {
			fmt.Printf("结果: 🔍 查询成功，但没有匹配的键（空结果）\n")
		}
		fmt.Printf("判断: err == nil\n")
	} else {
		fmt.Printf("错误: %v\n", err)
		fmt.Printf("结果: ❌ 查询失败（连接/服务器问题）\n")
		fmt.Printf("判断: err != nil\n")
	}
	fmt.Println()
}
