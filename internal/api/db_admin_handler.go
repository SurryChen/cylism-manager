package api

import (
	"fmt"
	"net/http"
	"reflect"
	"strconv"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm/schema"
)

// tableRegistry 注册可管理的数据表
var tableRegistry = map[string]interface{}{
	"servers":        model.Server{},
	"sites":          model.Site{},
	"certs":          model.Cert{},
	"audit_logs":     model.AuditLog{},
	"operation_logs": model.OperationLog{},
	"users":          model.User{},
}

// autoColumns 自动管理的字段，增改时排除
var autoColumns = map[string]bool{
	"id": true, "created_at": true, "updated_at": true, "deleted_at": true,
}

// DBAdminHandler 数据库管理 handler
type DBAdminHandler struct {
	store *store.Store
}

// NewDBAdminHandler 创建 DBAdminHandler
func NewDBAdminHandler(s *store.Store) *DBAdminHandler {
	return &DBAdminHandler{store: s}
}

// ListTables 返回所有可管理的表名
func (h *DBAdminHandler) ListTables(c *gin.Context) {
	names := make([]string, 0, len(tableRegistry))
	for name := range tableRegistry {
		names = append(names, name)
	}
	c.JSON(http.StatusOK, gin.H{"tables": names})
}

// ListRecords 分页查询指定表的数据
func (h *DBAdminHandler) ListRecords(c *gin.Context) {
	tableName := c.Param("table")
	model, ok := tableRegistry[tableName]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid table"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	sort := c.DefaultQuery("sort", "id")
	order := c.DefaultQuery("order", "desc")

	if page < 1 { page = 1 }
	if size < 1 { size = 20 }
	if size > 100 { size = 100 }

	// 校验排序字段
	if !h.columnExists(tableName, sort) {
		sort = "id"
	}
	if order != "asc" && order != "desc" {
		order = "desc"
	}

	db := h.store.DB()

	// 查询总数
	var total int64
	db.Table(tableName).Count(&total)

	// 分页查询
	offset := (page - 1) * size
	rows, err := db.Table(tableName).Select("*").Order(sort + " " + order).Limit(size).Offset(offset).Rows()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("query failed: %v", err)})
		return
	}
	defer rows.Close()

	// 读取列名
	columns, err := rows.Columns()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get columns"})
		return
	}

	// 收集敏感字段（json:"-"）
	sensitive := h.sensitiveColumns(model)

	var results []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range columns {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			continue
		}

		row := make(map[string]interface{}, len(columns))
		for i, col := range columns {
			if sensitive[col] {
				continue
			}
			val := values[i]
			if b, ok := val.([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = val
			}
		}
		results = append(results, row)
	}

	if results == nil {
		results = []map[string]interface{}{}
	}

	c.JSON(http.StatusOK, gin.H{
		"rows":      results,
		"columns":   h.visibleColumns(tableName, model),
		"total":     total,
		"page":      page,
		"size":      size,
	})
}

// CreateRecord 新增记录
func (h *DBAdminHandler) CreateRecord(c *gin.Context) {
	tableName := c.Param("table")
	_, ok := tableRegistry[tableName]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid table"})
		return
	}

	var body map[string]interface{}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 排除自动字段
	filtered := make(map[string]interface{})
	for k, v := range body {
		if !autoColumns[k] {
			filtered[k] = v
		}
	}

	if len(filtered) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no valid fields"})
		return
	}

	result := h.store.DB().Table(tableName).Create(filtered)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"ok": true, "id": filtered["id"]})
}

// UpdateRecord 按主键更新记录
func (h *DBAdminHandler) UpdateRecord(c *gin.Context) {
	tableName := c.Param("table")
	_, ok := tableRegistry[tableName]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid table"})
		return
	}

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var body map[string]interface{}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 排除自动字段
	filtered := make(map[string]interface{})
	for k, v := range body {
		if !autoColumns[k] {
			filtered[k] = v
		}
	}

	if len(filtered) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no valid fields"})
		return
	}

	result := h.store.DB().Table(tableName).Where("id = ?", id).Updates(filtered)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "record not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// DeleteRecord 按主键删除记录
func (h *DBAdminHandler) DeleteRecord(c *gin.Context) {
	tableName := c.Param("table")
	_, ok := tableRegistry[tableName]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid table"})
		return
	}

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	result := h.store.DB().Table(tableName).Where("id = ?", id).Delete(nil)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "record not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// columnExists 检查表中是否存在指定列（缓存到 map 中）
func (h *DBAdminHandler) columnExists(tableName, col string) bool {
	// 简单防 SQL 注入：只允许字母、数字和下划线
	for _, r := range col {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_') {
			return false
		}
	}
	// 直接信任 GORM Migrator
	return h.store.DB().Migrator().HasColumn(tableRegistry[tableName], col)
}

var gormNamer = schema.NamingStrategy{}

// sensitiveColumns 返回模型中 json:"-" 的字段集合
func (h *DBAdminHandler) sensitiveColumns(model interface{}) map[string]bool {
	result := make(map[string]bool)
	t := reflect.TypeOf(model)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		jsonTag := field.Tag.Get("json")
		if jsonTag == "-" {
			// 用 GORM naming strategy 获取列名：SSHPassword → ssh_password
			colName := gormNamer.ColumnName("", field.Name)
			result[colName] = true
		}
	}
	return result
}

// visibleColumns 返回可展示的列名（排除敏感字段）
func (h *DBAdminHandler) visibleColumns(tableName string, model interface{}) []string {
	sensitive := h.sensitiveColumns(model)
	columns, _ := h.store.DB().Migrator().ColumnTypes(tableName)
	var result []string
	for _, c := range columns {
		if !sensitive[c.Name()] {
			result = append(result, c.Name())
		}
	}
	return result
}

