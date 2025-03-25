package client

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-resty/resty/v2"
)

type CommonToken struct {
	Token     string
	StartTime time.Time
	ExpireIn  int64
}

func (t *CommonToken) Expire() bool {
	if t == nil {
		return true
	}

	return t.StartTime.Add(time.Duration(t.ExpireIn) * time.Second).After(time.Now())
}

type DefaultCredential struct {
	ClientKey    *string `json:"clientKey,omitempty" xml:"clientKey,omitempty" require:"true"`
	ClientSecret *string `json:"clientSecret,omitempty" xml:"clientSecret,omitempty" require:"true"`

	ClientToken *CommonToken
	AccessToken *CommonToken
}

func (client *DefaultCredential) Init(config *Config) (_err error) {
	client.ClientKey = config.ClientKey
	client.ClientSecret = config.ClientSecret
	return
}

func (client *DefaultCredential) GetClientKey() (_result *string) {
	return client.ClientKey
}

func (client *DefaultCredential) GetClientSecret() (_result *string) {
	return client.ClientSecret
}

func (client *DefaultCredential) GetClientToken() (_result *Token, _err error) {

	if client.ClientToken.Expire() {
		token, err := client.getClientToken()
		if err != nil {
			return nil, err
		}

		client.ClientToken = token
	}

	return &Token{AccessToken: &client.ClientToken.Token, ExpiresIn: &client.ClientToken.ExpireIn}, nil
}

func (client *DefaultCredential) getClientToken() (*CommonToken, error) {
	bodyStr, _ := json.Marshal(map[string]string{
		"client_key":    *client.ClientKey,
		"client_secret": *client.ClientSecret,
		"grant_type":    "client_credential",
	})
	res := new(ClientTokenResponse)

	httpResp, err := resty.New().R().SetHeader("Content-Type", "application/json").
		SetBody(bodyStr).
		SetResult(res).
		Post("https://open.douyin.com/oauth/client_token/")
	if err != nil {
		return nil, fmt.Errorf("get getAccessToken failed, unknow err, err=%w", err)
	}

	if httpResp.StatusCode() != 200 {
		return nil, fmt.Errorf("get getClientToken failed, http statusCode not 200, code=%d", httpResp.StatusCode())
	}

	if res.Data.ErrorCode != 0 {
		return nil, fmt.Errorf("get getClientToken failed, biz code not 0, err_no=%d err_msg=%s", res.Data.ErrorCode, res.Data.Description)
	}

	return &CommonToken{
		Token:     res.Data.AccessToken,
		StartTime: time.Now().Add(-5 * time.Second),
		ExpireIn:  int64(res.Data.ExpiresIn),
	}, nil

}

func (client *DefaultCredential) GetAccessToken() (_result *Token, _err error) {

	if client.AccessToken.Expire() {
		token, err := client.getAccessToken()
		if err != nil {
			return nil, err
		}

		client.AccessToken = token
	}

	return &Token{AccessToken: &client.AccessToken.Token, ExpiresIn: &client.AccessToken.ExpireIn}, nil
}

func (client *DefaultCredential) getAccessToken() (*CommonToken, error) {
	bodyStr, _ := json.Marshal(map[string]string{
		"appid":      *client.ClientKey,
		"secret":     *client.ClientSecret,
		"grant_type": "client_credential",
	})
	res := new(AccessTokenResponse)

	httpResp, err := resty.New().R().SetHeader("Content-Type", "application/json").
		SetBody(bodyStr).
		SetResult(res).
		Post("https://developer.toutiao.com/api/apps/v2/token")
	if err != nil {
		return nil, fmt.Errorf("get getAccessToken failed, unknow err, err=%w", err)
	}

	if httpResp.StatusCode() != 200 {
		return nil, fmt.Errorf("get getAccessToken failed, http statusCode not 200, code=%d", httpResp.StatusCode())
	}

	if res.ErrNo != 0 {
		return nil, fmt.Errorf("get getAccessToken failed, biz code not 0, err_no=%d err_msg=%s", res.ErrNo, res.ErrTips)
	}

	return &CommonToken{
		Token:     res.Data.AccessToken,
		StartTime: time.Now().Add(-5 * time.Second),
		ExpireIn:  int64(res.Data.ExpiresIn),
	}, nil
}

