package dao

import (
	"test.com/project-user/internal/database"
	"test.com/project-user/internal/database/gorms"
)

type TransactionImpl struct {
}

func (t *TransactionImpl) Action(f func(conn database.DbConn) error) error {
	conn := gorms.NewTran()
	if err := conn.Begin(); err != nil {
		return err
	}
	err := f(conn)
	if err != nil {
		_ = conn.Rollback()
		return err
	}
	if err = conn.Commit(); err != nil {
		_ = conn.Rollback()
		return err
	}
	return nil
}

func NewTransaction() *TransactionImpl {
	return &TransactionImpl{}
}
