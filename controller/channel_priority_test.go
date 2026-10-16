package controller

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func priorityDatabase(t *testing.T, kind, dsn string) *gorm.DB {
	t.Helper()
	database, isolatedDSN := newAuditTestDatabase(t, kind, dsn)
	previousDB := model.DB
	previousType := common.MainDatabaseType()
	previousMaster, previousCache, previousRedis, previousSQLite := common.IsMasterNode, common.MemoryCacheEnabled, common.RedisEnabled, common.SQLitePath
	if kind == "sqlite" {
		common.SQLitePath = isolatedDSN
		isolatedDSN = "local"
	}
	t.Setenv("SQL_DSN", isolatedDSN)
	t.Setenv("LOG_SQL_DSN", "")
	common.IsMasterNode, common.MemoryCacheEnabled, common.RedisEnabled = false, false, false
	require.NoError(t, model.InitDB())
	database = model.DB
	require.NoError(t, database.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.Option{}))
	var version string
	query := "SELECT version()"
	if kind == "sqlite" {
		query = "SELECT sqlite_version()"
	}
	require.NoError(t, database.Raw(query).Scan(&version).Error)
	t.Logf("database=%s version=%s", kind, version)
	t.Cleanup(func() {
		common.MemoryCacheEnabled = false
		model.InitChannelCache()
		connection, err := database.DB()
		require.NoError(t, err)
		require.NoError(t, connection.Close())
		model.DB = previousDB
		common.SetMainDatabaseType(previousType)
		common.IsMasterNode, common.MemoryCacheEnabled, common.RedisEnabled, common.SQLitePath = previousMaster, previousCache, previousRedis, previousSQLite
	})
	return database
}

type priorityResponse struct {
	Success bool   `json:"success"`
	Code    string `json:"code"`
	Data    struct {
		Items    []model.Channel `json:"items"`
		Total    int             `json:"total"`
		Snapshot string          `json:"order_snapshot"`
	} `json:"data"`
}

func priorityRequest(t *testing.T, handler gin.HandlerFunc, method, path string, body any) (int, priorityResponse) {
	t.Helper()
	encoded, err := common.Marshal(body)
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(method, path, bytes.NewReader(encoded))
	context.Set("id", 1)
	context.Set("role", common.RoleRootUser)
	context.Request.Header.Set("Content-Type", "application/json")
	handler(context)
	var response priorityResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response), recorder.Body.String())
	return recorder.Code, response
}

// A missing database must fail channel selection closed: the memory cache may
// not silently serve priorities whose committed order cannot be verified.
func TestChannelPrioritySelectionFailsClosedWithoutDatabase(t *testing.T) {
	previousDB := model.DB
	previousCache := common.MemoryCacheEnabled
	model.DB = nil
	common.MemoryCacheEnabled = true
	t.Cleanup(func() {
		model.DB = previousDB
		common.MemoryCacheEnabled = previousCache
	})

	channel, err := model.GetRandomSatisfiedChannel("default", "test-model", 0, nil)
	require.Error(t, err)
	require.Nil(t, channel)
	assert.Contains(t, err.Error(), "database")
}

func TestChannelPriorityList(t *testing.T) {
	for _, dialect := range []struct{ kind, env string }{{"sqlite", ""}, {"mysql", "TEST_MYSQL_DSN"}, {"postgres", "TEST_POSTGRES_DSN"}} {
		t.Run(dialect.kind, func(t *testing.T) {
			dsn := os.Getenv(dialect.env)
			if dialect.env != "" && dsn == "" {
				t.Skip("disposable database DSN not configured")
			}
			priorityDatabase(t, dialect.kind, dsn)
			for _, id := range []int{1, 2, 3, 4, 5, 6} {
				priority := int64(10)
				if id == 1 {
					priority = 0
				}
				channel := model.Channel{Id: id, Key: "secret", Name: "priority", Models: "test-model", Group: "default", Priority: &priority}
				require.NoError(t, channel.Insert())
			}
			status, response := priorityRequest(t, GetAllChannels, http.MethodGet, "/api/channel/?p=2&page_size=3", nil)
			require.Equal(t, http.StatusOK, status)
			require.True(t, response.Success)
			require.NotEmpty(t, response.Data.Snapshot)
			assert.Equal(t, 6, response.Data.Total)
			ids := []int{}
			for _, channel := range response.Data.Items {
				ids = append(ids, channel.Id)
				assert.Empty(t, channel.Key)
			}
			assert.Equal(t, []int{3, 2, 1}, ids)
			_, explicit := priorityRequest(t, GetAllChannels, http.MethodGet, "/api/channel/?sort_by=priority&sort_order=desc", nil)
			assert.Equal(t, response.Data.Snapshot, explicit.Data.Snapshot)
			for _, query := range []string{"group=default", "type=1", "tag_mode=true", "sort_by=id", "sort_by=priority&sort_order=asc", "id_sort=true", "keyword=priority"} {
				_, filtered := priorityRequest(t, GetAllChannels, http.MethodGet, "/api/channel/?"+query, nil)
				assert.Empty(t, filtered.Data.Snapshot, query)
			}
		})
	}
}

