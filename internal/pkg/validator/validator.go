// Package validator 提供数据验证功能。
package validator

import (
	"regexp"
	"unicode"
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

const MaxDescriptionNaturalLength = 500

// IsValidDescription 判断一个字符串是否是合法描述。
//
// 计数规则：
//  1. 汉字：每个汉字计为 1
//  2. 连续字母：计为 1 个单词
//  3. 连续数字：计为 1 个数字单元
//  4. 符号、标点、空格、Emoji 等：忽略，不计数
//  5. 非法 UTF-8：直接返回 false
//
// 示例：
//
//	"描述一个项目"        => 6
//	"hello world"       => 2
//	"项目 project 123"  => 4
//	"版本v2"            => 4
//	"你好😊world!!!123" => 4
//
// 说明：
//
//	"版本v2" 计算方式：
//	  版 = 1
//	  本 = 1
//	  v  = 1 个单词
//	  2  = 1 个数字单元
//	  总计 4
func IsValidDescription(s string) bool {
	return descriptionNaturalLengthWithinLimit(s, MaxDescriptionNaturalLength)
}

// descriptionNaturalLengthWithinLimit 判断自然长度是否不超过 limit。
//
// 这个函数的特点：
//  1. 一次遍历
//  2. 无额外分配
//  3. 无正则
//  4. ASCII 快路径
//  5. 超过限制立即返回
//  6. 正确处理 Unicode 汉字、字母、数字
func descriptionNaturalLengthWithinLimit(s string, limit int) bool {
	if limit < 0 {
		return false
	}

	count := 0       // 数量
	i := 0           // 索引
	n := len(s)      // 长度
	inWord := false  // 当前是否处于一个连续字母单词中
	inDigit := false // 当前是否处于一个连续数字单元中

	for i < n {
		c := s[i]
		if c < utf8.RuneSelf { // c 的值 < 128 说明是 ASCII 码
			//对 ASCII 直接用 byte 判断，避免额外开销
			switch {
			case isASCIILetter(c):
				// ASCII 字母。
				// 如果当前不在单词中，则说明遇到了一个新单词。
				if !inWord {
					count++
					if count > limit {
						return false
					}
					inWord = true
				}
				inDigit = false // 字母会打断数字。
				i++

			case isASCIIDigit(c):
				// ASCII 数字。
				// 如果当前不在数字单元中，则说明遇到了一个新的数字单元。
				if !inDigit {
					count++
					if count > limit {
						return false
					}
					inDigit = true
				}
				inWord = false // 数字会打断单词。
				i++

			default:
				// ASCII 其他字符，例如：
				//  空格、逗号、句号、短横线、括号等。
				// 这些字符不计数，同时作为分隔符，打断单词和数字。
				inWord = false
				inDigit = false
				i++
			}

			continue
		}
		// > 128 说明非 ACSII 字符
		r, size := utf8.DecodeRuneInString(s[i:]) //
		// 如果 r == utf8.RuneError 且 size == 1，说明遇到了非法 UTF-8 编码。
		//
		// 注意：
		// 不能简单判断 r == unicode.ReplacementChar。
		// 因为合法字符串中也可以包含真正的 '�' 字符。
		if r == utf8.RuneError && size == 1 {
			return false
		}

		switch {
		case unicode.Is(unicode.Han, r):
			// 汉字：每个汉字计为 1。
			count++
			if count > limit {
				return false
			}

			// 汉字是独立计数单位，会打断字母单词和数字单元。
			inWord = false
			inDigit = false

		case unicode.IsLetter(r):
			// Unicode 字母。
			//
			// 例如：
			//  é、ü、α、д 等。
			//
			// 注意：
			// 由于汉字也属于 Letter，所以必须先判断 unicode.Han，
			// 再判断 unicode.IsLetter。
			if !inWord {
				count++
				if count > limit {
					return false
				}
				inWord = true
			}

			inDigit = false

		case unicode.IsMark(r):
			// 组合音标。
			//
			// 例如：
			//  e + ́
			//
			// 组合音标不单独计数。
			// 如果它出现在单词中，就保持单词状态。
			// 如果它单独出现，就忽略。
			//
			// 这里不需要修改 inWord。
			// 但是它不是数字，所以需要打断数字状态。
			inDigit = false

		case unicode.IsDigit(r):
			// Unicode 数字。
			//
			// 例如：
			//  全角数字：１２３
			//  阿拉伯-印度数字：١٢٣
			if !inDigit {
				count++
				if count > limit {
					return false
				}
				inDigit = true
			}

			inWord = false

		default:
			// 其他 Unicode 字符。
			// 例如：
			//  中文标点、Emoji、数学符号、货币符号、空白字符等。
			//  不计数，同时作为分隔符。
			inWord = false
			inDigit = false
		}
		i += size
	}

	return true
}

// isASCIILetter 判断 c 是否是 ASCII 英文字母。
func isASCIILetter(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// isASCIIDigit 判断 c 是否是 ASCII 数字。
func isASCIIDigit(c byte) bool {
	return c >= '0' && c <= '9'
}