//
//func (client *DefaultCredential) GetBusinessToken(scope *string, code *string) (_result *string, _err error) {
//
//	key := fmt.Sprintf("%s-%s", *scope, *code)
//
//	userAccessToken, ok := client.BusinessToken[key]
//	userRefreshToken := client.BusinessRefreshToken[key]
//
//	// token存在且未过期
//	if ok && !client.ClientToken.Expire() {
//		return &userAccessToken.Token, nil
//	}
//
//	// token存在已过期，且refreshToken未过期
//	if ok && userAccessToken.Expire() && !userRefreshToken.Expire() {
//		token, refreshToken, err := client.refreshBusinessToken(userRefreshToken.Token)
//		if err != nil {
//			return nil, err
//		}
//
//		client.BusinessToken[key] = token
//		client.BusinessRefreshToken[key] = refreshToken
//
//		return &token.Token, nil
//	}
//
//	// 其他情况直接获取新的token
//	openid, err := client.code2session(*code)
//	if err != nil {
//		return nil, err
//	}
//
//	token, refreshToken, err := client.getBusinessToken(scope, openid)
//	if err != nil {
//		return nil, err
//	}
//	client.BusinessToken[key] = token
//	client.BusinessRefreshToken[key] = refreshToken
//
//	return &token.Token, nil
//}