func TestChannelPriorityReorder(t *testing.T) {
	for _, dialect := range []struct{ kind, env string }{{"sqlite", ""}, {"mysql", "TEST_MYSQL_DSN"}, {"postgres", "TEST_POSTGRES_DSN"}} {
		t.Run(dialect.kind, func(t *testing.T) {
			dsn := os.Getenv(dialect.env)
			if dialect.env != "" && dsn == "" {
				t.Skip("disposable database DSN not configured")
			}
			database := priorityDatabase(t, dialect.kind, dsn)
			before := make(map[int]model.Channel)
			for _, id := range []int{1, 2, 3, 4, 5, 6} {
				priority, weight, tag := int64(10), uint(id*7), "batch"
				if id == 6 {
					priority = 90
				}
				channel := model.Channel{Id: id, Key: "secret", Name: "priority", Models: "test-model", Group: "default", Status: common.ChannelStatusEnabled, Priority: &priority, Weight: &weight, Tag: &tag}
				if id == 3 {
					channel.Status = common.ChannelStatusManuallyDisabled
				}
				require.NoError(t, channel.Insert())
				before[id] = channel
			}
			common.MemoryCacheEnabled = true
			model.InitChannelCache()
			selected, err := model.GetRandomSatisfiedChannel("default", "test-model", 0, nil)
			require.NoError(t, err)
			require.NotNil(t, selected)
			assert.Equal(t, 6, selected.Id)
			_, baseline := priorityRequest(t, GetAllChannels, http.MethodGet, "/api/channel/?p=2&page_size=3", nil)
			move := model.ChannelPriorityMove{ExpectedSnapshot: baseline.Data.Snapshot, Page: 2, PageSize: 3, MovedID: 1, TargetID: 3}
			status, saved := priorityRequest(t, ReorderChannelPriority, http.MethodPost, "/api/channel/priority/reorder", move)
			require.Equal(t, http.StatusOK, status)
			require.True(t, saved.Success)
			assert.NotEqual(t, baseline.Data.Snapshot, saved.Data.Snapshot)
			_, refreshed := priorityRequest(t, GetAllChannels, http.MethodGet, "/api/channel/?page_size=100", nil)
			assert.Equal(t, saved.Data.Snapshot, refreshed.Data.Snapshot)
			ids := []int{}
			for _, channel := range refreshed.Data.Items {
				ids = append(ids, channel.Id)
			}
			assert.Equal(t, []int{6, 5, 4, 1, 3, 2}, ids)
			var channels []model.Channel
			require.NoError(t, database.Order("priority DESC, id DESC").Find(&channels).Error)
			for i, channel := range channels {
				assert.Equal(t, int64(6-i), channel.GetPriority())
				original := before[channel.Id]
				channel.Priority, original.Priority = nil, nil
				assert.Equal(t, original, channel, "only priority changes for channel %d", channel.Id)
			}
			var abilities []model.Ability
			require.NoError(t, database.Order("priority DESC, channel_id DESC").Find(&abilities).Error)
			require.Len(t, abilities, 6)
			for i, ability := range abilities {
				assert.Equal(t, ids[i], ability.ChannelId)
				require.NotNil(t, ability.Priority)
				assert.Equal(t, int64(6-i), *ability.Priority)
			}
			status, conflict := priorityRequest(t, ReorderChannelPriority, http.MethodPost, "/api/channel/priority/reorder", move)
			assert.Equal(t, http.StatusConflict, status)
			assert.Equal(t, "channel_order_conflict", conflict.Code)
			move = model.ChannelPriorityMove{ExpectedSnapshot: saved.Data.Snapshot, Page: 1, PageSize: 3, MovedID: 4, TargetID: 6}
			status, _ = priorityRequest(t, ReorderChannelPriority, http.MethodPost, "/api/channel/priority/reorder", move)
			require.Equal(t, http.StatusOK, status)
			for _, cached := range []bool{true, false} {
				common.MemoryCacheEnabled = cached
				selected, err = model.GetRandomSatisfiedChannel("default", "test-model", 0, nil)
				require.NoError(t, err)
				require.NotNil(t, selected)
				assert.Equal(t, 4, selected.Id, "routing cache=%v", cached)
			}
		})
	}
}

