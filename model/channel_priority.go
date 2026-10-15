package model

import (
	"crypto/sha256"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const channelOrderCoordinatorKey = "ChannelOrderCoordinator"

// lockChannelOrder must be the first database operation in a transaction that
// reads or changes channel membership, priority, or rebuilds abilities. Updating
// an existing options row locks it until commit on every supported engine,
// including SQLite, where SELECT FOR UPDATE would be ignored. The upsert also
// coordinates first use and the empty channel set without a schema migration.
func lockChannelOrder(tx *gorm.DB) error {
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&Option{Key: channelOrderCoordinatorKey, Value: ""}).Error; err != nil {
		return err
	}
	return tx.Model(&Option{}).Where(clause.Eq{Column: clause.Column{Name: "key"}, Value: channelOrderCoordinatorKey}).UpdateColumn("value", gorm.Expr("value")).Error
}

func channelOrderSnapshot(channels []*Channel) string {
	hash := sha256.New()
	for _, channel := range channels {
		fmt.Fprintf(hash, "%d:%d;", channel.Id, channel.GetPriority())
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

// GetChannelOrderPage returns rows, total and the full-set priority snapshot
// from one coordinated transaction. Secrets are never loaded for this view.
func GetChannelOrderPage(offset, limit int) (channels []*Channel, total int64, snapshot string, err error) {
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := lockChannelOrder(tx); err != nil {
			return err
		}
		var all []*Channel
		if err := tx.Order("priority DESC, id DESC").Omit("key").Find(&all).Error; err != nil {
			return err
		}
		total, snapshot = int64(len(all)), channelOrderSnapshot(all)
		start := min(max(offset, 0), len(all))
		end := min(start+max(limit, 0), len(all))
		channels = all[start:end]
		return nil
	})
	return
}
