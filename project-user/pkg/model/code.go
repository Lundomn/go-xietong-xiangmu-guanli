package model

import (
	"test.com/project-common/errs"
)

var (
	RedisError           = errs.NewError(999, "redis错误")
	DBError              = errs.NewError(998, "db错误")
	InvalidParameter     = errs.NewError(400, "参数无效")
	NoLogin              = errs.NewError(997, "未登录")
	NoLegalMobile        = errs.NewError(10102001, "手机号不合法")
	CaptchaNotExist      = errs.NewError(10102002, "验证码不存在或者已过期")
	CaptchaError         = errs.NewError(10102003, "验证码错误")
	EmailExist           = errs.NewError(10102004, "邮箱已经存在了")
	AccountExist         = errs.NewError(10102005, "账号已经存在了")
	MobileExist          = errs.NewError(10102006, "手机号已经存在了")
	AccountAndPwdError   = errs.NewError(10102007, "账号密码不正确")
	CaptchaGenerateError = errs.NewError(10102008, "验证码生成失败")
	SmsNotConfigured     = errs.NewError(10102009, "短信云服务未配置")
	SmsSendError         = errs.NewError(10102010, "短信发送失败，请稍后重试")
	CaptchaTooFrequent   = errs.NewError(10102011, "验证码发送过于频繁，请稍后再试")
	SmsConfigError       = errs.NewError(10102012, "短信云服务配置无效")
)
