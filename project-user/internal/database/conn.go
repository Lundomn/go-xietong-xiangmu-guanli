package database

type DbConn interface {
	Begin() error
	Rollback() error
	Commit() error
}
