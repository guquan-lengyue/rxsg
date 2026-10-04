package auth

// 请求上下文中保存登录态的键。由 middleware.RequireAuth 写入，各 handler 读取。
// 放在 auth 包以避免 auth 与 middleware 互相 import。
const (
	CtxUID = "uid"
	CtxSID = "sid"
)