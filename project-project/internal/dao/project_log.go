package dao

import (
	"context"
	"test.com/project-project/internal/data"
	"test.com/project-project/internal/database/gorms"
)

type ProjectLogDao struct {
	conn *gorms.GormConn
}

func (p *ProjectLogDao) FindLogByMemberCode(ctx context.Context, memberId int64, page int64, size int64) (list []*data.ProjectLog, total int64, err error) {
	session := p.conn.Session(ctx)
	offset := (page - 1) * size
	err = session.Model(&data.ProjectLog{}).
		Where("member_code=?", memberId).
		Limit(int(size)).
		Offset(int(offset)).Order("create_time desc").Find(&list).Error
	if err != nil {
		return list, 0, err
	}
	err = session.Model(&data.ProjectLog{}).
		Where("member_code=?", memberId).Count(&total).Error
	return
}

func (p *ProjectLogDao) SaveProjectLog(ctx context.Context, pl *data.ProjectLog) error {
	session := p.conn.Session(ctx)
	return session.Save(pl).Error
}

func (p *ProjectLogDao) FindLogByTaskCode(ctx context.Context, taskCode int64, comment int) (list []*data.ProjectLog, total int64, err error) {
	session := p.conn.Session(ctx)
	model := session.Model(&data.ProjectLog{})
	if comment == 1 {
		model = model.Where("source_code=? and is_comment=?", taskCode, comment)
	} else {
		model = model.Where("source_code=?", taskCode)
	}
	if err = model.Order("create_time desc").Find(&list).Error; err != nil {
		return list, 0, err
	}
	if err = model.Count(&total).Error; err != nil {
		return list, 0, err
	}
	return
}

func (p *ProjectLogDao) FindLogByTaskCodePage(ctx context.Context, taskCode int64, comment int, page int, pageSize int) (list []*data.ProjectLog, total int64, err error) {
	session := p.conn.Session(ctx)
	model := session.Model(&data.ProjectLog{})
	offset := (page - 1) * pageSize
	if comment == 1 {
		model = model.Where("source_code=? and is_comment=?", taskCode, comment)
	} else {
		model = model.Where("source_code=?", taskCode)
	}
	if err = model.Order("create_time desc").Limit(pageSize).Offset(offset).Find(&list).Error; err != nil {
		return list, 0, err
	}
	if err = model.Count(&total).Error; err != nil {
		return list, 0, err
	}
	return
}

func NewProjectLogDao() *ProjectLogDao {
	return &ProjectLogDao{
		conn: gorms.New(),
	}
}