func TestChannelPriorityStatusScope(t *testing.T) {
	for _, dialect := range []struct{ kind, env string }{{"sqlite", ""}, {"mysql", "TEST_MYSQL_DSN"}, {"postgres", "TEST_POSTGRES_DSN"}} {
		t.Run(dialect.kind, func(t *testing.T) {
			dsn := os.Getenv(dialect.env)
			if dialect.env != "" && dsn == "" {
				t.Skip("disposable database DSN not configured")
			}
			database := priorityDatabase(t, dialect.kind, dsn)
			for _, test := range []struct {
				status string
				ids    []int
				hidden []int
			}{
				{"enabled", []int{8, 6, 1, 4}, []int{2, 3, 5, 7}},
				{"disabled", []int{7, 5, 2, 3}, []int{1, 4, 6, 8}},
			} {
				t.Run(test.status, func(t *testing.T) {
					require.NoError(t, database.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&model.Ability{}).Error)
					require.NoError(t, database.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&model.Channel{}).Error)
					for i, status := range []int{1, 2, 3, 1, 2, 1, 3, 1} {
						priority := int64(10)
						channel := model.Channel{Id: i + 1, Name: "scoped", Key: "secret", Models: "test-model", Group: "default", Status: status, Priority: &priority}
						require.NoError(t, channel.Insert())
					}
					path := "/api/channel/?status=" + test.status
					status, baseline := priorityRequest(t, GetAllChannels, http.MethodGet, path+"&p=2&page_size=2", nil)
					require.Equal(t, http.StatusOK, status)
					require.True(t, baseline.Success)
					require.NotEmpty(t, baseline.Data.Snapshot)
					assert.Equal(t, 4, baseline.Data.Total)
					require.Len(t, baseline.Data.Items, 2)
					move := map[string]any{"status": test.status, "expected_snapshot": baseline.Data.Snapshot, "page": 2, "page_size": 2, "moved_id": test.ids[2], "target_id": test.ids[3]}
					for _, invalid := range []struct {
						name, key string
						value     any
					}{
						{"hidden target", "target_id", test.hidden[0]},
						{"other page", "target_id", test.ids[0]},
						{"invalid scope", "status", "unknown"},
					} {
						t.Run(invalid.name, func(t *testing.T) {
							original := move[invalid.key]
							move[invalid.key] = invalid.value
							status, rejected := priorityRequest(t, ReorderChannelPriority, http.MethodPost, "/api/channel/priority/reorder", move)
							assert.Equal(t, http.StatusBadRequest, status)
							assert.Equal(t, "invalid_priority_move", rejected.Code)
							move[invalid.key] = original
							_, unchanged := priorityRequest(t, GetAllChannels, http.MethodGet, path+"&p=2&page_size=2", nil)
							assert.Equal(t, baseline, unchanged)
						})
					}
					status, saved := priorityRequest(t, ReorderChannelPriority, http.MethodPost, "/api/channel/priority/reorder", move)
					require.Equal(t, http.StatusOK, status)
					require.True(t, saved.Success)
					_, refreshed := priorityRequest(t, GetAllChannels, http.MethodGet, path+"&page_size=100", nil)
					assert.Equal(t, saved.Data.Snapshot, refreshed.Data.Snapshot)
					ids := make([]int, 0, len(refreshed.Data.Items))
					for i, channel := range refreshed.Data.Items {
						ids = append(ids, channel.Id)
						assert.Equal(t, int64(4-i), channel.GetPriority())
						assert.Empty(t, channel.Key)
					}
					assert.Equal(t, test.ids, ids)
					for _, id := range test.hidden {
						channel, err := model.GetChannelById(id, true)
						require.NoError(t, err)
						assert.Equal(t, int64(10), channel.GetPriority(), "hidden channel %d", id)
					}
					var abilities []model.Ability
					require.NoError(t, database.Find(&abilities).Error)
					require.Len(t, abilities, 8)
					for _, ability := range abilities {
						channel, err := model.GetChannelById(ability.ChannelId, true)
						require.NoError(t, err)
						assert.Equal(t, channel.Priority, ability.Priority)
					}
					status, stale := priorityRequest(t, ReorderChannelPriority, http.MethodPost, "/api/channel/priority/reorder", move)
					assert.Equal(t, http.StatusConflict, status)
					assert.Equal(t, "channel_order_conflict", stale.Code)
					// A status change must invalidate this subset's snapshot even when priority is unchanged.
					nextStatus := common.ChannelStatusEnabled
					if test.status == "enabled" {
						nextStatus = common.ChannelStatusManuallyDisabled
					}
					require.NoError(t, (&model.Channel{Id: test.ids[0], Status: nextStatus}).Update())
					move["expected_snapshot"] = saved.Data.Snapshot
					status, changed := priorityRequest(t, ReorderChannelPriority, http.MethodPost, "/api/channel/priority/reorder", move)
					assert.Equal(t, http.StatusConflict, status)
					assert.Equal(t, "channel_order_conflict", changed.Code)
				})
			}
		})
	}
}

