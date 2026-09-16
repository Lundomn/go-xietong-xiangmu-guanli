package login_service_v1

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/json"
	"github.com/go-redis/redis/v8"
	"github.com/jinzhu/copier"
	"go.uber.org/zap"
	"math/big"
	"os"
	"strconv"
	"strings"
	common "test.com/project-common"
	"test.com/project-common/encrypts"
	"test.com/project-common/errs"
	"test.com/project-common/jwts"
	"test.com/project-common/tms"
	"test.com/project-grpc/user/login"
	"test.com/project-user/config"
	"test.com/project-user/internal/dao"
	"test.com/project-user/internal/data/member"
	"test.com/project-user/internal/data/organization"
	"test.com/project-user/internal/database"
	"test.com/project-user/internal/database/tran"
	"test.com/project-user/internal/repo"
	"test.com/project-user/internal/security/password"
	"test.com/project-user/internal/sms"
	"test.com/project-user/pkg/model"
	"time"
)

const (
	captchaTTL      = 15 * time.Minute
	captchaCooldown = 60 * time.Second
)

type LoginService struct {
	login.UnimplementedLoginServiceServer
	cache            repo.Cache
	memberRepo       repo.MemberRepo
	organizationRepo repo.OrganizationRepo
	transaction      tran.Transaction
	smsSender        sms.Sender
	smsConfigErr     error
}

func New() *LoginService {
	sender, configErr := sms.NewFromEnv()
	if configErr != nil {
		zap.L().Error("短信云服务配置无效", zap.Error(configErr))
	}
	return &LoginService{
		cache:            dao.Rc,
		memberRepo:       dao.NewMemberDao(),
		organizationRepo: dao.NewOrganizationDao(),
		transaction:      dao.NewTransaction(),
		smsSender:        sender,
		smsConfigErr:     configErr,
	}
}

func (ls *LoginService) GetCaptcha(ctx context.Context, msg *login.CaptchaMessage) (*login.CaptchaResponse, error) {
	//1.获取参数
	if msg == nil {
		return nil, errs.GrpcError(model.NoLegalMobile)
	}
	mobile := msg.Mobile
	//2.校验参数
	if !common.VerifyMobile(mobile) {
		return nil, errs.GrpcError(model.NoLegalMobile)
	}
	exposeCode := os.Getenv("MS_CAPTCHA_EXPOSE_CODE") == "1"
	if !exposeCode {
		if ls.smsConfigErr != nil {
			return nil, errs.GrpcError(model.SmsConfigError)
		}
		if ls.smsSender == nil {
			return nil, errs.GrpcError(model.SmsNotConfigured)
		}
		cooldownKey := model.RegisterRedisKey + "COOLDOWN_" + mobile
		if _, cooldownErr := ls.cache.Get(ctx, cooldownKey); cooldownErr == nil {
			return nil, errs.GrpcError(model.CaptchaTooFrequent)
		} else if cooldownErr != redis.Nil {
			zap.L().Error("验证码频率检查失败", zap.Error(cooldownErr))
			return nil, errs.GrpcError(model.RedisError)
		}
	}
	//3.生成验证码，并在发送前写入 Redis，发送失败时回滚。这样云服务
	//不可用时不会把“已发送”的假状态返回给前端。
	n, err := cryptorand.Int(cryptorand.Reader, big.NewInt(900000))
	if err != nil {
		zap.L().Error("生成验证码失败", zap.Error(err))
		return nil, errs.GrpcError(model.CaptchaGenerateError)
	}
	code := strconv.FormatInt(n.Int64()+100000, 10)
	codeKey := model.RegisterRedisKey + mobile
	if err = ls.cache.Put(ctx, codeKey, code, captchaTTL); err != nil {
		zap.L().Error("验证码存入 Redis 失败", zap.Error(err))
		return nil, errs.GrpcError(model.RedisError)
	}
	if !exposeCode {
		if err = ls.smsSender.Send(ctx, mobile, code); err != nil {
			if deleteErr := ls.cache.Delete(ctx, codeKey); deleteErr != nil && deleteErr != redis.Nil {
				zap.L().Warn("短信发送失败后清理验证码失败", zap.Error(deleteErr))
			}
			zap.L().Warn("短信验证码发送失败", zap.Error(err))
			return nil, errs.GrpcError(model.SmsSendError)
		}
		cooldownKey := model.RegisterRedisKey + "COOLDOWN_" + mobile
		if cooldownErr := ls.cache.Put(ctx, cooldownKey, "1", captchaCooldown); cooldownErr != nil {
			// 短信已经发送成功，频率限制写入失败不应让用户重试并重复付费。
			zap.L().Warn("验证码频率限制写入失败", zap.Error(cooldownErr))
		}
		zap.L().Info("短信验证码已发送", zap.String("mobile", maskMobile(mobile)))
	} else {
		zap.L().Info("本地演示验证码已生成", zap.String("mobile", maskMobile(mobile)))
	}
	return &login.CaptchaResponse{Code: code}, nil
}

