package server

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	credential "github.com/bytedance/douyin-openapi-credential-go/client"
	openApiSdkClient "github.com/bytedance/douyin-openapi-sdk-go/client"
	"github.com/heroiclabs/nakama/v3/game"
	"go.uber.org/zap"

	"github.com/gofrs/uuid/v5"
	"github.com/heroiclabs/nakama-common/api"
)

type WeChatResponse struct {
	OpenID     string `json:"openid"`
	SessionKey string `json:"session_key"`
	UnionID    string `json:"unionid,omitempty"`
	ErrCode    int    `json:"errcode,omitempty"`
	ErrMsg     string `json:"errmsg,omitempty"`
}

// getWeChatOpenID 获取微信用户的 OpenID，改为使用带超时和日志记录的方式
func getWeChatOpenID(ctx context.Context, logger *zap.Logger, c Config, code string) (string, error) {
	// 微信获取 OpenID 的 API 地址
	apiURL := "https://api.weixin.qq.com/sns/jscode2session"
	grantType := "authorization_code"

	// 设置请求参数
	params := url.Values{}
	params.Add("appid", c.GetSocial().GetWechat().GetAppId())
	params.Add("secret", c.GetSocial().GetWechat().GetAppSecret())
	params.Add("js_code", code)
	params.Add("grant_type", grantType)

	// 设置 HTTP 客户端，带有超时机制
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// 构造请求 URL
	requestURL := fmt.Sprintf("%s?%s", apiURL, params.Encode())

	// 创建 HTTP 请求
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		logger.Error("failed to create HTTP request", zap.Error(err))
		return "", err
	}

	// 发起请求
	resp, err := client.Do(req)
	if err != nil {
		logger.Error("failed to request WeChat API", zap.Error(err))
		return "", err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Warn("failed to close response body", zap.Error(err))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		logger.Error("received non-OK response from WeChat API", zap.Int("status_code", resp.StatusCode))
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// 解析响应体
	var weChatResp WeChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&weChatResp); err != nil {
		logger.Error("failed to decode response from WeChat API", zap.Error(err))
		return "", err
	}

	// 检查微信 API 返回的错误码
	if weChatResp.ErrCode != 0 {
		logger.Warn("WeChat API returned an error", zap.Int("err_code", weChatResp.ErrCode), zap.String("err_msg", weChatResp.ErrMsg))
		return "", fmt.Errorf("WeChat API error: %s (code: %d)", weChatResp.ErrMsg, weChatResp.ErrCode)
	}

	return weChatResp.OpenID, nil
}

// getByteGameOpenID 获取 TikTok OpenID 的函数
func getByteGameOpenID(ctx context.Context, logger *zap.Logger, cfg Config, code, anonymousCode string) (string, error) {
	opt := new(credential.Config).
		SetClientKey(cfg.GetSocial().GetTikTok().GetAppId()).       // 改成自己的app_id
		SetClientSecret(cfg.GetSocial().GetTikTok().GetAppSecret()) // 改成自己的secret

	sdkClient, err := openApiSdkClient.NewClient(opt)
	if err != nil {
		logger.Error("tiktok sdk init err:", zap.Error(err))
		return "", err
	}

	sdkRequest := &openApiSdkClient.AppsJscode2sessionRequest{}
	sdkRequest.SetAnonymousCode(anonymousCode)
	sdkRequest.SetCode(code)
	sdkRequest.SetSecret(cfg.GetSocial().GetTikTok().GetAppSecret())
	sdkRequest.SetAppid(cfg.GetSocial().GetTikTok().GetAppId())

	// sdk调用
	sdkResponse, err := sdkClient.AppsJscode2session(sdkRequest)
	if err != nil {
		logger.Error("tiktok call err:", zap.Error(err))
		return "", err
	}
	return *sdkResponse.Openid, nil
}

func (s *ApiServer) AuthenticateWechat(ctx context.Context, in *game.AuthenticateWechatRequest) (*api.Session, error) {
	openID, err := getWeChatOpenID(ctx, s.logger, s.config, in.Code)
	if err != nil || openID == "" {
		return nil, err
	}
	return s.createSession(ctx, openID)
}

func (s *ApiServer) AuthenticateTikTok(ctx context.Context, in *game.AuthenticateTiktokRequest) (*api.Session, error) {
	openID, err := getByteGameOpenID(ctx, s.logger, s.config, in.Code, in.AnonymousCode)
	if err != nil || openID == "" {
		return nil, err
	}
	return s.createSession(ctx, openID)
}

// AlipayOAuthResponse 支付宝 OAuth 响应结构
type AlipayOAuthResponse struct {
	AlipaySystemOauthTokenResponse struct {
		Code        string `json:"code"`
		Msg         string `json:"msg"`
		SubCode     string `json:"sub_code,omitempty"`
		SubMsg      string `json:"sub_msg,omitempty"`
		UserID      string `json:"user_id"`
		OpenID      string `json:"open_id"`
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	} `json:"alipay_system_oauth_token_response"`
	Sign string `json:"sign"`
}