func TestChannelPriorityStaleAbilityRebuild(t *testing.T) {
	for _, dialect := range []struct{ kind, env string }{{"sqlite", ""}, {"mysql", "TEST_MYSQL_DSN"}, {"postgres", "TEST_POSTGRES_DSN"}} {
		t.Run(dialect.kind, func(t *testing.T) {
			dsn := os.Getenv(dialect.env)
			if dialect.env != "" && dsn == "" {
				t.Skip("disposable database DSN not configured")
			}
			database := priorityDatabase(t, dialect.kind, dsn)
			firstPriority, secondPriority := int64(90), int64(10)
			first := model.Channel{Id: 1, Key: "secret", Models: "test-model", Group: "default", Status: common.ChannelStatusEnabled, Priority: &firstPriority, Setting: common.GetPointer("invalid"), OtherSettings: "invalid"}
			second := model.Channel{Id: 2, Key: "secret", Models: "test-model", Group: "default", Status: common.ChannelStatusEnabled, Priority: &secondPriority}
			require.NoError(t, first.Insert())
			require.NoError(t, second.Insert())
			_, baseline := priorityRequest(t, GetAllChannels, http.MethodGet, "/api/channel/", nil)
			status, _ := priorityRequest(t, ReorderChannelPriority, http.MethodPost, "/api/channel/priority/reorder", model.ChannelPriorityMove{ExpectedSnapshot: baseline.Data.Snapshot, Page: 1, PageSize: 10, MovedID: 2, TargetID: 1})
			require.Equal(t, http.StatusOK, status)
			// Recovery from malformed settings on a stale receiver must not restore
			// its old priority or overwrite an unrelated edit made after it loaded.
			require.NoError(t, (&model.Channel{Id: 1, Name: "updated"}).Update())
			first.GetSetting()
			first.GetOtherSettings()
			repaired, err := model.GetChannelById(1, true)
			require.NoError(t, err)
			assert.Nil(t, repaired.Setting)
			assert.Equal(t, "{}", repaired.OtherSettings)
			assert.Equal(t, "updated", repaired.Name)
			assert.EqualValues(t, 1, repaired.GetPriority())
			// Background/upstream callers may retain a channel loaded before reorder.
			require.NoError(t, first.UpdateAbilities(nil))
			selected, err := model.GetRandomSatisfiedChannel("default", "test-model", 0, nil)
			require.NoError(t, err)
			require.NotNil(t, selected)
			assert.Equal(t, 2, selected.Id)
			var ability model.Ability
			require.NoError(t, database.Where("channel_id = ?", 1).First(&ability).Error)
			require.NotNil(t, ability.Priority)
			assert.EqualValues(t, 1, *ability.Priority)
			count, failures, err := model.FixAbility()
			require.NoError(t, err)
			assert.Equal(t, 2, count)
			assert.Zero(t, failures)
			selected, err = model.GetRandomSatisfiedChannel("default", "test-model", 0, nil)
			require.NoError(t, err)
			require.NotNil(t, selected)
			assert.Equal(t, 2, selected.Id)
		})
	}
}

