package domain

import (
	"context"
	"test.com/project-common/errs"
	"test.com/project-grpc/user/login"
	"test.com/project-project/internal/rpc"
	"time"
)

type UserRpcDomain struct {
	lc login.LoginServiceClient
}

func NewUserRpcDomain() *UserRpcDomain {
	return &UserRpcDomain{
		lc: rpc.LoginServiceClient,
	}
}

func (d *UserRpcDomain) MemberList(ctx context.Context, mIdList []int64) ([]*login.MemberMessage, map[int64]*login.MemberMessage, error) {
	c, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	messageList, err := d.lc.FindMemInfoByIds(c, &login.UserMessage{MIds: mIdList})
	if err != nil {
		return nil, nil, err
	}
	if messageList == nil {
		return nil, nil, errs.NewError(998, "用户服务返回为空")
	}
	mMap := make(map[int64]*login.MemberMessage)
	for _, v := range messageList.List {
		if v == nil {
			continue
		}
		mMap[v.Id] = v
	}
	return messageList.List, mMap, nil
}

func (d *UserRpcDomain) MemberInfo(ctx context.Context, memberCode int64) (*login.MemberMessage, error) {
	c, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	memberMessage, err := d.lc.FindMemInfoById(c, &login.UserMessage{MemId: memberCode})
	return memberMessage, err
}
