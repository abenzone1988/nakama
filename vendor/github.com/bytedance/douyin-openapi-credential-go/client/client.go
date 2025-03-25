// This file is auto-generated, don't edit it. Thanks.
package client

import (
	"github.com/alibabacloud-go/tea/tea"
)

type Config struct {
	ClientKey    *string `json:"clientKey,omitempty" xml:"clientKey,omitempty" require:"true"`
	ClientSecret *string `json:"clientSecret,omitempty" xml:"clientSecret,omitempty" require:"true"`
}

func (s Config) String() string {
	return tea.Prettify(s)
}

func (s Config) GoString() string {
	return s.String()
}

func (s *Config) SetClientKey(v string) *Config {
	s.ClientKey = &v
	return s
}

func (s *Config) SetClientSecret(v string) *Config {
	s.ClientSecret = &v
	return s
}

type Token struct {
	AccessToken *string `json:"accessToken,omitempty" xml:"accessToken,omitempty" require:"true"`
	ExpiresIn   *int64  `json:"expiresIn,omitempty" xml:"expiresIn,omitempty" require:"true"`
}

func (s Token) String() string {
	return tea.Prettify(s)
}

func (s Token) GoString() string {
	return s.String()
}

func (s *Token) SetAccessToken(v string) *Token {
	s.AccessToken = &v
	return s
}

func (s *Token) SetExpiresIn(v int64) *Token {
	s.ExpiresIn = &v
	return s
}

type Credential interface {
	GetClientKey() (_result *string)
	GetClientSecret() (_result *string)
	GetClientToken() (_result *Token, _err error)
	GetAccessToken() (_result *Token, _err error)
	//GetBusinessToken(scope *string, code *string) (_result *string, _err error)
	//GetUserAccessToken(code *string) (_result *string, _err error)
}

func NewCredential(config *Config) (Credential, error) {
	client := new(DefaultCredential)
	err := client.Init(config)
	return client, err
}
