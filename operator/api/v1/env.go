// Package v1 的环境变量访问封装，供 webhook 读取 ENABLE_WEBHOOK。
package v1

import "os"

// envLookup 读环境变量，缺省返回 def。
func envLookup(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
