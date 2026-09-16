package sms

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTencentSenderSendsApprovedTemplatePayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", request.Method)
		}
		if request.Header.Get("X-TC-Action") != "SendSms" {
			t.Errorf("X-TC-Action = %q", request.Header.Get("X-TC-Action"))
		}
		if request.Header.Get("X-TC-Version") != "2021-01-11" {
			t.Errorf("X-TC-Version = %q", request.Header.Get("X-TC-Version"))
		}
		authorization := request.Header.Get("Authorization")
		if !strings.HasPrefix(authorization, "TC3-HMAC-SHA256 Credential=test-id/") {
			t.Errorf("unexpected authorization header: %q", authorization)
		}

		var payload sendSmsRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if len(payload.PhoneNumberSet) != 1 || payload.PhoneNumberSet[0] != "+8613800138000" {
			t.Fatalf("phone payload = %#v", payload.PhoneNumberSet)
		}
		if payload.SmsSdkAppID != "1400000000" || payload.SignName != "协同项目" || payload.TemplateID != "1000001" {
			t.Fatalf("template payload = %#v", payload)
		}
		if len(payload.TemplateParamSet) != 1 || payload.TemplateParamSet[0] != "123456" {
			t.Fatalf("template params = %#v", payload.TemplateParamSet)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"Response":{"SendStatusSet":[{"Code":"Ok","Message":"send success"}],"RequestId":"test-request"}}`))
	}))
	defer server.Close()

	sender, err := NewTencentSender(TencentConfig{
		SecretID:       "test-id",
		SecretKey:      "test-key",
		Endpoint:       server.URL,
		SmsSdkAppID:    "1400000000",
		SignName:       "协同项目",
		TemplateID:     "1000001",
		CountryCode:    "+86",
		SessionContext: "test",
	})
	if err != nil {
		t.Fatalf("NewTencentSender: %v", err)
	}
	if err := sender.Send(context.Background(), "13800138000", "123456"); err != nil {
		t.Fatalf("Send: %v", err)
	}
}

func TestTencentSenderReturnsProviderError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(`{"Response":{"Error":{"Code":"UnauthorizedOperation","Message":"invalid credential"},"RequestId":"test-request"}}`))
	}))
	defer server.Close()

	sender, err := NewTencentSender(TencentConfig{
		SecretID:    "test-id",
		SecretKey:   "test-key",
		Endpoint:    server.URL,
		SmsSdkAppID: "1400000000",
		SignName:    "协同项目",
		TemplateID:  "1000001",
	})
	if err != nil {
		t.Fatalf("NewTencentSender: %v", err)
	}
	if err := sender.Send(context.Background(), "13800138000", "123456"); err == nil || !strings.Contains(err.Error(), "invalid credential") {
		t.Fatalf("Send error = %v, want provider error", err)
	}
}

func TestNormalizePhone(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		country string
		want    string
		ok      bool
	}{
		{name: "domestic", input: "138 0013-8000", country: "+86", want: "+8613800138000", ok: true},
		{name: "international plus", input: "+14155550100", country: "+86", want: "+14155550100", ok: true},
		{name: "international 00", input: "0014155550100", country: "+86", want: "+14155550100", ok: true},
		{name: "invalid", input: "abc", country: "+86", ok: false},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := normalizePhone(testCase.input, testCase.country)
			if (err == nil) != testCase.ok {
				t.Fatalf("error = %v, want success = %v", err, testCase.ok)
			}
			if got != testCase.want {
				t.Errorf("phone = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestNewFromEnvProviderValidation(t *testing.T) {
	t.Setenv("MS_SMS_PROVIDER", "none")
	sender, err := NewFromEnv()
	if err != nil || sender != nil {
		t.Fatalf("none provider = (%v, %v), want (nil, nil)", sender, err)
	}

	t.Setenv("MS_SMS_PROVIDER", "tencent")
	if _, err := NewFromEnv(); err == nil {
		t.Fatal("tencent provider without credentials should fail configuration validation")
	}

	t.Setenv("MS_SMS_PROVIDER", "unknown")
	if _, err := NewFromEnv(); err == nil {
		t.Fatal("unknown provider should fail configuration validation")
	}
}
