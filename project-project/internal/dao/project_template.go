package dao

import (
	"context"
	"gorm.io/gorm"
	"test.com/project-project/internal/data"
	"test.com/project-project/internal/database/gorms"
)

type ProjectTemplateDao struct {
	conn *gorms.GormConn
}

func (p *ProjectTemplateDao) FindProjectTemplateById(ctx context.Context, id int) (*data.ProjectTemplate, error) {
	var template *data.ProjectTemplate
	err := p.conn.Session(ctx).Where("id=?", id).First(&template).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return template, err
}

func (p *ProjectTemplateDao) FindProjectTemplateSystem(ctx context.Context, page int64, size int64) (pts []data.ProjectTemplate, total int64, err error) {
	session := p.conn.Session(ctx)
	err = session.
		Model(&data.ProjectTemplate{}).
		Where("is_system=?", 1).
		Limit(int(size)).
		Offset(int((page - 1) * size)).
		Find(&pts).Error
	if err != nil {
		return pts, total, err
	}
	err = session.Model(&data.ProjectTemplate{}).Where("is_system=?", 1).Count(&total).Error
	return pts, total, err
}

func (p *ProjectTemplateDao) FindProjectTemplateCustom(ctx context.Context, memId int64, organizationCode int64, page int64, size int64) (pts []data.ProjectTemplate, total int64, err error) {
	session := p.conn.Session(ctx)
	err = session.
		Model(&data.ProjectTemplate{}).
		Where("is_system=? and member_code=? and organization_code=?", 0, memId, organizationCode).
		Limit(int(size)).
		Offset(int((page - 1) * size)).
		Find(&pts).Error
	if err != nil {
		return pts, total, err
	}
	err = session.Model(&data.ProjectTemplate{}).Where("is_system=? and member_code=? and organization_code=?", 0, memId, organizationCode).Count(&total).Error
	return pts, total, err
}

func (p *ProjectTemplateDao) FindProjectTemplateAll(ctx context.Context, organizationCode int64, page int64, size int64) (pts []data.ProjectTemplate, total int64, err error) {
	session := p.conn.Session(ctx)
	err = session.
		Model(&data.ProjectTemplate{}).
		// "all" is what the project creation page uses: it must include
		// system templates as well as templates owned by the current org.
		Where("is_system=? OR organization_code=?", 1, organizationCode).
		Limit(int(size)).
		Offset(int((page - 1) * size)).
		Find(&pts).Error
	if err != nil {
		return pts, total, err
	}
	err = session.Model(&data.ProjectTemplate{}).Where("is_system=? OR organization_code=?", 1, organizationCode).Count(&total).Error
	return pts, total, err
}

func NewProjectTemplateDao() *ProjectTemplateDao {
	return &ProjectTemplateDao{
		conn: gorms.New(),
	}
}