func TestChannelPriorityValidationAndWriterConflicts(t *testing.T) {
	for _, dialect := range []struct{ kind, env string }{{"sqlite", ""}, {"mysql", "TEST_MYSQL_DSN"}, {"postgres", "TEST_POSTGRES_DSN"}} {
		t.Run(dialect.kind, func(t *testing.T) {
			dsn := os.Getenv(dialect.env)
			if dialect.env != "" && dsn == "" {
				t.Skip("disposable database DSN not configured")
			}
			database := priorityDatabase(t, dialect.kind, dsn)
			newChannel := func(id int) model.Channel {
				priority, weight, tag := int64(40-id), uint(7), "batch"
				return model.Channel{Id: id, Name: "original", Key: "secret", Models: "test-model", Group: "default", Status: common.ChannelStatusEnabled, Priority: &priority, Weight: &weight, Tag: &tag}
			}
			read := func() priorityResponse {
				status, response := priorityRequest(t, GetAllChannels, http.MethodGet, "/api/channel/?page_size=100", nil)
				require.Equal(t, http.StatusOK, status)
				require.True(t, response.Success)
				return response
			}
			for _, id := range []int{1, 2, 3} {
				channel := newChannel(id)
				require.NoError(t, channel.Insert())
			}
			baseline := read()
			valid := model.ChannelPriorityMove{ExpectedSnapshot: baseline.Data.Snapshot, Page: 1, PageSize: 2, MovedID: 1, TargetID: 2}
			for _, test := range []struct {
				name   string
				change func(*model.ChannelPriorityMove)
			}{
				{"missing snapshot", func(m *model.ChannelPriorityMove) { m.ExpectedSnapshot = "" }},
				{"same channel", func(m *model.ChannelPriorityMove) { m.TargetID = m.MovedID }},
				{"missing channel", func(m *model.ChannelPriorityMove) { m.TargetID = 99 }},
				{"outside page", func(m *model.ChannelPriorityMove) { m.TargetID = 3 }},
				{"zero id", func(m *model.ChannelPriorityMove) { m.MovedID = 0 }},
				{"negative id", func(m *model.ChannelPriorityMove) { m.TargetID = -1 }},
				{"zero page", func(m *model.ChannelPriorityMove) { m.Page = 0 }},
				{"empty page", func(m *model.ChannelPriorityMove) { m.Page = 4 }},
				{"overflow page", func(m *model.ChannelPriorityMove) { m.Page = int(^uint(0) >> 1) }},
				{"zero size", func(m *model.ChannelPriorityMove) { m.PageSize = 0 }},
				{"large size", func(m *model.ChannelPriorityMove) { m.PageSize = 101 }},
			} {
				t.Run(test.name, func(t *testing.T) {
					move := valid
					test.change(&move)
					status, response := priorityRequest(t, ReorderChannelPriority, http.MethodPost, "/api/channel/priority/reorder", move)
					assert.Equal(t, http.StatusBadRequest, status)
					assert.Equal(t, "invalid_priority_move", response.Code)
					assert.Equal(t, baseline, read())
				})
			}
			status, response := priorityRequest(t, ReorderChannelPriority, http.MethodPost, "/api/channel/priority/reorder", map[string]any{"page": "invalid"})
			assert.Equal(t, http.StatusBadRequest, status)
			assert.Equal(t, "invalid_priority_move", response.Code)
			// Snapshot identity excludes all non-priority channel metadata.
			weight, tag := uint(55), "renamed"
			edit := model.Channel{Id: 1, Name: "changed", Models: "other-model", Group: "other-group", Weight: &weight, Tag: &tag, Status: common.ChannelStatusManuallyDisabled}
			require.NoError(t, edit.Update())
			assert.Equal(t, baseline.Data.Snapshot, read().Data.Snapshot)
			status, _ = priorityRequest(t, ReorderChannelPriority, http.MethodPost, "/api/channel/priority/reorder", valid)
			require.Equal(t, http.StatusOK, status)
			stored, err := model.GetChannelById(1, true)
			require.NoError(t, err)
			assert.Equal(t, "changed", stored.Name)
			assert.Equal(t, "other-model", stored.Models)
			assert.Equal(t, "other-group", stored.Group)
			assert.Equal(t, &weight, stored.Weight)
			assert.Equal(t, &tag, stored.Tag)
			for _, test := range []struct {
				name  string
				write func() error
			}{
				{"single priority", func() error { p := int64(80); return (&model.Channel{Id: 1, Priority: &p}).Update() }},
				{"full save", func() error {
					c, err := model.GetChannelById(1, true)
					if err != nil {
						return err
					}
					p := int64(80)
					c.Priority = &p
					return c.Save()
				}},
				{"tag priority and rename", func() error {
					p, next := int64(80), "renamed"
					return model.EditChannelByTag("batch", &next, nil, nil, nil, &p, nil, nil, nil)
				}},
				{"ability tag priority", func() error { p := int64(80); return model.UpdateAbilityByTag("batch", nil, &p, nil) }},
				{"insert", func() error { c := newChannel(4); return c.Insert() }},
				{"batch insert", func() error { return model.BatchInsertChannels([]model.Channel{newChannel(4), newChannel(5)}) }},
				{"delete", func() error { return (&model.Channel{Id: 1}).Delete() }},
				{"batch delete", func() error { _, err := model.BatchDeleteChannels([]int{1, 2}); return err }},
				{"delete status", func() error { _, err := model.DeleteChannelByStatus(common.ChannelStatusManuallyDisabled); return err }},
				{"delete disabled", func() error { _, err := model.DeleteDisabledChannel(); return err }},
			} {
				t.Run(test.name, func(t *testing.T) {
					require.NoError(t, database.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&model.Ability{}).Error)
					require.NoError(t, database.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&model.Channel{}).Error)
					for _, id := range []int{1, 2, 3} {
						c := newChannel(id)
						if id == 3 {
							c.Status = common.ChannelStatusManuallyDisabled
						}
						require.NoError(t, c.Insert())
					}
					before := read()
					require.NoError(t, test.write())
					after := read()
					require.NotEqual(t, before.Data.Snapshot, after.Data.Snapshot)
					move := valid
					move.ExpectedSnapshot = before.Data.Snapshot
					status, response := priorityRequest(t, ReorderChannelPriority, http.MethodPost, "/api/channel/priority/reorder", move)
					assert.Equal(t, http.StatusConflict, status)
					assert.Equal(t, "channel_order_conflict", response.Code)
					assert.Equal(t, after, read(), "conflict must preserve the winning write")
					var abilities []model.Ability
					require.NoError(t, database.Find(&abilities).Error)
					assert.Len(t, abilities, after.Data.Total)
					for _, ability := range abilities {
						channel, err := model.GetChannelById(ability.ChannelId, true)
						require.NoError(t, err)
						assert.Equal(t, channel.Priority, ability.Priority)
						assert.Equal(t, channel.Tag, ability.Tag)
					}
				})
			}
			for _, count := range []int{0, 1} {
				require.NoError(t, database.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&model.Ability{}).Error)
				require.NoError(t, database.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&model.Channel{}).Error)
				if count == 1 {
					c := newChannel(1)
					require.NoError(t, c.Insert())
				}
				before := read()
				move := valid
				move.ExpectedSnapshot = before.Data.Snapshot
				status, _ := priorityRequest(t, ReorderChannelPriority, http.MethodPost, "/api/channel/priority/reorder", move)
				assert.Equal(t, http.StatusBadRequest, status)
				assert.Equal(t, before, read())
			}
		})
	}
}

