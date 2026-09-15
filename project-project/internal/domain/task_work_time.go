package domain

import (
	"context"
	"go.uber.org/zap"
	"test.com/project-common/errs"
	"test.com/project-project/internal/dao"
	"test.com/project-project/internal/data"
	"test.com/project-project/internal/repo"
	"test.com/project-project/pkg/model"
	"time"
)

type TaskWorkTimeDomain struct {
	taskWorkTimeRepo repo.TaskWorkTimeRepo
	userRpcDomain    *UserRpcDomain
}

func NewTaskWorkTimeDomain() *TaskWorkTimeDomain {
	return &TaskWorkTimeDomain{
		taskWorkTimeRepo: dao.NewTaskWorkTimeDao(),
		userRpcDomain:    NewUserRpcDomain(),
	}
}

func (d *TaskWorkTimeDomain) TaskWorkTimeList(ctx context.Context, taskCode int64) ([]*data.TaskWorkTimeDisplay, *errs.BError) {
	c, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var list []*data.TaskWorkTime
	var err error
	list, err = d.taskWorkTimeRepo.FindWorkTimeList(c, taskCode)
	if err != nil {
		zap.L().Error("project task TaskWorkTimeList taskWorkTimeRepo.FindWorkTimeList error", zap.Error(err))
		return nil, model.DBError
	}
	if len(list) == 0 {
		return []*data.TaskWorkTimeDisplay{}, nil
	}
	var displayList []*data.TaskWorkTimeDisplay
	var mIdList []int64
	for _, v := range list {
		if v == nil {
			continue
		}
		mIdList = append(mIdList, v.MemberCode)
	}
	_, mMap, err := d.userRpcDomain.MemberList(c, mIdList)
	if err != nil {
		return nil, errs.ToBError(err)
	}
	for _, v := range list {
		if v == nil {
			continue
		}
		display := v.ToDisplay()
		message := mMap[v.MemberCode]
		if message != nil {
			display.Member = data.Member{
				Name:   message.Name,
				Id:     message.Id,
				Avatar: message.Avatar,
				Code:   message.Code,
			}
		}
		displayList = append(displayList, display)
	}
	return displayList, nil
}
