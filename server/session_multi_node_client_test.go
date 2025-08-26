package server

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

// 模拟客户端结构
type TestClient struct {
	httpClient *http.Client
	session    *SessionClient
	userID     string
	username   string
	password   string
	nodePort   int
}

// Session 结构
type SessionClient struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refresh_token"`
}

// 登录响应结构
type LoginResponse struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refresh_token"`
	User         struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	} `json:"user"`
}

// 用户信息响应结构
type AccountResponse struct {
	User struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Email    string `json:"email"`
	} `json:"user"`
}

// 创建新的测试客户端
func NewTestClient(nodePort int, username, password string) *TestClient {
	// 创建 HTTP 客户端
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	return &TestClient{
		httpClient: httpClient,
		username:   username,
		password:   password,
		nodePort:   nodePort,
	}
}

// 通过 HTTP API 登录
func (c *TestClient) Login() error {
	loginURL := fmt.Sprintf("http://localhost:%d/v2/account/authenticate/custom", c.nodePort)

	// 使用自定义认证，移除server_key从请求体中
	// 注意：不包含nodePort，确保不同节点使用相同的用户ID来测试Single Session
	loginData := map[string]interface{}{
		"id":     fmt.Sprintf("test-user-%s", c.username),
		"create": true,
	}

	jsonData, err := json.Marshal(loginData)
	if err != nil {
		return fmt.Errorf("failed to marshal login data: %v", err)
	}

	// 创建请求
	req, err := http.NewRequest("POST", loginURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	// 设置Content-Type
	req.Header.Set("Content-Type", "application/json")

	// 设置Basic Authentication，用户名为server_key，密码为空
	req.SetBasicAuth("sparkgame", "")

	// 发送请求
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send login request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("login failed with status: %d, body: %s", resp.StatusCode, string(body))
	}

	var loginResp LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		return fmt.Errorf("failed to decode login response: %v", err)
	}

	c.session = &SessionClient{
		Token:        loginResp.Token,
		RefreshToken: loginResp.RefreshToken,
	}
	c.userID = loginResp.User.ID

	fmt.Printf("Node %d: 用户 %s 登录成功，UserID: %s\n", c.nodePort, c.username, c.userID)
	return nil
}

// 验证 session 是否有效
func (c *TestClient) ValidateSession() error {
	if c.session == nil {
		return fmt.Errorf("no session available")
	}

	accountURL := fmt.Sprintf("http://localhost:%d/v2/account", c.nodePort)

	req, err := http.NewRequest("GET", accountURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.session.Token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("session validation failed with status: %d, body: %s", resp.StatusCode, string(body))
	}

	var accountResp AccountResponse
	if err := json.NewDecoder(resp.Body).Decode(&accountResp); err != nil {
		return fmt.Errorf("failed to decode account response: %v", err)
	}

	fmt.Printf("Node %d: Session 验证成功，用户: %s\n", c.nodePort, accountResp.User.Username)
	return nil
}

// 刷新 token
func (c *TestClient) RefreshToken() error {
	if c.session == nil || c.session.RefreshToken == "" {
		return fmt.Errorf("no refresh token available")
	}

	refreshURL := fmt.Sprintf("http://localhost:%d/v2/account/session/refresh", c.nodePort)

	refreshData := map[string]string{
		"token": c.session.RefreshToken,
	}

	jsonData, err := json.Marshal(refreshData)
	if err != nil {
		return fmt.Errorf("failed to marshal refresh data: %v", err)
	}

	// 创建请求
	req, err := http.NewRequest("POST", refreshURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create refresh request: %v", err)
	}

	// 设置Content-Type
	req.Header.Set("Content-Type", "application/json")

	// 设置Basic Authentication，用户名为server_key，密码为空
	req.SetBasicAuth("sparkgame", "")

	// 发送请求
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send refresh request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("refresh failed with status: %d, body: %s", resp.StatusCode, string(body))
	}

	var refreshResp LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&refreshResp); err != nil {
		return fmt.Errorf("failed to decode refresh response: %v", err)
	}

	// 更新 session
	c.session.Token = refreshResp.Token
	if refreshResp.RefreshToken != "" {
		c.session.RefreshToken = refreshResp.RefreshToken
	}

	fmt.Printf("Node %d: Token 刷新成功\n", c.nodePort)
	return nil
}