func TestChannelPriorityRollback(t *testing.T) {
	for _, dialect := range []struct{ kind, env string }{{"sqlite", ""}, {"mysql", "TEST_MYSQL_DSN"}, {"postgres", "TEST_POSTGRES_DSN"}} {
		t.Run(dialect.kind, func(t *testing.T) {
			dsn := os.Getenv(dialect.env)
			if dialect.env != "" && dsn == "" {
				t.Skip("disposable database DSN not configured")
			}
			database := priorityDatabase(t, dialect.kind, dsn)
			for _, id := range []int{1, 2, 3} {
				p := int64(40 - id)
				c := model.Channel{Id: id, Key: "secret", Models: "test-model", Group: "default", Status: common.ChannelStatusEnabled, Priority: &p}
				require.NoError(t, c.Insert())
			}
			_, before := priorityRequest(t, GetAllChannels, http.MethodGet, "/api/channel/", nil)
			var channels []model.Channel
			var abilities []model.Ability
			require.NoError(t, database.Order("id").Find(&channels).Error)
			require.NoError(t, database.Order("channel_id").Find(&abilities).Error)
			updates := 0
			require.NoError(t, database.Callback().Update().Before("gorm:update").Register("fail_priority_ability_write", func(tx *gorm.DB) {
				if tx.Statement.Table == "abilities" {
					updates++
					if updates == 2 {
						tx.AddError(errors.New("injected ability failure"))
					}
				}
			}))
			status, response := priorityRequest(t, ReorderChannelPriority, http.MethodPost, "/api/channel/priority/reorder", model.ChannelPriorityMove{ExpectedSnapshot: before.Data.Snapshot, Page: 1, PageSize: 3, MovedID: 3, TargetID: 1})
			require.NoError(t, database.Callback().Update().Remove("fail_priority_ability_write"))
			assert.Equal(t, http.StatusInternalServerError, status)
			assert.Equal(t, "channel_order_failed", response.Code)
			_, after := priorityRequest(t, GetAllChannels, http.MethodGet, "/api/channel/", nil)
			assert.Equal(t, before, after)
			var actualChannels []model.Channel
			var actualAbilities []model.Ability
			require.NoError(t, database.Order("id").Find(&actualChannels).Error)
			require.NoError(t, database.Order("channel_id").Find(&actualAbilities).Error)
			assert.Equal(t, channels, actualChannels)
			assert.Equal(t, abilities, actualAbilities)
			// An interrupted ability repair must keep the previous complete table.
			require.NoError(t, database.Callback().Create().Before("gorm:create").Register("fail_priority_rebuild", func(tx *gorm.DB) {
				if tx.Statement.Table == "abilities" {
					tx.AddError(errors.New("injected rebuild failure"))
				}
			}))
			_, _, err := model.FixAbility()
			require.NoError(t, database.Callback().Create().Remove("fail_priority_rebuild"))
			require.Error(t, err)
			require.NoError(t, database.Order("channel_id").Find(&actualAbilities).Error)
			assert.Equal(t, abilities, actualAbilities)
			selected, err := model.GetRandomSatisfiedChannel("default", "test-model", 0, nil)
			require.NoError(t, err)
			require.NotNil(t, selected)
			assert.Equal(t, 1, selected.Id)
		})
	}
}

// The helper runs an independent gateway process against the same disposable DB.
// Its warmed routing cache cannot be refreshed by the parent's process globals.
type priorityProcessCommand struct {
	Operation string                    `json:"operation"`
	Move      model.ChannelPriorityMove `json:"move"`
}

type priorityProcessResult struct {
	Event    string           `json:"event"`
	Status   int              `json:"status"`
	Response priorityResponse `json:"response"`
	Selected int              `json:"selected"`
	Error    string           `json:"error"`
}

func TestChannelPriorityProcess(t *testing.T) {
	if os.Getenv("CHANNEL_PRIORITY_PROCESS") != "1" {
		t.Skip("subprocess helper")
	}
	common.IsMasterNode, common.MemoryCacheEnabled, common.RedisEnabled = false, true, false
	common.SQLitePath = os.Getenv("CHANNEL_PRIORITY_SQLITE_PATH")
	require.NoError(t, model.InitDB())
	model.InitChannelCache()
	emit := func(result priorityProcessResult) {
		encoded, err := common.Marshal(result)
		require.NoError(t, err)
		fmt.Println("@priority:" + string(encoded))
	}
	// Signal the first SQL write attempt so the parent can hold a competing
	// transaction across the exact point where the child starts its mutation.
	announce := false
	beforeWrite := func(tx *gorm.DB) {
		if announce {
			announce = false
			emit(priorityProcessResult{Event: "attempt"})
		}
	}
	require.NoError(t, model.DB.Callback().Create().Before("gorm:create").Register("priority_process_write", beforeWrite))
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register("priority_process_write", beforeWrite))
	require.NoError(t, model.DB.Callback().Delete().Before("gorm:delete").Register("priority_process_write", beforeWrite))
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var command priorityProcessCommand
		require.NoError(t, common.Unmarshal(scanner.Bytes(), &command))
		result := priorityProcessResult{Event: "result"}
		switch command.Operation {
		case "select":
			channel, err := model.GetRandomSatisfiedChannel("default", "test-model", 0, nil)
			if err != nil {
				result.Error = err.Error()
			} else if channel != nil {
				result.Selected = channel.Id
			}
		case "move":
			announce = true
			result.Status, result.Response = priorityRequest(t, ReorderChannelPriority, http.MethodPost, "/api/channel/priority/reorder", command.Move)
		case "update":
			announce = true
			priority := int64(80)
			if err := (&model.Channel{Id: 1, Priority: &priority}).Update(); err != nil {
				result.Error = err.Error()
			}
		default:
			t.Fatalf("unknown operation %q", command.Operation)
		}
		emit(result)
	}
	require.NoError(t, scanner.Err())
}

