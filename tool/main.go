package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	appid     = "wxc2b1cadd88c8973b"
	appsecret = "46cb21c728931e6b1085d25ffd8bf9cc"
	apiURL    = "https://api.weixin.qq.com/cgi-bin/token"
)

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	ErrCode     int    `json:"errcode,omitempty"`
	ErrMsg      string `json:"errmsg,omitempty"`
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	params := url.Values{}
	params.Set("grant_type", "client_credential")
	params.Set("appid", appid)
	params.Set("secret", appsecret)

	requestURL := apiURL + "?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		fmt.Printf("创建请求失败: %v\n", err)
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("请求失败: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("读取响应失败: %v\n", err)
		return
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		fmt.Printf("解析响应失败: %v\n", err)
		fmt.Printf("原始响应: %s\n", string(body))
		return
	}

	if tokenResp.ErrCode != 0 {
		fmt.Printf("错误: %d - %s\n", tokenResp.ErrCode, tokenResp.ErrMsg)
		return
	}

	fmt.Printf("Access Token: %s\n", tokenResp.AccessToken)
	fmt.Printf("过期时间: %d 秒\n", tokenResp.ExpiresIn)
}
