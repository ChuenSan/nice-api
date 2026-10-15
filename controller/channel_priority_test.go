package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
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
	context.Request.Header.Set("Content-Type", "application/json")
	handler(context)
	var response priorityResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response), recorder.Body.String())
	return recorder.Code, response
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
			for _, query := range []string{"group=default", "status=1", "type=1", "tag_mode=true", "sort_by=id", "sort_by=priority&sort_order=asc", "id_sort=true", "keyword=priority"} {
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
