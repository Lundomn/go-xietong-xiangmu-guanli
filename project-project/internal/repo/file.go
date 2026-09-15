package repo

import (
	"context"
	"test.com/project-project/internal/data"
)

type FileRepo interface {
	Save(ctx context.Context, file *data.File) error
	Delete(ctx context.Context, id int64) error
	FindByIds(background context.Context, ids []int64) (list []*data.File, err error)
}
