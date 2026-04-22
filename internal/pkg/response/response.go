package response

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"smart-task-platform/internal/pkg/errmsg"
)

// Response 统一响应结构
type Response struct {
	Code    int         `json:"code"`           // 业务状态码
	Message string      `json:"message"`        // 提示信息
	Data    interface{} `json:"data,omitempty"` // 响应数据
}

// PageQuery 通用分页查询参数
type PageQuery struct {
	Page     int `form:"page" binding:"omitempty,min=1"`              // 页码
	PageSize int `form:"page_size" binding:"omitempty,min=1,max=100"` // 每页数量
}

// PageData 统一分页响应结构 任务列表/项目列表...
type PageData struct {
	List     interface{} `json:"list"`      // 列表数据
	Total    int64       `json:"total"`     // 总数
	Page     int         `json:"page"`      // 当前页
	PageSize int         `json:"page_size"` // 每页大小
}

// JSON 输出统一 JSON 响应
func JSON(c *gin.Context, httpCode int, code int, data interface{}) {
	c.JSON(httpCode, Response{
		Code:    code,
		Message: errmsg.GetMsg(code),
		Data:    data,
	})
}

// Success 返回成功响应，无数据
func Success(c *gin.Context) {
	JSON(c, http.StatusOK, errmsg.Success, nil)
}

// SuccessWithData 返回成功响应，带数据
func SuccessWithData(c *gin.Context, data interface{}) {
	JSON(c, http.StatusOK, errmsg.Success, data)
}

// SuccessWithMessage 返回成功响应，自定义 message
func SuccessWithMessage(c *gin.Context, message string, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    errmsg.Success,
		Message: message,
		Data:    data,
	})
}

// SuccessWithPage 返回分页成功响应
func SuccessWithPage(c *gin.Context, list interface{}, total int64, page, pageSize int) {
	JSON(c, http.StatusOK, errmsg.Success, PageData{
		List:     list,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}

// Fail 返回失败响应，HTTP 状态码默认 200，业务码表示错误
// 适用于前后端统一通过业务码判断错误的场景。
func Fail(c *gin.Context, code int) {
	JSON(c, http.StatusOK, code, nil)
}

// FailWithData 返回失败响应并携带附加数据
func FailWithData(c *gin.Context, code int, data interface{}) {
	JSON(c, http.StatusOK, code, data)
}

// FailWithMessage 返回失败响应，自定义错误信息
func FailWithMessage(c *gin.Context, code int, message string) {
	c.JSON(http.StatusOK, Response{
		Code:    code,
		Message: message,
		Data:    nil,
	})
}

// Abort 返回错误并中断后续处理
func Abort(c *gin.Context, httpCode int, code int) {
	c.AbortWithStatusJSON(httpCode, Response{
		Code:    code,
		Message: errmsg.GetMsg(code),
		Data:    nil,
	})
}

// AbortWithData 返回错误数据并中断后续处理
func AbortWithData(c *gin.Context, httpCode int, code int, data interface{}) {
	c.AbortWithStatusJSON(httpCode, Response{
		Code:    code,
		Message: errmsg.GetMsg(code),
		Data:    data,
	})
}

// AbortWithMessage 返回自定义错误信息并中断后续处理
func AbortWithMessage(c *gin.Context, httpCode int, code int, message string) {
	c.AbortWithStatusJSON(httpCode, Response{
		Code:    code,
		Message: message,
		Data:    nil,
	})
}
