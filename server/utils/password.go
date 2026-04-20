package utils

// 文件说明：这个文件封装密码哈希工具。
// 实现方式：统一使用 bcrypt 默认成本生成密码摘要。
// 这样做的好处是密码处理策略在一处集中，后续切换成本或算法时不需要搜全仓库。

import "golang.org/x/crypto/bcrypt"

// HashPassword 对明文密码做 bcrypt 哈希。
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}