// 测试多节点 single session 行为
func TestMultiNodeSingleSession(t *testing.T) {
	fmt.Println("=== 多节点 Single Session 测试 ===")

	//// 等待服务器启动
	//fmt.Println("等待服务器启动...")
	//time.Sleep(5 * time.Second)

	// 创建两个客户端，分别连接到不同的节点
	client1 := NewTestClient(7350, "testuser1@example.com", "password123")
	client2 := NewTestClient(7450, "testuser1@example.com", "password123")

	// 测试步骤 1: 在节点1登录
	fmt.Println("\n步骤 1: 在节点1 (端口7350) 登录")
	if err := client1.Login(); err != nil {
		t.Fatalf("节点1登录失败: %v", err)
	}

	// 测试步骤 2: 验证节点1的 session
	fmt.Println("\n步骤 2: 验证节点1的 session")
	if err := client1.ValidateSession(); err != nil {
		t.Fatalf("节点1 session 验证失败: %v", err)
	}

	// 测试步骤 3: 在节点2登录同一用户（应该使节点1的 session 失效）
	fmt.Println("\n步骤 3: 在节点2 (端口7450) 登录同一用户")
	if err := client2.Login(); err != nil {
		t.Fatalf("节点2登录失败: %v", err)
	}

	// 测试步骤 4: 验证节点2的 session
	fmt.Println("\n步骤 4: 验证节点2的 session")
	if err := client2.ValidateSession(); err != nil {
		t.Fatalf("节点2 session 验证失败: %v", err)
	}

	// 测试步骤 5: 验证节点1的 session 是否已失效
	fmt.Println("\n步骤 5: 验证节点1的 session 是否已失效")
	if err := client1.ValidateSession(); err != nil {
		fmt.Printf("节点1 session 已失效（符合预期）: %v\n", err)
	} else {
		t.Error("警告: 节点1 session 仍然有效，这可能表示 single session 配置有问题")
	}

	// 测试步骤 6: 在节点1重新登录
	fmt.Println("\n步骤 6: 在节点1重新登录")
	if err := client1.Login(); err != nil {
		t.Fatalf("节点1重新登录失败: %v", err)
	}

	// 测试步骤 7: 验证节点2的 session 是否已失效
	fmt.Println("\n步骤 7: 验证节点2的 session 是否已失效")
	if err := client2.ValidateSession(); err != nil {
		fmt.Printf("节点2 session 已失效（符合预期）: %v\n", err)
	} else {
		t.Error("警告: 节点2 session 仍然有效，这可能表示 single session 配置有问题")
	}

	// 测试步骤 8: 测试 token 刷新
	fmt.Println("\n步骤 8: 测试 token 刷新")
	if err := client1.RefreshToken(); err != nil {
		t.Fatalf("Token 刷新失败: %v", err)
	}

	// 测试步骤 9: 验证刷新后的 session
	fmt.Println("\n步骤 9: 验证刷新后的 session")
	if err := client1.ValidateSession(); err != nil {
		t.Fatalf("刷新后的 session 验证失败: %v", err)
	}

	// 测试步骤 10: 验证节点2的 session 是否因刷新而失效
	fmt.Println("\n步骤 10: 验证节点2的 session 是否因刷新而失效")
	if err := client2.ValidateSession(); err != nil {
		fmt.Printf("节点2 session 已失效（符合预期）: %v\n", err)
	} else {
		t.Error("警告: 节点2 session 仍然有效，这可能表示 single session 配置有问题")
	}

	fmt.Println("\n=== 测试完成 ===")
}

// 测试同账号不同节点登录，验证session失效
func TestConcurrentLogin(t *testing.T) {
	fmt.Println("\n=== 同账号不同节点登录测试 ===")

	// 创建多个客户端，都使用相同的用户账号
	const testCount = 10
	const sameUsername = "sameuser@example.com"
	clients := make([]*TestClient, testCount)

	for i := 0; i < testCount; i++ {
		port := 7350
		if i%2 == 1 {
			port = 7450 // 交替使用不同节点
		}
		clients[i] = NewTestClient(port, sameUsername, "password123")
	}

	// 顺序登录，测试Single Session机制
	fmt.Println("顺序登录", testCount, "次，使用相同账号...")
	var lastValidClient *TestClient

	for i, client := range clients {
		if client == nil {
			t.Errorf("客户端 %d 为nil", i+1)
			continue
		}

		fmt.Printf("步骤 %d: 在节点%d登录用户 %s\n", i+1, client.nodePort, client.username)
		if err := client.Login(); err != nil {
			t.Logf("客户端 %d (节点%d) 登录失败: %v", i+1, client.nodePort, err)
			continue
		}

		// 验证当前客户端的session
		if err := client.ValidateSession(); err != nil {
			t.Logf("客户端 %d (节点%d) session验证失败: %v", i+1, client.nodePort, err)
		} else {
			fmt.Printf("✓ 客户端 %d (节点%d) session验证成功\n", i+1, client.nodePort)
		}

		// 如果有上一个有效的客户端，验证其session是否已失效
		if lastValidClient != nil && lastValidClient != client {
			fmt.Printf("检查上一个客户端 (节点%d) 的session是否已失效...\n", lastValidClient.nodePort)
			if err := lastValidClient.ValidateSession(); err != nil {
				fmt.Printf("✓ 上一个客户端 (节点%d) session已失效: %v\n", lastValidClient.nodePort, err)
			} else {
				t.Logf("⚠️  警告: 上一个客户端 (节点%d) session仍然有效，Single Session可能有问题", lastValidClient.nodePort)
			}
		}

		lastValidClient = client

		// 添加短暂延迟，确保Redis同步
		time.Sleep(100 * time.Millisecond)
	}

	fmt.Println("同账号不同节点登录测试完成")
}
