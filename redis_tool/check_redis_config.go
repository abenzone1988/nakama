package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

func main() {
	// 连接到Redis
	client := redis.NewClient(&redis.Options{
		Addr:     "192.168.102.223:6379",
		Password: "", // 空密码
		DB:       0,
	})

	ctx := context.Background()

	// 测试连接
	pong, err := client.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("无法连接到Redis: %v", err)
	}
	fmt.Printf("Redis连接成功: %s\n", pong)

	// 查看Redis信息
	info, err := client.Info(ctx).Result()
	if err != nil {
		log.Printf("获取Redis信息失败: %v", err)
	} else {
		fmt.Println("\n=== Redis信息 ===")
		fmt.Println(info)
	}

	// 查看具体配置
	configs := []string{
		"maxclients",       // 最大客户端连接数
		"timeout",          // 客户端空闲超时
		"tcp-keepalive",    // TCP keepalive
		"maxmemory",        // 最大内存
		"maxmemory-policy", // 内存策略
		"save",             // 持久化配置
	}

	fmt.Println("\n=== 关键配置参数 ===")
	for _, config := range configs {
		result, err := client.ConfigGet(ctx, config).Result()
		if err != nil {
			fmt.Printf("%s: 获取失败 - %v\n", config, err)
		} else {
			fmt.Printf("%s: %v\n", config, result)
		}
	}

	// 查看当前连接数
	result, err := client.Info(ctx, "clients").Result()
	if err != nil {
		log.Printf("获取客户端信息失败: %v", err)
	} else {
		fmt.Println("\n=== 客户端连接信息 ===")
		fmt.Println(result)
	}

	// 测试多个并发连接
	fmt.Println("\n=== 测试并发连接 ===")
	for i := 0; i < 5; i++ {
		go func(id int) {
			testClient := redis.NewClient(&redis.Options{
				Addr:     "192.168.102.223:6379",
				Password: "",
				DB:       0,
			})
			defer testClient.Close()

			pong, err := testClient.Ping(ctx).Result()
			if err != nil {
				fmt.Printf("连接 %d 失败: %v\n", id, err)
			} else {
				fmt.Printf("连接 %d 成功: %s\n", id, pong)
			}
		}(i)
	}

	// 等待goroutine完成
	time.Sleep(2 * time.Second)

	client.Close()
}
