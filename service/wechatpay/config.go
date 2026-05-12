// Package wechatpay wraps github.com/wechatpay-apiv3/wechatpay-go (Tencent-maintained API v3 SDK).
//
// Configuration (environment):
//   WECHATPAY_APP_ID                 — WeChat appid (required for Native prepay)
//   WECHATPAY_MCH_ID                 — merchant id
//   WECHATPAY_MCH_CERTIFICATE_SERIAL — merchant API cert serial number
//   WECHATPAY_MCH_API_V3_KEY         — API v3 key (32 bytes)
//   WECHATPAY_MCH_PRIVATE_KEY_PATH   — path to apiclient_key.pem (preferred)
//   WECHATPAY_MCH_PRIVATE_KEY        — PEM text (alternative to PATH)
package wechatpay

import (
	"crypto/rsa"
	"os"
	"strings"
)

// Config holds WeChat Pay merchant settings loaded from the environment.
type Config struct {
	AppID                      string
	MchID                      string
	MchCertificateSerialNumber string
	MchAPIv3Key                string
	PrivateKey                 *rsa.PrivateKey
}

func LoadConfigFromEnv() (*Config, error) {
	appID := strings.TrimSpace(os.Getenv("WECHATPAY_APP_ID"))
	mchID := strings.TrimSpace(os.Getenv("WECHATPAY_MCH_ID"))
	serial := strings.TrimSpace(os.Getenv("WECHATPAY_MCH_CERTIFICATE_SERIAL"))
	if s := strings.TrimSpace(os.Getenv("WECHATPAY_MCH_CERTIFICATE_SERIAL_NUMBER")); s != "" {
		serial = s
	}
	apiV3 := strings.TrimSpace(os.Getenv("WECHATPAY_MCH_API_V3_KEY"))
	keyPath := strings.TrimSpace(os.Getenv("WECHATPAY_MCH_PRIVATE_KEY_PATH"))
	keyPEM := strings.TrimSpace(os.Getenv("WECHATPAY_MCH_PRIVATE_KEY"))

	if appID == "" || mchID == "" || serial == "" || apiV3 == "" {
		return nil, nil // not configured — caller treats as disabled
	}

	var pk *rsa.PrivateKey
	var err error
	if keyPath != "" {
		pk, err = loadPrivateKeyFromFile(keyPath)
	} else if keyPEM != "" {
		pk, err = loadPrivateKeyFromPEM([]byte(keyPEM))
	} else {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &Config{
		AppID:                      appID,
		MchID:                      mchID,
		MchCertificateSerialNumber: serial,
		MchAPIv3Key:                apiV3,
		PrivateKey:                 pk,
	}, nil
}

func (c *Config) IsComplete() bool {
	return c != nil && c.AppID != "" && c.MchID != "" && c.MchCertificateSerialNumber != "" &&
		c.MchAPIv3Key != "" && c.PrivateKey != nil
}
