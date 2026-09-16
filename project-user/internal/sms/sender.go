package sms

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultTencentEndpoint = "https://sms.tencentcloudapi.com"
	defaultTencentRegion   = "ap-guangzhou"
	defaultCountryCode     = "+86"
	defaultTimeout         = 8 * time.Second
	contentType            = "application/json; charset=utf-8"
)

// Sender delivers a one-time verification code to a mobile number.
// Implementations must not log the code or credentials.
type Sender interface {
	Send(ctx context.Context, mobile, code string) error
}

// TencentConfig contains the minimum configuration required by Tencent Cloud
// SMS SendSms (API 3.0, version 2021-01-11).
type TencentConfig struct {
	SecretID       string
	SecretKey      string
	Region         string
	Endpoint       string
	SmsSdkAppID    string
	SignName       string
	TemplateID     string
	CountryCode    string
	SessionContext string
	Timeout        time.Duration
}

// TencentSender calls Tencent Cloud's official SMS API without exposing the
// provider SDK to the rest of the user service. This keeps the deployment
// image small and makes the signing logic easy to test locally.
type TencentSender struct {
	config   TencentConfig
	endpoint *url.URL
	client   *http.Client
}

// NewFromEnv creates the configured sender. An empty provider, "none" or
// "local" intentionally means that no external SMS is sent; this is useful
// for local demos when MS_CAPTCHA_EXPOSE_CODE=1 is enabled.
func NewFromEnv() (Sender, error) {
	provider := strings.ToLower(strings.TrimSpace(os.Getenv("MS_SMS_PROVIDER")))
	switch provider {
	case "", "none", "local":
		return nil, nil
	case "tencent", "tencent-cloud", "tencentcloud":
		return NewTencentSender(TencentConfig{
			SecretID:       strings.TrimSpace(os.Getenv("MS_SMS_SECRET_ID")),
			SecretKey:      strings.TrimSpace(os.Getenv("MS_SMS_SECRET_KEY")),
			Region:         strings.TrimSpace(os.Getenv("MS_SMS_REGION")),
			Endpoint:       strings.TrimSpace(os.Getenv("MS_SMS_ENDPOINT")),
			SmsSdkAppID:    strings.TrimSpace(os.Getenv("MS_SMS_SDK_APP_ID")),
			SignName:       strings.TrimSpace(os.Getenv("MS_SMS_SIGN_NAME")),
			TemplateID:     strings.TrimSpace(os.Getenv("MS_SMS_TEMPLATE_ID")),
			CountryCode:    strings.TrimSpace(os.Getenv("MS_SMS_COUNTRY_CODE")),
			SessionContext: strings.TrimSpace(os.Getenv("MS_SMS_SESSION_CONTEXT")),
			Timeout:        parseTimeout(os.Getenv("MS_SMS_TIMEOUT_SECONDS")),
		})
	default:
		return nil, fmt.Errorf("不支持的短信服务商 %q", provider)
	}
}

func parseTimeout(value string) time.Duration {
	seconds, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || seconds <= 0 || seconds > 60 {
		return defaultTimeout
	}
	return time.Duration(seconds) * time.Second
}

// NewTencentSender validates configuration before the service starts using
// the provider. It accepts an HTTP endpoint in tests; production should use
// the default HTTPS endpoint or another HTTPS-compatible endpoint.
func NewTencentSender(config TencentConfig) (*TencentSender, error) {
	if config.SecretID == "" || config.SecretKey == "" {
		return nil, errors.New("腾讯云短信缺少 MS_SMS_SECRET_ID 或 MS_SMS_SECRET_KEY")
	}
	if config.SmsSdkAppID == "" || config.SignName == "" || config.TemplateID == "" {
		return nil, errors.New("腾讯云短信缺少 MS_SMS_SDK_APP_ID、MS_SMS_SIGN_NAME 或 MS_SMS_TEMPLATE_ID")
	}
	if config.Region == "" {
		config.Region = defaultTencentRegion
	}
	if config.Endpoint == "" {
		config.Endpoint = defaultTencentEndpoint
	}
	if config.CountryCode == "" {
		config.CountryCode = defaultCountryCode
	}
	if config.Timeout <= 0 {
		config.Timeout = defaultTimeout
	}
	parsed, err := url.Parse(config.Endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("腾讯云短信 MS_SMS_ENDPOINT 不是合法 URL")
	}
	return &TencentSender{
		config:   config,
		endpoint: parsed,
		client:   &http.Client{Timeout: config.Timeout},
	}, nil
}

type sendSmsRequest struct {
	PhoneNumberSet   []string `json:"PhoneNumberSet"`
	SmsSdkAppID      string   `json:"SmsSdkAppId"`
	SignName         string   `json:"SignName"`
	TemplateID       string   `json:"TemplateId"`
	TemplateParamSet []string `json:"TemplateParamSet"`
	SessionContext   string   `json:"SessionContext,omitempty"`
}

