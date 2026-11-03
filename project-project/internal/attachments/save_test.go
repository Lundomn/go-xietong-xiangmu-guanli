package attachments

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"test.com/project-project/internal/data"
)

func TestSaveKeepsFileAndTaskLinkInOneTransaction(t *testing.T) {
	for _, failure := range []string{"", "file", "link", "commit"} {
		t.Run(failure, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			conn, err := gorm.Open(mysql.New(mysql.Config{Conn: db, SkipInitializeWithVersion: true}),
				&gorm.Config{DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent)})
			if err != nil {
				t.Fatal(err)
			}
			writeErr := errors.New("database write rejected")
			mock.ExpectBegin()
			fileInsert := mock.ExpectExec("INSERT INTO `ms_file`")
			if failure == "file" {
				fileInsert.WillReturnError(writeErr)
				mock.ExpectRollback()
			} else {
				fileInsert.WillReturnResult(sqlmock.NewResult(41, 1))
				linkInsert := mock.ExpectExec("INSERT INTO `ms_source_link`")
				if failure == "link" {
					linkInsert.WillReturnError(writeErr)
					mock.ExpectRollback()
				} else {
					linkInsert.WillReturnResult(sqlmock.NewResult(61, 1))
					if failure == "commit" {
						// The server may have committed both rows. A lost response
						// must return uncertainty without a new compensating DELETE.
						mock.ExpectCommit().WillReturnError(io.ErrUnexpectedEOF)
					} else {
						mock.ExpectCommit()
					}
				}
			}
			file := &data.File{PathName: "upload/project/task/document.txt", TaskCode: 7, ProjectCode: 9, OrganizationCode: 11}
			link := &data.SourceLink{SourceType: "file", LinkType: "task", LinkCode: 7, OrganizationCode: 11}
			err = Save(context.Background(), conn, file, link)
			wantErr := error(nil)
			if failure == "file" || failure == "link" {
				wantErr = writeErr
			} else if failure == "commit" {
				wantErr = io.ErrUnexpectedEOF
			}
			if !errors.Is(err, wantErr) {
				t.Errorf("Save error = %v, want %v", err, wantErr)
			}
			if failure != "file" && (file.Id != 41 || link.SourceCode != file.Id) {
				t.Errorf("file and task link differ: file=%d source=%d", file.Id, link.SourceCode)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