func TestChannelPriorityMultiProcess(t *testing.T) {
	for _, dialect := range []struct{ kind, env string }{{"sqlite", ""}, {"mysql", "TEST_MYSQL_DSN"}, {"postgres", "TEST_POSTGRES_DSN"}} {
		t.Run(dialect.kind, func(t *testing.T) {
			dsn := os.Getenv(dialect.env)
			if dialect.env != "" && dsn == "" {
				t.Skip("disposable database DSN not configured")
			}
			database := priorityDatabase(t, dialect.kind, dsn)
			for _, id := range []int{1, 2, 3} {
				priority := int64(40 - id)
				channel := model.Channel{Id: id, Key: "secret", Models: "test-model", Group: "default", Status: common.ChannelStatusEnabled, Priority: &priority}
				require.NoError(t, channel.Insert())
			}
			common.MemoryCacheEnabled = true
			model.InitChannelCache()
			processContext, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)
			child := exec.CommandContext(processContext, os.Args[0], "-test.run=^TestChannelPriorityProcess$")
			child.Env = append(os.Environ(), "CHANNEL_PRIORITY_PROCESS=1", "CHANNEL_PRIORITY_SQLITE_PATH="+common.SQLitePath)
			input, err := child.StdinPipe()
			require.NoError(t, err)
			output, err := child.StdoutPipe()
			require.NoError(t, err)
			child.Stderr = os.Stderr
			require.NoError(t, child.Start())
			t.Cleanup(func() { require.NoError(t, input.Close()); require.NoError(t, child.Wait()) })
			scanner := bufio.NewScanner(output)
			send := func(command priorityProcessCommand) {
				encoded, err := common.Marshal(command)
				require.NoError(t, err)
				_, err = fmt.Fprintln(input, string(encoded))
				require.NoError(t, err)
			}
			receive := func() priorityProcessResult {
				for scanner.Scan() {
					if data, ok := strings.CutPrefix(scanner.Text(), "@priority:"); ok {
						var result priorityProcessResult
						require.NoError(t, common.Unmarshal([]byte(data), &result))
						return result
					}
				}
				t.Fatalf("child process ended before result: %v", scanner.Err())
				return priorityProcessResult{}
			}
			send(priorityProcessCommand{Operation: "select"})
			assert.Equal(t, 1, receive().Selected)
			_, before := priorityRequest(t, GetAllChannels, http.MethodGet, "/api/channel/", nil)
			move := model.ChannelPriorityMove{ExpectedSnapshot: before.Data.Snapshot, Page: 1, PageSize: 3, MovedID: 3, TargetID: 1}
			status, saved := priorityRequest(t, ReorderChannelPriority, http.MethodPost, "/api/channel/priority/reorder", move)
			require.Equal(t, http.StatusOK, status)
			send(priorityProcessCommand{Operation: "select"})
			selected := receive()
			require.Empty(t, selected.Error)
			assert.Equal(t, 3, selected.Selected, "another process must invalidate its warmed cache immediately")
			move.ExpectedSnapshot, move.MovedID, move.TargetID = saved.Data.Snapshot, 2, 3
			send(priorityProcessCommand{Operation: "move", Move: move})
			require.Equal(t, "attempt", receive().Event)
			remote := receive()
			require.Equal(t, http.StatusOK, remote.Status)
			channel, err := model.GetRandomSatisfiedChannel("default", "test-model", 0, nil)
			require.NoError(t, err)
			require.NotNil(t, channel)
			assert.Equal(t, 2, channel.Id, "parent cache must also observe child commits")
			for _, operation := range []string{"move", "update"} {
				t.Run(operation, func(t *testing.T) {
					_, baseline := priorityRequest(t, GetAllChannels, http.MethodGet, "/api/channel/", nil)
					move := model.ChannelPriorityMove{ExpectedSnapshot: baseline.Data.Snapshot, Page: 1, PageSize: 3, MovedID: 1, TargetID: 2}
					baselineRead, release := make(chan struct{}), make(chan struct{})
					var paused atomic.Bool
					require.NoError(t, database.Callback().Query().After("gorm:query").Register("hold_priority_snapshot", func(tx *gorm.DB) {
						if tx.Statement.Table == "channels" && paused.CompareAndSwap(false, true) {
							close(baselineRead)
							<-release
						}
					}))
					result := make(chan error, 1)
					go func() { _, err := model.ReorderChannelPriority(move); result <- err }()
					<-baselineRead
					send(priorityProcessCommand{Operation: operation, Move: move})
					require.Equal(t, "attempt", receive().Event)
					close(release)
					require.NoError(t, <-result)
					require.NoError(t, database.Callback().Query().Remove("hold_priority_snapshot"))
					remote := receive()
					if operation == "move" {
						assert.Equal(t, http.StatusConflict, remote.Status)
						assert.Equal(t, "channel_order_conflict", remote.Response.Code)
					} else {
						require.Empty(t, remote.Error)
						c, err := model.GetChannelById(1, true)
						require.NoError(t, err)
						assert.EqualValues(t, 80, c.GetPriority(), "serialized manual edit must not be lost after comparison")
						var ability model.Ability
						require.NoError(t, database.Where("channel_id = ?", 1).First(&ability).Error)
						require.NotNil(t, ability.Priority)
						assert.EqualValues(t, 80, *ability.Priority)
					}
				})
			}
		})
	}
}

