// internal/pkg/utils/repoutil/task_batch_update.go
// 构造批量更新任务的 条件语句 语句
package repoutil

import (
	"strings"

	"gorm.io/gorm/clause"
)

// CaseWhenItem 批量 CASE WHEN 更新项
//
// 字段说明：
//   - ID：匹配字段的值，例如任务 ID
//   - Value：需要更新的新值，例如新的 sort_order
type CaseWhenItem struct {
	ID    interface{}
	Value interface{}
}

// BuildCaseWhenExpr 构建通用批量更新 CASE WHEN 表达式
//
// 示例:
//
//	CASE `id`
//	    WHEN ? THEN ?
//	    WHEN ? THEN ?
//	    ELSE `sort_order`
//	END
//
// 参数说明：
//   - idField：匹配字段名，例如 "id"
//   - updateField：要更新的字段名，例如 "sort_order"
//   - items：匹配值和更新值列表
func BuildCaseWhenExpr(idField string, updateField string, items []CaseWhenItem) clause.Expr {
	if len(items) == 0 {
		return clause.Expr{
			SQL: "`" + updateField + "`",
		}
	}

	sqlBuilder := strings.Builder{}
	sqlBuilder.WriteString("CASE `" + idField + "` ")

	args := make([]interface{}, 0, len(items)*2)

	for _, item := range items {
		sqlBuilder.WriteString("WHEN ? THEN ? ")
		args = append(args, item.ID, item.Value)
	}

	sqlBuilder.WriteString("ELSE `" + updateField + "` END")

	return clause.Expr{
		SQL:  sqlBuilder.String(),
		Vars: args,
	}
}
