package client

type BusinessTokenResponse struct {
	ErrNo   int    `json:"error_code"`
	ErrTips string `json:"message"`
	Data    struct {
		BizToken            string `json:"biz_token"`
		BizExpiresIn        int    `json:"biz_expires_in"`
		BizRefreshToken     string `json:"biz_refresh_token"`
		BizRefreshExpiresIn int64  `json:"biz_refresh_expires_in"`
	} `json:"data"`
}

type UserAccessTokenResponse struct {
	Data struct {
		AccessToken      string `json:"access_token"`
		Captcha          string `json:"captcha"`
		DescUrl          string `json:"desc_url"`
		Description      string `json:"description"`
		ErrorCode        int    `json:"error_code"`
		ExpiresIn        int    `json:"expires_in"`
		LogId            string `json:"log_id"`
		OpenId           string `json:"open_id"`
		RefreshExpiresIn int    `json:"refresh_expires_in"`
		RefreshToken     string `json:"refresh_token"`
		Scope            string `json:"scope"`
	} `json:"data"`
	Message string `json:"message"`
}

type ClientTokenResponse struct {
	Data struct {
		AccessToken string `json:"access_token"`
		Description string `json:"description"`
		ErrorCode   int    `json:"error_code"`
		ExpiresIn   int    `json:"expires_in"`
	} `json:"data"`
	Message string `json:"message"`
}

type AccessTokenResponse struct {
	ErrNo   int    `json:"err_no"`
	ErrTips string `json:"err_tips"`
	Data    struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	} `json:"data"`
}
