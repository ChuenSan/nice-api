package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"github.com/QuantumNous/new-api/common"

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

var ErrChannelOrderConflict = errors.New("channel_order_conflict")
var ErrInvalidPriorityMove = errors.New("invalid_priority_move")

type ChannelPriorityMove struct {
	ExpectedSnapshot string `json:"expected_snapshot"`
	Page             int    `json:"page"`
	PageSize         int    `json:"page_size"`
	MovedID          int    `json:"moved_id"`
	TargetID         int    `json:"target_id"`
}

// withChannelOrder coordinates even callers supplying a plain DB handle rather
// than an active transaction. Callers with a transaction must acquire the lock
// before any channel reads, to avoid carrying a stale baseline into a rebuild.
func withChannelOrder(db *gorm.DB, write bool, fn func(*gorm.DB) error) error {
	if db == nil {
		db = DB
	}
	run := func(tx *gorm.DB) error {
		if err := lockChannelOrder(tx); err != nil {
			return err
		}
		if err := fn(tx); err != nil {
			return err
		}
		if !write {
			return nil
		}
		return tx.Model(&Option{}).Where(clause.Eq{Column: clause.Column{Name: "key"}, Value: channelOrderCoordinatorKey}).UpdateColumn("value", common.GetUUID()).Error
	}
	if _, ok := db.Statement.ConnPool.(gorm.TxCommitter); ok {
		return run(db)
	}
	return db.Transaction(run)
}

func ReorderChannelPriority(move ChannelPriorityMove) (string, error) {
	if move.ExpectedSnapshot == "" || move.Page < 1 || move.PageSize < 1 || move.PageSize > 100 || move.MovedID <= 0 || move.TargetID <= 0 || move.MovedID == move.TargetID {
		return "", ErrInvalidPriorityMove
	}
	snapshot := ""
	err := withChannelOrder(nil, true, func(tx *gorm.DB) error {
		var channels []*Channel
		if err := tx.Select("id", "priority").Order("priority DESC, id DESC").Find(&channels).Error; err != nil {
			return err
		}
		if channelOrderSnapshot(channels) != move.ExpectedSnapshot {
			return ErrChannelOrderConflict
		}
		// Bound page before multiplication to reject overflow as well as empty pages.
		if len(channels) < 2 || move.Page > (len(channels)-1)/move.PageSize+1 {
			return ErrInvalidPriorityMove
		}
		start := (move.Page - 1) * move.PageSize
		end := min(start+move.PageSize, len(channels))
		from, to := -1, -1
		for i := start; i < end; i++ {
			if channels[i].Id == move.MovedID {
				from = i
			}
			if channels[i].Id == move.TargetID {
				to = i
			}
		}
		if from < 0 || to < 0 {
			return ErrInvalidPriorityMove
		}
		moved := channels[from]
		if from < to {
			copy(channels[from:to], channels[from+1:to+1])
		} else {
			copy(channels[to+1:from+1], channels[to:from])
		}
		channels[to] = moved
		for i, channel := range channels {
			priority := int64(len(channels) - i)
			if err := tx.Model(&Channel{}).Where("id = ?", channel.Id).UpdateColumn("priority", priority).Error; err != nil {
				return err
			}
			if err := tx.Model(&Ability{}).Where("channel_id = ?", channel.Id).UpdateColumn("priority", priority).Error; err != nil {
				return err
			}
			channel.Priority = &priority
		}
		snapshot = channelOrderSnapshot(channels)
		return nil
	})
	return snapshot, err
}
