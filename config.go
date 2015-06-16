package main

import (
	"encoding/json"
	"io/ioutil"
)

type CORSConfig struct {
	AllowOrigins     []string `json:"allow_origins"`
	AllowMethods     []string `json:"allow_methods"`
	AllowHeaders     []string `json:"allow_headers"`
	ExposeHeaders    []string `json:"expose_headers"`
	AllowCerdentials bool     `json:"allow_cerdentials"`
}

type ACLConfig struct {
	IPACLEnabled bool            `json:"ip_acl_enabled"`
	IPWhiteList  []string        `json:"ip_white_list"`
	AuthEnabled  bool            `json:"auth_enabled"`
	BasicAuth    BasicAuthConfig `json:"basic_auth"`
}

type BasicAuthConfig map[string]string

type HTTPConfig struct {
	Address string     `json:"address"`
	CORS    CORSConfig `json:"cors"`
}

type StringKeeperConfig struct {
	HTTP HTTPConfig `json:"http"`
	ACL  ACLConfig  `json:"acl"`
}

func LoadConfig(fileName string) (config StringKeeperConfig, err error) {
	var data []byte
	if data, err = ioutil.ReadFile(fileName); err != nil {
		return
	}

	if err = json.Unmarshal(data, &config); err != nil {
		return
	}

	return
}

func DefaultConfig() StringKeeperConfig {
	return StringKeeperConfig{
		HTTPConfig{
			":8080",
			CORSConfig{
				AllowOrigins:     []string{"*"},
				AllowMethods:     []string{"POST"},
				AllowHeaders:     []string{"Origin"},
				ExposeHeaders:    []string{"Content-Length"},
				AllowCerdentials: true,
			},
		},
		ACLConfig{
			IPACLEnabled: false,
			IPWhiteList:  []string{},
			AuthEnabled:  false,
			BasicAuth:    BasicAuthConfig{},
		},
	}
}
