package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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

type BiliGameResponse struct {
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

// getBiliGameOpenID 获取 BiliGame 用户的 OpenID
func getBiliGameOpenID(ctx context.Context, logger *zap.Logger, c Config, code, appId string) (string, error) {
	apiURL := "https://miniapp.bilibili.com/api/sns/jscode2session"
	grantType := "authorization_code"

	if appId == "" {
		logger.Error("appid is required")
		return "", fmt.Errorf("appid is required")
	}

	biliGameConfig := c.GetSocial().GetBiliGame()
	if biliGameConfig == nil {
		logger.Error("BiliGame configuration is not set")
		return "", fmt.Errorf("BiliGame configuration is not set")
	}

	secret := biliGameConfig.GetSecret(appId)
	if secret == "" {
		logger.Error("BiliGame appid not found in configuration", zap.String("appid", appId))
		return "", fmt.Errorf("BiliGame appid not found: %s", appId)
	}

	params := url.Values{}
	params.Add("appid", appId)
	params.Add("secret", secret)
	params.Add("js_code", code)
	params.Add("grant_type", grantType)

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	requestURL := fmt.Sprintf("%s?%s", apiURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		logger.Error("failed to create HTTP request", zap.Error(err))
		return "", err
	}

	resp, err := client.Do(req)
	if err != nil {
		logger.Error("failed to request BiliGame API", zap.Error(err))
		return "", err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Warn("failed to close response body", zap.Error(err))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		logger.Error("received non-OK response from BiliGame API", zap.Int("status_code", resp.StatusCode))
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var biliGameResp BiliGameResponse
	if err := json.NewDecoder(resp.Body).Decode(&biliGameResp); err != nil {
		logger.Error("failed to decode response from BiliGame API", zap.Error(err))
		return "", err
	}

	if biliGameResp.ErrCode != 0 {
		logger.Warn("BiliGame API returned an error", zap.Int("err_code", biliGameResp.ErrCode), zap.String("err_msg", biliGameResp.ErrMsg))
		return "", fmt.Errorf("BiliGame API error: %s (code: %d)", biliGameResp.ErrMsg, biliGameResp.ErrCode)
	}

	return biliGameResp.OpenID, nil
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

func (s *ApiServer) AuthenticateBiliGame(ctx context.Context, in *game.AuthenticateBiliGameRequest) (*api.Session, error) {
	openID, err := getBiliGameOpenID(ctx, s.logger, s.config, in.Code, in.Appid)
	if err != nil || openID == "" {
		return nil, err
	}
	return s.createSession(ctx, openID)
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
