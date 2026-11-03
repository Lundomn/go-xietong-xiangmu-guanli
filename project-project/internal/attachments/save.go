// Package attachments saves attachment metadata and its task association.
package attachments

import (
	"context"

	"gorm.io/gorm"
	"test.com/project-project/internal/data"
)

// Save commits both rows together. A commit error may hide a successful commit;
// return it without issuing an independent compensating delete.
func Save(ctx context.Context, db *gorm.DB, file *data.File, link *data.SourceLink) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(file).Error; err != nil {
			return err
		}
		link.SourceCode = file.Id
		return tx.Create(link).Error
	})
}