func maskMobile(mobile string) string {
	if len(mobile) < 7 {
		return "***"
	}
	return mobile[:3] + "****" + mobile[len(mobile)-4:]
}
func (ls *LoginService) Register(ctx context.Context, msg *login.RegisterMessage) (*login.RegisterResponse, error) {
	if msg == nil || msg.Name == "" || msg.Password == "" ||
		!common.VerifyEmailFormat(msg.Email) || !common.VerifyMobile(msg.Mobile) || msg.Captcha == "" {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	c := ctx
	//1.可以校验参数
	//2.校验验证码
	redisCode, err := ls.cache.Get(c, model.RegisterRedisKey+msg.Mobile)
	if err == redis.Nil {
		return nil, errs.GrpcError(model.CaptchaNotExist)
	}
	if err != nil {
		zap.L().Error("Register redis get error", zap.Error(err))
		return nil, errs.GrpcError(model.RedisError)
	}
	if redisCode != msg.Captcha {
		return nil, errs.GrpcError(model.CaptchaError)
	}
	//3.校验业务逻辑（邮箱是否被注册 账号是否被注册 手机号是否被注册）
	exist, err := ls.memberRepo.GetMemberByEmail(c, msg.Email)
	if err != nil {
		zap.L().Error("Register db get error", zap.Error(err))
		return nil, errs.GrpcError(model.DBError)
	}
	if exist {
		return nil, errs.GrpcError(model.EmailExist)
	}
	exist, err = ls.memberRepo.GetMemberByAccount(c, msg.Name)
	if err != nil {
		zap.L().Error("Register db get error", zap.Error(err))
		return nil, errs.GrpcError(model.DBError)
	}
	if exist {
		return nil, errs.GrpcError(model.AccountExist)
	}
	exist, err = ls.memberRepo.GetMemberByMobile(c, msg.Mobile)
	if err != nil {
		zap.L().Error("Register db get error", zap.Error(err))
		return nil, errs.GrpcError(model.DBError)
	}
	if exist {
		return nil, errs.GrpcError(model.MobileExist)
	}
	//4.执行业务 将数据存入member表 生成一个数据 存入组织表 organization
	pwd, err := password.Hash(msg.Password)
	if err != nil {
		zap.L().Error("Register password hash error", zap.Error(err))
		return nil, errs.GrpcError(model.DBError)
	}
	mem := &member.Member{
		Account:       msg.Name,
		Password:      pwd,
		Name:          msg.Name,
		Mobile:        msg.Mobile,
		Email:         msg.Email,
		CreateTime:    time.Now().UnixMilli(),
		LastLoginTime: time.Now().UnixMilli(),
		Status:        model.Normal,
	}
	err = ls.transaction.Action(func(conn database.DbConn) error {
		err = ls.memberRepo.SaveMember(conn, c, mem)
		if err != nil {
			zap.L().Error("Register db SaveMember error", zap.Error(err))
			return errs.GrpcError(model.DBError)
		}
		//存入组织
		org := &organization.Organization{
			Name:       mem.Name + "个人组织",
			MemberId:   mem.Id,
			CreateTime: time.Now().UnixMilli(),
			Personal:   model.Personal,
			Avatar:     "https://gimg2.baidu.com/image_search/src=http%3A%2F%2Fc-ssl.dtstatic.com%2Fuploads%2Fblog%2F202103%2F31%2F20210331160001_9a852.thumb.1000_0.jpg&refer=http%3A%2F%2Fc-ssl.dtstatic.com&app=2002&size=f9999,10000&q=a80&n=0&g=0n&fmt=auto?sec=1673017724&t=ced22fc74624e6940fd6a89a21d30cc5",
		}
		err = ls.organizationRepo.SaveOrganization(conn, c, org)
		if err != nil {
			zap.L().Error("register SaveOrganization db err", zap.Error(err))
			return errs.GrpcError(model.DBError)
		}
		return nil
	})

	if err == nil {
		if deleteErr := ls.cache.Delete(c, model.RegisterRedisKey+msg.Mobile); deleteErr != nil && deleteErr != redis.Nil {
			zap.L().Warn("注册成功后删除验证码失败", zap.Error(deleteErr))
		}
		if deleteErr := ls.cache.Delete(c, model.RegisterRedisKey+"COOLDOWN_"+msg.Mobile); deleteErr != nil && deleteErr != redis.Nil {
			zap.L().Warn("注册成功后删除验证码频率限制失败", zap.Error(deleteErr))
		}
	}

	//5. 返回
	return &login.RegisterResponse{}, err
}

func (ls *LoginService) Login(ctx context.Context, msg *login.LoginMessage) (*login.LoginResponse, error) {
	if msg == nil {
		return nil, errs.GrpcError(model.AccountAndPwdError)
	}
	c := ctx
	//1.去数据库查询 账号密码是否正确
	if msg.Account == "" || msg.Password == "" {
		return nil, errs.GrpcError(model.AccountAndPwdError)
	}
	mem, err := ls.memberRepo.FindMemberByAccount(c, msg.Account)
	if err != nil {
		zap.L().Error("Login db FindMember error", zap.Error(err))
		return nil, errs.GrpcError(model.DBError)
	}
	if mem == nil {
		return nil, errs.GrpcError(model.AccountAndPwdError)
	}
	passwordOK, legacyPassword := password.Verify(mem.Password, msg.Password)
	if !passwordOK {
		return nil, errs.GrpcError(model.AccountAndPwdError)
	}
	if legacyPassword {
		if upgraded, upgradeErr := password.Hash(msg.Password); upgradeErr == nil {
			if updateErr := ls.memberRepo.UpdateMemberPassword(c, mem.Id, upgraded); updateErr != nil {
				zap.L().Warn("Login legacy password upgrade failed", zap.Error(updateErr))
			}
		} else {
			zap.L().Warn("Login legacy password hash failed", zap.Error(upgradeErr))
		}
	}
	memMsg := &login.MemberMessage{}
	err = copier.Copy(memMsg, mem)
	if err != nil {
		return nil, errs.GrpcError(model.DBError)
	}
	memMsg.Code, _ = encrypts.EncryptInt64(mem.Id, model.AESKey)
	memMsg.LastLoginTime = tms.FormatByMill(mem.LastLoginTime)
	memMsg.CreateTime = tms.FormatByMill(mem.CreateTime)
	//2.根据用户id查组织
	orgs, err := ls.organizationRepo.FindOrganizationByMemId(c, mem.Id)
	if err != nil {
		zap.L().Error("Login db FindMember error", zap.Error(err))
		return nil, errs.GrpcError(model.DBError)
	}
	var orgsMessage []*login.OrganizationMessage
	err = copier.Copy(&orgsMessage, orgs)
	if err != nil {
		return nil, errs.GrpcError(model.DBError)
	}
	for _, v := range orgsMessage {
		if v == nil {
			continue
		}
		v.Code, _ = encrypts.EncryptInt64(v.Id, model.AESKey)
		v.OwnerCode = memMsg.Code
		if o := organization.ToMap(orgs)[v.Id]; o != nil {
			v.CreateTime = tms.FormatByMill(o.CreateTime)
		}
	}
	for _, org := range orgs {
		if org != nil {
			memMsg.OrganizationCode, _ = encrypts.EncryptInt64(org.Id, model.AESKey)
			break
		}
	}
	//3.用jwt生成token
	memIdStr := strconv.FormatInt(mem.Id, 10)
	exp := time.Duration(config.C.JwtConfig.AccessExp*3600*24) * time.Second
	rExp := time.Duration(config.C.JwtConfig.RefreshExp*3600*24) * time.Second
	token := jwts.CreateToken(memIdStr, exp, config.C.JwtConfig.AccessSecret, rExp, config.C.JwtConfig.RefreshSecret, msg.Ip)
	//可以给token做加密处理 增加安全性
	tokenList := &login.TokenMessage{
		AccessToken:    token.AccessToken,
		RefreshToken:   token.RefreshToken,
		AccessTokenExp: token.AccessExp,
		TokenType:      "bearer",
	}
	// 登录成功前同步写入缓存，保证返回的 token 可以立即通过认证。
	marshal, err := json.Marshal(mem)
	if err != nil {
		zap.L().Error("Login marshal member error", zap.Error(err))
		return nil, errs.GrpcError(model.RedisError)
	}
	if err = ls.cache.Put(c, model.Member+"::"+memIdStr, string(marshal), exp); err != nil {
		zap.L().Error("Login cache member error", zap.Error(err))
		return nil, errs.GrpcError(model.RedisError)
	}
	orgsJSON, err := json.Marshal(orgs)
	if err != nil {
		zap.L().Error("Login marshal organization error", zap.Error(err))
		return nil, errs.GrpcError(model.RedisError)
	}
	if err = ls.cache.Put(c, model.MemberOrganization+"::"+memIdStr, string(orgsJSON), exp); err != nil {
		zap.L().Error("Login cache organization error", zap.Error(err))
		return nil, errs.GrpcError(model.RedisError)
	}
	return &login.LoginResponse{
		Member:           memMsg,
		OrganizationList: orgsMessage,
		TokenList:        tokenList,
	}, nil
}

func (ls *LoginService) TokenVerify(ctx context.Context, msg *login.LoginMessage) (*login.LoginResponse, error) {
	if msg == nil {
		return nil, errs.GrpcError(model.NoLogin)
	}
	tokenParts := strings.Fields(msg.Token)
	if len(tokenParts) == 2 && strings.EqualFold(tokenParts[0], "bearer") {
		tokenParts = tokenParts[1:]
	}
	if len(tokenParts) != 1 {
		return nil, errs.GrpcError(model.NoLogin)
	}
	parseToken, err := jwts.ParseToken(tokenParts[0], config.C.JwtConfig.AccessSecret, msg.Ip)
	if err != nil {
		zap.L().Error("Login  TokenVerify error", zap.Error(err))
		return nil, errs.GrpcError(model.NoLogin)
	}
	//从缓存中查询 如果没有 直接返回认证失败
	memJson, err := ls.cache.Get(ctx, model.Member+"::"+parseToken)
	if err != nil {
		zap.L().Error("TokenVerify cache get member error", zap.Error(err))
		return nil, errs.GrpcError(model.NoLogin)
	}
	if memJson == "" {
		zap.L().Error("TokenVerify cache get member expire")
		return nil, errs.GrpcError(model.NoLogin)
	}
	memberById := &member.Member{}
	if err = json.Unmarshal([]byte(memJson), memberById); err != nil || memberById.Id == 0 {
		zap.L().Error("TokenVerify cache member decode error", zap.Error(err))
		return nil, errs.GrpcError(model.NoLogin)
	}
	//数据库查询 优化点 登录之后 应该把用户信息缓存起来
	memMsg := &login.MemberMessage{}
	copier.Copy(memMsg, memberById)
	memMsg.Code, _ = encrypts.EncryptInt64(memberById.Id, model.AESKey)

	orgsJson, err := ls.cache.Get(ctx, model.MemberOrganization+"::"+parseToken)
	if err != nil {
		zap.L().Error("TokenVerify cache get organization error", zap.Error(err))
		return nil, errs.GrpcError(model.NoLogin)
	}
	if orgsJson == "" {
		zap.L().Error("TokenVerify cache get organization expire")
		return nil, errs.GrpcError(model.NoLogin)
	}
	var orgs []*organization.Organization
	if err = json.Unmarshal([]byte(orgsJson), &orgs); err != nil {
		zap.L().Error("TokenVerify cache organization decode error", zap.Error(err))
		return nil, errs.GrpcError(model.NoLogin)
	}

	for _, org := range orgs {
		if org != nil {
			memMsg.OrganizationCode, _ = encrypts.EncryptInt64(org.Id, model.AESKey)
			break
		}
	}
	memMsg.CreateTime = tms.FormatByMill(memberById.CreateTime)
	return &login.LoginResponse{Member: memMsg}, nil
}

func (l *LoginService) MyOrgList(ctx context.Context, msg *login.UserMessage) (*login.OrgListResponse, error) {
	if msg == nil || msg.MemId <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	memId := msg.MemId
	orgs, err := l.organizationRepo.FindOrganizationByMemId(ctx, memId)
	if err != nil {
		zap.L().Error("MyOrgList FindOrganizationByMemId err", zap.Error(err))
		return nil, errs.GrpcError(model.DBError)
	}
	var orgsMessage []*login.OrganizationMessage
	err = copier.Copy(&orgsMessage, orgs)
	if err != nil {
		return nil, errs.GrpcError(model.DBError)
	}
	for _, org := range orgsMessage {
		if org == nil {
			continue
		}
		org.Code, _ = encrypts.EncryptInt64(org.Id, model.AESKey)
	}
	return &login.OrgListResponse{OrganizationList: orgsMessage}, nil
}

func (ls *LoginService) FindMemInfoById(ctx context.Context, msg *login.UserMessage) (*login.MemberMessage, error) {
	if msg == nil || msg.MemId <= 0 {
		return nil, errs.GrpcError(model.NoLogin)
	}
	memberById, err := ls.memberRepo.FindMemberById(ctx, msg.MemId)
	if err != nil {
		zap.L().Error("TokenVerify db FindMemberById error", zap.Error(err))
		return nil, errs.GrpcError(model.DBError)
	}
	if memberById == nil {
		return nil, errs.GrpcError(model.NoLogin)
	}
	memMsg := &login.MemberMessage{}
	copier.Copy(memMsg, memberById)
	memMsg.Code, _ = encrypts.EncryptInt64(memberById.Id, model.AESKey)
	orgs, err := ls.organizationRepo.FindOrganizationByMemId(ctx, memberById.Id)
	if err != nil {
		zap.L().Error("TokenVerify db FindMember error", zap.Error(err))
		return nil, errs.GrpcError(model.DBError)
	}
	for _, org := range orgs {
		if org != nil {
			memMsg.OrganizationCode, _ = encrypts.EncryptInt64(org.Id, model.AESKey)
			break
		}
	}
	memMsg.CreateTime = tms.FormatByMill(memberById.CreateTime)
	return memMsg, nil
}

func (ls *LoginService) FindMemInfoByIds(ctx context.Context, msg *login.UserMessage) (*login.MemberMessageList, error) {
	if msg == nil {
		return &login.MemberMessageList{List: nil}, nil
	}
	memberList, err := ls.memberRepo.FindMemberByIds(ctx, msg.MIds)
	if err != nil {
		zap.L().Error("FindMemInfoByIds db memberRepo.FindMemberByIds error", zap.Error(err))
		return nil, errs.GrpcError(model.DBError)
	}
	if memberList == nil || len(memberList) <= 0 {
		return &login.MemberMessageList{List: nil}, nil
	}
	mMap := make(map[int64]*member.Member)
	for _, v := range memberList {
		if v == nil {
			continue
		}
		mMap[v.Id] = v
	}
	var memMsgs []*login.MemberMessage
	copier.Copy(&memMsgs, memberList)
	for _, v := range memMsgs {
		if v == nil {
			continue
		}
		m := mMap[v.Id]
		if m == nil {
			continue
		}
		v.CreateTime = tms.FormatByMill(m.CreateTime)
		v.Code = encrypts.EncryptNoErr(v.Id)
	}

	return &login.MemberMessageList{List: memMsgs}, nil
}
