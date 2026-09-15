package project_service_v1

import (
	"context"
	"go.uber.org/zap"
	"strconv"
	"test.com/project-common/encrypts"
	"test.com/project-common/errs"
	"test.com/project-grpc/project"
	"test.com/project-project/internal/data"
	"test.com/project-project/pkg/model"
	"time"
)

func (ps *ProjectService) UpdateCollectProject(ctx context.Context, msg *project.ProjectRpcMessage) (*project.CollectProjectResponse, error) {
	if msg == nil || msg.ProjectCode == "" || (msg.CollectType != "collect" && msg.CollectType != "cancel") {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	projectCodeStr, _ := encrypts.Decrypt(msg.ProjectCode, model.AESKey)
	projectCode, _ := strconv.ParseInt(projectCodeStr, 10, 64)
	if projectCode <= 0 || msg.MemberId <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	c, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	projectMember, memberErr := ps.projectRepo.FindProjectByPIdAndMemId(c, projectCode, msg.MemberId)
	if memberErr != nil {
		return nil, errs.GrpcError(model.DBError)
	}
	if projectMember == nil || projectMember.Id <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	var err error
	if "collect" == msg.CollectType {
		collected, findErr := ps.projectRepo.FindCollectByPidAndMemId(c, projectCode, msg.MemberId)
		if findErr != nil {
			return nil, errs.GrpcError(model.DBError)
		}
		if collected {
			return &project.CollectProjectResponse{}, nil
		}
		pc := &data.ProjectCollection{
			ProjectCode: projectCode,
			MemberCode:  msg.MemberId,
			CreateTime:  time.Now().UnixMilli(),
		}
		err = ps.projectRepo.SaveProjectCollect(c, pc)
	}
	if "cancel" == msg.CollectType {
		err = ps.projectRepo.DeleteProjectCollect(c, msg.MemberId, projectCode)
	}
	if err != nil {
		zap.L().Error("project UpdateCollectProject SaveProjectCollect error", zap.Error(err))
		return nil, errs.GrpcError(model.DBError)
	}
	return &project.CollectProjectResponse{}, nil
}
