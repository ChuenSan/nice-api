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