//func (client *DefaultCredential) GetUserAccessToken(code *string) (_result *string, _err error) {
//
//	userAccessToken, ok := client.UserAccessToken[*code]
//	userRefreshToken := client.UserRefreshToken[*code]
//
//	// token存在且未过期
//	if ok && !client.ClientToken.Expire() {
//		return &userAccessToken.Token, nil
//	}
//
//	// token存在已过期，且refreshToken未过期
//	if ok && userAccessToken.Expire() && !userRefreshToken.Expire() {
//		token, refreshToken, err := client.refreshUserAccessToken(code)
//		if err != nil {
//			return nil, err
//		}
//		client.UserAccessToken[*code] = token
//
//		client.UserAccessToken[*code] = token
//		client.UserRefreshToken[*code] = refreshToken
//
//		return &token.Token, nil
//	}
//
//	// 其他情况直接获取新的token
//	token, refreshToken, err := client.getUserAccessToken(code)
//	if err != nil {
//		return nil, err
//	}
//	client.UserAccessToken[*code] = token
//	client.UserRefreshToken[*code] = refreshToken
//
//	return &token.Token, nil
//}
//
//func (client *DefaultCredential) code2session(code string) (_result *string, _err error) {
//
//	type TokenResponse struct {
//		ErrNo   int    `json:"err_no"`
//		ErrTips string `json:"err_tips"`
//		Data    struct {
//			SessionKey      string `json:"session_key"`
//			Openid          string `json:"openid"`
//			AnonymousOpenid string `json:"anonymous_openid"`
//			Unionid         string `json:"unionid"`
//		} `json:"data"`
//	}
//
//	bodyStr, _ := json.Marshal(map[string]string{
//		"appid":          *client.ClientKey,
//		"secret":         *client.ClientSecret,
//		"code":           code,
//		"anonymous_code": "",
//	})
//	res := new(TokenResponse)
//
//	_, err := resty.New().R().SetHeader("Content-Type", "application/json").
//		SetBody(bodyStr).
//		SetResult(res).
//		Post("https://developer.toutiao.com/api/apps/v2/jscode2session")
//	if err != nil {
//		log.Println("http statusCode not 200")
//		return nil, err
//	}
//
//	if res.ErrNo != 0 {
//		log.Println("res.ErrNo != 0 ,res.Err_tips=", res.ErrTips)
//		return nil, fmt.Errorf("%s", res.ErrTips)
//	}
//
//	return &res.Data.Openid, nil
//}
//
//func (client *DefaultCredential) getBusinessToken(scope *string, openid *string) (*CommonToken, *CommonToken, error) {
//	bodyStr, _ := json.Marshal(map[string]string{
//		"client_key":    *client.ClientKey,
//		"client_secret": *client.ClientSecret,
//		"open_id":       *openid,
//		"scope":         *scope,
//	})
//	res := new(BusinessTokenResponse)
//
//	_, err := resty.New().R().SetHeader("Content-Type", "application/json").
//		SetBody(bodyStr).
//		SetResult(res).
//		Post("https://open.douyin.com/oauth/business_token/")
//	if err != nil {
//		log.Println("http statusCode not 200")
//		return nil, nil, err
//	}
//
//	if res.ErrNo != 0 {
//		log.Println("res.ErrNo != 0 ,res.Err_tips=", res.ErrTips)
//		return nil, nil, fmt.Errorf("%s", res.ErrTips)
//	}
//	return &CommonToken{
//			Token:     res.Data.BizToken,
//			StartTime: time.Now(),
//			ExpireIn:  int64(res.Data.BizExpiresIn),
//		}, &CommonToken{
//			Token:     res.Data.BizRefreshToken,
//			StartTime: time.Now(),
//			ExpireIn:  int64(res.Data.BizRefreshExpiresIn),
//		}, nil
//
//}
//
//func (client *DefaultCredential) refreshBusinessToken(refreshToken string) (_result *CommonToken, token *CommonToken, _err error) {
//
//	bodyStr, _ := json.Marshal(map[string]string{
//		"client_key":    *client.ClientKey,
//		"client_secret": *client.ClientSecret,
//		"refresh_token": refreshToken,
//	})
//	res := new(BusinessTokenResponse)
//
//	_, err := resty.New().R().SetHeader("Content-Type", "application/json").
//		SetBody(bodyStr).
//		SetResult(res).
//		Post("https://open.douyin.com/oauth/refresh_biz_token/")
//	if err != nil {
//		log.Println("http statusCode not 200")
//		return nil, nil, err
//	}
//
//	if res.ErrNo != 0 {
//		log.Println("res.ErrNo != 0 ,res.Err_tips=", res.ErrTips)
//		return nil, nil, fmt.Errorf("%s", res.ErrTips)
//	}
//
//	return &CommonToken{
//			Token:     res.Data.BizToken,
//			StartTime: time.Now(),
//			ExpireIn:  int64(res.Data.BizExpiresIn),
//		}, &CommonToken{
//			Token:     res.Data.BizRefreshToken,
//			StartTime: time.Now(),
//			ExpireIn:  int64(res.Data.BizRefreshExpiresIn),
//		}, nil
//}
//
//func (client *DefaultCredential) getUserAccessToken(code *string) (*CommonToken, *CommonToken, error) {
//	bodyStr, _ := json.Marshal(map[string]string{
//		"client_key":    *client.ClientKey,
//		"client_secret": *client.ClientSecret,
//		"grant_type":    "authorization_code",
//		"code":          *code,
//	})
//	res := new(UserAccessTokenResponse)
//
//	_, err := resty.New().R().SetHeader("Content-Type", "application/json").
//		SetBody(bodyStr).
//		SetResult(res).
//		Post("https://open.douyin.com/oauth/access_token/")
//	if err != nil {
//		log.Println("http statusCode not 200")
//		return nil, nil, err
//	}
//
//	if res.Data.ErrorCode != 0 {
//		log.Println("res.ErrNo != 0 ,res.Err_tips=", res.Data.Description)
//		return nil, nil, fmt.Errorf("%s", res.Data.Description)
//	}
//
//	return &CommonToken{
//			Token:     res.Data.AccessToken,
//			StartTime: time.Now(),
//			ExpireIn:  int64(res.Data.ExpiresIn),
//		}, &CommonToken{
//			Token:     res.Data.RefreshToken,
//			StartTime: time.Now(),
//			ExpireIn:  int64(res.Data.RefreshExpiresIn),
//		}, nil
//}
//
//func (client *DefaultCredential) refreshUserAccessToken(code *string) (token *CommonToken, refreshToken *CommonToken, _err error) {
//
//	bodyStr, _ := json.Marshal(map[string]string{
//		"client_key":    *client.ClientKey,
//		"grant_type":    "refresh_token",
//		"refresh_token": client.UserRefreshToken[*code].Token,
//	})
//	res := new(UserAccessTokenResponse)
//
//	_, err := resty.New().R().SetHeader("Content-Type", "application/json").
//		SetBody(bodyStr).
//		SetResult(res).
//		Post("https://open.douyin.com/oauth/refresh_token/")
//	if err != nil {
//		log.Println("http statusCode not 200")
//		return nil, nil, err
//	}
//
//	if res.Data.ErrorCode != 0 {
//		log.Println("res.ErrNo != 0 ,res.Err_tips=", res.Data.Description)
//		return nil, nil, fmt.Errorf("%s", res.Data.Description)
//	}
//
//	return &CommonToken{
//			Token:     res.Data.AccessToken,
//			StartTime: time.Now(),
//			ExpireIn:  int64(res.Data.ExpiresIn),
//		}, &CommonToken{
//			Token:     res.Data.RefreshToken,
//			StartTime: time.Now(),
//			ExpireIn:  int64(res.Data.RefreshExpiresIn),
//		}, nil
//}