func TestChannelPriorityPermissions(t *testing.T) {
	user, legacy := setupAccessTokenAudit(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.Ability{}))
	previousCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { common.MemoryCacheEnabled = previousCache })
	for _, id := range []int{1, 2} {
		priority := int64(10)
		channel := model.Channel{Id: id, Key: "secret", Models: "test-model", Group: "default", Status: common.ChannelStatusEnabled, Priority: &priority}
		require.NoError(t, channel.Insert())
	}
	_, browser := createAccessTokenTestSession(t, user.Id, "priority-permissions")
	readOnly, _ := createScopedAccessToken(t, user.Id, 0, "channel:read")
	writeOnly, _ := createScopedAccessToken(t, user.Id, 0, "channel:write")
	both, _ := createScopedAccessToken(t, user.Id, 0, "channel:read", "channel:write")
	middleware.DeclareAccessTokenPermissionRoute(http.MethodPost, "/api/channel/priority/reorder", authz.ChannelWrite)
	router := gin.New()
	router.Use(middleware.RequestId(), middleware.AccessTokenAudit())
	router.POST("/api/channel/priority/reorder", middleware.AdminAuth(), middleware.RequirePermission(authz.ChannelWrite), ReorderChannelPriority)
	for _, test := range []struct {
		name, credential string
		read, write      bool
		status           int
	}{
		{"anonymous", "", true, true, http.StatusUnauthorized},
		{"browser write only", browser, false, true, http.StatusForbidden},
		{"browser read only", browser, true, false, http.StatusForbidden},
		{"PAT write scope only", writeOnly, true, true, http.StatusForbidden},
		{"PAT read scope only", readOnly, true, true, http.StatusForbidden},
		{"PAT user lacks read", both, false, true, http.StatusForbidden},
		{"PAT user lacks write", both, true, false, http.StatusForbidden},
		{"legacy user lacks read", legacy, false, true, http.StatusForbidden},
		{"browser both permissions", browser, true, true, http.StatusOK},
		{"PAT both scopes and permissions", both, true, true, http.StatusOK},
		{"legacy both permissions", legacy, true, true, http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, authz.SetUserPermissions(user.Id, authz.PermissionsMap{authz.ResourceChannel: {authz.ActionRead: test.read, authz.ActionWrite: test.write}}))
			_, before := priorityRequest(t, GetAllChannels, http.MethodGet, "/api/channel/", nil)
			var abilities []model.Ability
			require.NoError(t, model.DB.Order("channel_id").Find(&abilities).Error)
			move := model.ChannelPriorityMove{ExpectedSnapshot: before.Data.Snapshot, Page: 1, PageSize: 10, MovedID: before.Data.Items[1].Id, TargetID: before.Data.Items[0].Id}
			body, err := common.Marshal(move)
			require.NoError(t, err)
			response := accessTokenRequest(router, http.MethodPost, "/api/channel/priority/reorder", test.credential, "", string(body))
			require.Equal(t, test.status, response.Code, response.Body.String())
			_, after := priorityRequest(t, GetAllChannels, http.MethodGet, "/api/channel/", nil)
			if test.status != http.StatusOK {
				assert.Equal(t, before, after, "denied request must preserve channel state")
				var actual []model.Ability
				require.NoError(t, model.DB.Order("channel_id").Find(&actual).Error)
				assert.Equal(t, abilities, actual, "denied request must preserve routing abilities")
				return
			}
			assert.Equal(t, []int{move.MovedID, move.TargetID}, []int{after.Data.Items[0].Id, after.Data.Items[1].Id})
		})
	}
}
