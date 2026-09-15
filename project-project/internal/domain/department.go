package domain

import (
	"context"
	"test.com/project-common/errs"
	"test.com/project-project/internal/dao"
	"test.com/project-project/internal/data"
	"test.com/project-project/internal/repo"
	"test.com/project-project/pkg/model"
	"time"
)

type DepartmentDomain struct {
	departmentRepo repo.DepartmentRepo
}

func (d *DepartmentDomain) FindDepartmentById(ctx context.Context, id int64) (*data.Department, *errs.BError) {
	c, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	dp, err := d.departmentRepo.FindDepartmentById(c, id)
	if err != nil {
		return nil, model.DBError
	}
	return dp, nil
}

func (d *DepartmentDomain) List(ctx context.Context, organizationCode int64, parentDepartmentCode int64, page int64, size int64) ([]*data.DepartmentDisplay, int64, *errs.BError) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 10
	}
	c, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	list, total, err := d.departmentRepo.ListDepartment(c, organizationCode, parentDepartmentCode, page, size)
	if err != nil {
		return nil, 0, model.DBError
	}
	var dList []*data.DepartmentDisplay
	for _, v := range list {
		if v == nil {
			continue
		}
		dList = append(dList, v.ToDisplay())
	}
	return dList, total, nil
}

func (d *DepartmentDomain) Save(
	ctx context.Context,
	organizationCode int64,
	departmentCode int64,
	parentDepartmentCode int64,
	name string) (*data.DepartmentDisplay, *errs.BError) {

	c, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if organizationCode <= 0 || name == "" {
		return nil, model.InvalidParameter
	}
	if parentDepartmentCode > 0 {
		parent, err := d.departmentRepo.FindDepartmentById(c, parentDepartmentCode)
		if err != nil {
			return nil, model.DBError
		}
		if parent == nil || parent.OrganizationCode != organizationCode {
			return nil, model.InvalidParameter
		}
	}
	dpm, err := d.departmentRepo.FindDepartment(c, organizationCode, parentDepartmentCode, name)
	if err != nil {
		return nil, model.DBError
	}
	if dpm == nil {
		dpm = &data.Department{
			Name:             name,
			OrganizationCode: organizationCode,
			CreateTime:       time.Now().UnixMilli(),
		}
		if parentDepartmentCode > 0 {
			dpm.Pcode = parentDepartmentCode
		}
		err := d.departmentRepo.Save(c, dpm)
		if err != nil {
			return nil, model.DBError
		}
		return dpm.ToDisplay(), nil
	}
	return dpm.ToDisplay(), nil
}

func NewDepartmentDomain() *DepartmentDomain {
	return &DepartmentDomain{
		departmentRepo: dao.NewDepartmentDao(),
	}
}
