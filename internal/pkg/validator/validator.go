// Package validator 提供数据验证功能。
package validator

import (
	"regexp"
	"unicode/utf8"
)

// IsValidPassword 检查密码是否符合要求
func IsValidPassword(password string) bool {
	if len(password) < 8 || len(password) > 65 { // 密码长度必须在8到64个字符之间
		return false
	}

	base := regexp.MustCompile(`^[A-Za-z0-9@$!%*?&.]+$`) // 密码只能包含字母、数字和特殊字符
	if !base.MatchString(password) {                     // 密码包含非法字符
		return false
	}

	hasDigit := regexp.MustCompile(`[0-9]`).MatchString(password)        // 密码必须包含数字
	hasLetter := regexp.MustCompile(`[A-Za-z]`).MatchString(password)    // 密码必须包含字母
	hasSpecial := regexp.MustCompile(`[@$!%*?&.]`).MatchString(password) // 密码必须包含特殊字符

	count := 0
	if hasDigit {
		count++
	}
	if hasLetter {
		count++
	}
	if hasSpecial {
		count++
	}

	return count >= 2
}

// IsValidEmail 检查邮箱地址是否符合要求
func IsValidEmail(email string) bool {
	emailRegex, err := regexp.Compile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,126}$`)
	if err != nil {
		return false
	}
	return emailRegex.MatchString(email) // 检查邮箱地址是否符合正则表达式
}

// IsValidUsername 检查用户名是否符合要求
func IsValidUsername(username string) bool {
	usernameRegex, err := regexp.Compile(`^[a-zA-Z0-9_]{3,32}$`)
	if err != nil {
		return false
	}
	return usernameRegex.MatchString(username) // 检查用户名是否符合正则表达式
}

// IsValidNickname 检查昵称是否符合要求
func IsValidNickname(nickname string) bool {
	// 昵称不能超过16个 utf-8 编码
	// 仅支持：（大小写）英文字符、下划线、数字、汉字
	if utf8.RuneCountInString(nickname) > 16 {
		return false
	}
	return regexp.MustCompile(`^[a-zA-Z0-9_\p{Han}]+$`).MatchString(nickname)
}

// IsValidAvatarURL 检查头像 URL 是否符合要求
func IsValidAvatarURL(avatar string) bool {
	// 1. 长度校验：不能为空，且不超过 255 字符
	if len(avatar) == 0 || len(avatar) > 255 {
		return false
	}

	// 2. 不允许包含任何空白字符
	if regexp.MustCompile(`\s`).MatchString(avatar) {
		return false
	}

	// 3. 必须以 http:// 或 https:// 开头（严谨正则）
	// ^https?:// s? 可以有 s 也可以没有（https /http）
	if !regexp.MustCompile(`^https?://`).MatchString(avatar) {
		return false
	}

	// 4. 合法 URL 格式校验（宽松、安全、通用）
	pattern := regexp.MustCompile(`^https?://[-a-zA-Z0-9+&@#/%?=~_|!:,.;]*[-a-zA-Z0-9+&@#/%=~_|]$`)
	return pattern.MatchString(avatar)
}

// IsValidProjectName 检查项目名称是否符合要求
func IsValidProjectName(name string) bool {
	return isValidName(name)
}

// IsValidTaskTitle 检查任务标题是否符合要求
func IsValidTaskTitle(name string) bool {
	return isValidName(name)
}

// 规则：
//  1. 长度：2 ~ 20 个 utf-8 字符（rune）
//  2. 首字符：只能是 英文 / 汉字
//  3. 允许字符：英文、数字、汉字、下划线、非连续的空格
func isValidName(name string) bool {
	// 1. 长度校验：2 ~ 20 个 rune
	nameLen := utf8.RuneCountInString(name)
	if nameLen < 2 || nameLen > 20 {
		return false
	}

	// 2. 完整正则：首字符必须是英文/汉字，后面允许英文/数字/汉字/下划线/【中间有单个空格】
	// 规则：
	// 1. 首字符：英文 / 汉字
	// 2. 中间可以包含：英文、数字、汉字、下划线、【单个空格】
	// 3. 禁止连续空格
	// 4. 禁止首尾空格
	// ^ 开头
	// [a-zA-Z\p{Han}]+         首字符：英文、汉字（至少1个）
	// (?: [a-zA-Z0-9_\p{Han}]+) 分组：以【单个空格】开头，后接合法字符
	// *                        表示这一组可以出现 0 次或多次
	// $ 结尾
	reg := regexp.MustCompile(`^[a-zA-Z\p{Han}]+(?: [a-zA-Z0-9_\p{Han}]+)*$`)
	return reg.MatchString(name)
}

const (
	MaxDescriptionLength = 500 // 任务描述
	MaxCommentLength     = 500 // 评论内容
)

// IsValidDescription 判断一个字符串是否是合法描述。
func IsValidDescription(description string) bool {
	length := len([]rune(description))
	return length <= MaxDescriptionLength
}

// IsValidCommentContent 判断是否是一个合法的评论
func IsValidCommentContent(content string) bool {
	length := len([]rune(content))
	return length <= MaxCommentLength
}

// getStringLength 计算字符串字符长度（汉字/英文/符号都算1个）
func getStringLength(s string) int {
	return len([]rune(s))
}