type sendSmsResponse struct {
	Response struct {
		Error         *apiError       `json:"Error"`
		SendStatusSet []sendSmsStatus `json:"SendStatusSet"`
		RequestID     string          `json:"RequestId"`
	} `json:"Response"`
}

type apiError struct {
	Code    string `json:"Code"`
	Message string `json:"Message"`
}

type sendSmsStatus struct {
	Code        string `json:"Code"`
	Message     string `json:"Message"`
	PhoneNumber string `json:"PhoneNumber"`
}

// Send sends one domestic or international phone number. Tencent's approved
// template must contain exactly one variable, which receives the code.
func (s *TencentSender) Send(ctx context.Context, mobile, code string) error {
	if s == nil || s.endpoint == nil || s.client == nil {
		return errors.New("腾讯云短信发送器未初始化")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(code) == "" {
		return errors.New("短信验证码为空")
	}
	phone, err := normalizePhone(mobile, s.config.CountryCode)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(sendSmsRequest{
		PhoneNumberSet:   []string{phone},
		SmsSdkAppID:      s.config.SmsSdkAppID,
		SignName:         s.config.SignName,
		TemplateID:       s.config.TemplateID,
		TemplateParamSet: []string{code},
		SessionContext:   s.config.SessionContext,
	})
	if err != nil {
		return err
	}

	timestamp := time.Now().Unix()
	service := "sms"
	host := s.endpoint.Host
	uri := s.endpoint.EscapedPath()
	if uri == "" {
		uri = "/"
	}
	authorization := buildAuthorization(
		s.config.SecretID,
		s.config.SecretKey,
		service,
		host,
		uri,
		s.endpoint.RawQuery,
		payload,
		timestamp,
	)

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Host = host
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("X-TC-Action", "SendSms")
	request.Header.Set("X-TC-Version", "2021-01-11")
	request.Header.Set("X-TC-Timestamp", strconv.FormatInt(timestamp, 10))
	request.Header.Set("X-TC-Region", s.config.Region)
	request.Header.Set("Authorization", authorization)

	response, err := s.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("腾讯云短信 HTTP 状态异常: %d", response.StatusCode)
	}

	var parsed sendSmsResponse
	if err := json.Unmarshal(responseBody, &parsed); err != nil {
		return fmt.Errorf("腾讯云短信响应格式异常: %w", err)
	}
	if parsed.Response.Error != nil {
		return fmt.Errorf("腾讯云短信 API 错误: %s", parsed.Response.Error.Message)
	}
	if len(parsed.Response.SendStatusSet) == 0 {
		return errors.New("腾讯云短信未返回发送状态")
	}
	status := parsed.Response.SendStatusSet[0]
	if !strings.EqualFold(status.Code, "ok") {
		return fmt.Errorf("腾讯云短信发送失败: %s", status.Message)
	}
	return nil
}

func normalizePhone(value, countryCode string) (string, error) {
	value = strings.TrimSpace(value)
	value = strings.NewReplacer(" ", "", "-", "", "(", "", ")", "").Replace(value)
	if value == "" {
		return "", errors.New("手机号码为空")
	}
	if strings.HasPrefix(value, "00") {
		value = "+" + value[2:]
	}
	if !strings.HasPrefix(value, "+") {
		countryCode = strings.TrimSpace(countryCode)
		if countryCode == "" {
			countryCode = defaultCountryCode
		}
		if !strings.HasPrefix(countryCode, "+") {
			countryCode = "+" + countryCode
		}
		value = countryCode + value
	}
	if len(value) < 8 || !allDigits(value[1:]) {
		return "", errors.New("手机号码不是合法的 E.164 格式")
	}
	return value, nil
}

func allDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func buildAuthorization(secretID, secretKey, service, host, uri, query string, payload []byte, timestamp int64) string {
	date := time.Unix(timestamp, 0).UTC().Format("2006-01-02")
	hashedPayload := sha256Hex(payload)
	canonicalHeaders := "content-type:" + contentType + "\n" + "host:" + host + "\n"
	signedHeaders := "content-type;host"
	canonicalRequest := strings.Join([]string{
		http.MethodPost,
		uri,
		query,
		canonicalHeaders,
		signedHeaders,
		hashedPayload,
	}, "\n")
	credentialScope := date + "/" + service + "/tc3_request"
	stringToSign := strings.Join([]string{
		"TC3-HMAC-SHA256",
		strconv.FormatInt(timestamp, 10),
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")
	secretDate := hmacSHA256([]byte("TC3"+secretKey), date)
	secretService := hmacSHA256(secretDate, service)
	secretSigning := hmacSHA256(secretService, "tc3_request")
	signature := hex.EncodeToString(hmacSHA256(secretSigning, stringToSign))
	return "TC3-HMAC-SHA256 Credential=" + secretID + "/" + credentialScope +
		", SignedHeaders=" + signedHeaders + ", Signature=" + signature
}

func sha256Hex(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func hmacSHA256(key []byte, value string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}
