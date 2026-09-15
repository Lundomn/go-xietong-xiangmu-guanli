package department_service_v1

import (
	"context"
	"github.com/jinzhu/copier"
	"strings"
	"test.com/project-common/encrypts"
	"test.com/project-common/errs"
	"test.com/project-grpc/department"
	"test.com/project-project/internal/dao"
	"test.com/project-project/internal/database/tran"
	"test.com/project-project/internal/domain"
	"test.com/project-project/internal/repo"
	"test.com/project-project/pkg/model"
)

type DepartmentService struct {
	department.UnimplementedDepartmentServiceServer
	cache            repo.Cache
	transaction      tran.Transaction
	departmentDomain *domain.DepartmentDomain
}

func New() *DepartmentService {
	return &DepartmentService{
		cache:            dao.Rc,
		transaction:      dao.NewTransaction(),
		departmentDomain: domain.NewDepartmentDomain(),
	}
}
func (d *DepartmentService) List(ctx context.Context, msg *department.DepartmentReqMessage) (*department.ListDepartmentMessage, error) {
	if msg == nil {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	organizationCode := encrypts.DecryptNoErr(msg.OrganizationCode)
	if organizationCode <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	var parentDepartmentCode int64
	if msg.ParentDepartmentCode != "" {
		parentDepartmentCode = encrypts.DecryptNoErr(msg.ParentDepartmentCode)
		if parentDepartmentCode <= 0 {
			return nil, errs.GrpcError(model.InvalidParameter)
		}
	}
	dps, total, err := d.departmentDomain.List(ctx,
		organizationCode,
		parentDepartmentCode,
		msg.Page,
		msg.PageSize)
	if err != nil {
		return nil, errs.GrpcError(err)
	}
	var list []*department.DepartmentMessage
	copier.Copy(&list, dps)
	return &department.ListDepartmentMessage{List: list, Total: total}, nil
}

func (d *DepartmentService) Save(ctx context.Context, msg *department.DepartmentReqMessage) (*department.DepartmentMessage, error) {
	if msg == nil || strings.TrimSpace(msg.Name) == "" {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	organizationCode := encrypts.DecryptNoErr(msg.OrganizationCode)
	if organizationCode <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	var departmentCode int64
	if msg.DepartmentCode != "" {
		departmentCode = encrypts.DecryptNoErr(msg.DepartmentCode)
	}
	var parentDepartmentCode int64
	if msg.ParentDepartmentCode != "" {
		parentDepartmentCode = encrypts.DecryptNoErr(msg.ParentDepartmentCode)
		if parentDepartmentCode <= 0 {
			return nil, errs.GrpcError(model.InvalidParameter)
		}
	}
	dp, err := d.departmentDomain.Save(ctx,
		organizationCode,
		departmentCode,
		parentDepartmentCode,
		msg.Name)
	if err != nil {
		return &department.DepartmentMessage{}, errs.GrpcError(err)
	}
	var res = &department.DepartmentMessage{}
	copier.Copy(res, dp)
	return res, nil
}

func (d *DepartmentService) Read(ctx context.Context, msg *department.DepartmentReqMessage) (*department.DepartmentMessage, error) {
	if msg == nil || msg.DepartmentCode == "" {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	//organizationCode := encrypts.DecryptNoErr(msg.OrganizationCode)
	departmentCode := encrypts.DecryptNoErr(msg.DepartmentCode)
	if departmentCode <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	organizationCode := encrypts.DecryptNoErr(msg.OrganizationCode)
	if organizationCode <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	dp, err := d.departmentDomain.FindDepartmentById(ctx, departmentCode)
	if err != nil {
		return &department.DepartmentMessage{}, errs.GrpcError(err)
	}
	if dp == nil || dp.OrganizationCode != organizationCode {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	var res = &department.DepartmentMessage{}
	copier.Copy(res, dp.ToDisplay())
	return res, nil
}