// getAlipayUserID 获取支付宝用户的 UserID
func getAlipayUserID(ctx context.Context, logger *zap.Logger, cfg Config, code string) (string, error) {
	alipayCfg := cfg.GetSocial().GetAlipay()
	if alipayCfg == nil {
		return "", fmt.Errorf("Alipay configuration is not set")
	}

	appID := alipayCfg.GetAppId()
	privateKey := alipayCfg.GetPrivateKey()
	if appID == "" || privateKey == "" {
		return "", fmt.Errorf("Alipay app_id or private_key is not configured")
	}

	// 解析私钥（支持PEM格式和直接Base64字符串）
	var privateKeyBytes []byte
	block, _ := pem.Decode([]byte(privateKey))
	if block != nil {
		// PEM格式（有头尾标记）
		privateKeyBytes = block.Bytes
	} else {
		// 直接Base64字符串（没有PEM头尾），尝试解码
		var decodeErr error
		privateKeyBytes, decodeErr = base64.StdEncoding.DecodeString(strings.TrimSpace(privateKey))
		if decodeErr != nil {
			// 如果Base64解码失败，尝试直接使用原始字符串
			privateKeyBytes = []byte(privateKey)
		}
	}

	var privateKeyParsed *rsa.PrivateKey
	var err error

	// 尝试解析PKCS8格式
	if key, parseErr := x509.ParsePKCS8PrivateKey(privateKeyBytes); parseErr == nil {
		var ok bool
		privateKeyParsed, ok = key.(*rsa.PrivateKey)
		if !ok {
			return "", fmt.Errorf("not an RSA private key")
		}
	} else {
		// 尝试解析PKCS1格式
		privateKeyParsed, err = x509.ParsePKCS1PrivateKey(privateKeyBytes)
		if err != nil {
			return "", fmt.Errorf("failed to parse RSA private key (tried PKCS8 and PKCS1): %v", err)
		}
	}

	// 准备请求参数
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	params := map[string]string{
		"app_id":     appID,
		"method":     "alipay.system.oauth.token",
		"format":     "JSON",
		"charset":    "utf-8",
		"sign_type":  "RSA2",
		"timestamp":  timestamp,
		"version":    "1.0",
		"code":       code,
		"grant_type": "authorization_code",
	}

	// 生成签名字符串
	signStr := buildSignString(params)

	// 使用 RSA2 签名
	hashed := sha256.Sum256([]byte(signStr))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKeyParsed, crypto.SHA256, hashed[:])
	if err != nil {
		logger.Error("failed to sign Alipay request", zap.Error(err))
		return "", fmt.Errorf("failed to sign request: %v", err)
	}

	// Base64 编码签名
	signatureBase64 := base64.StdEncoding.EncodeToString(signature)
	params["sign"] = signatureBase64

	// 构建请求 URL
	apiURL := "https://openapi.alipay.com/gateway.do"
	values := url.Values{}
	for k, v := range params {
		values.Add(k, v)
	}
	requestURL := fmt.Sprintf("%s?%s", apiURL, values.Encode())

	// 创建 HTTP 请求
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		logger.Error("failed to create HTTP request", zap.Error(err))
		return "", err
	}

	// 发起请求
	resp, err := client.Do(req)
	if err != nil {
		logger.Error("failed to request Alipay API", zap.Error(err))
		return "", err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Warn("failed to close response body", zap.Error(err))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		logger.Error("received non-OK response from Alipay API", zap.Int("status_code", resp.StatusCode))
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// 解析响应体
	var alipayResp AlipayOAuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&alipayResp); err != nil {
		logger.Error("failed to decode response from Alipay API", zap.Error(err))
		return "", err
	}

	if alipayResp.AlipaySystemOauthTokenResponse.OpenID == "" {
		return "", fmt.Errorf("Alipay API returned empty openid")
	}

	return alipayResp.AlipaySystemOauthTokenResponse.OpenID, nil
}

// buildSignString 构建签名字符串（按字典序排序并拼接）
func buildSignString(params map[string]string) string {
	// 排除 sign 和空值
	filteredParams := make(map[string]string)
	for k, v := range params {
		if k != "sign" && v != "" {
			filteredParams[k] = v
		}
	}

	// 排序
	keys := make([]string, 0, len(filteredParams))
	for k := range filteredParams {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// 拼接
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, filteredParams[k]))
	}

	return strings.Join(parts, "&")
}

func (s *ApiServer) AuthenticateAlipay(ctx context.Context, in *game.AuthenticateAlipayRequest) (*api.Session, error) {
	userID, err := getAlipayUserID(ctx, s.logger, s.config, in.Code)
	if err != nil || userID == "" {
		return nil, err
	}
	return s.createSession(ctx, userID)
}

// Common function to create a session
func (s *ApiServer) createSession(ctx context.Context, openID string) (*api.Session, error) {
	username := generateUsername()
	dbUserID, dbUsername, created, err := AuthenticateDevice(ctx, s.logger, s.db, openID, username, true)
	if err != nil {
		return nil, err
	}

	if s.config.GetSession().SingleSession {
		s.sessionCache.RemoveAll(uuid.Must(uuid.FromString(dbUserID)))
	}

	tokenID := uuid.Must(uuid.NewV4()).String()
	tokenIssuedAt := time.Now().Unix()
	token, exp := generateToken(s.config, tokenID, tokenIssuedAt, dbUserID, dbUsername, nil)
	refreshToken, refreshExp := generateRefreshToken(s.config, tokenID, tokenIssuedAt, dbUserID, dbUsername, nil)
	s.sessionCache.Add(uuid.FromStringOrNil(dbUserID), exp, tokenID, refreshExp, tokenID)

	return &api.Session{Created: created, Token: token, RefreshToken: refreshToken}, nil
}
