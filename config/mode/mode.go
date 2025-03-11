package mode

// Mode 定义了服务器运行模式
type Mode string

const (
	// Dev 开发模式
	Dev Mode = "dev"
	// Prod 生产模式
	Prod Mode = "prod"
)

// Get 返回当前模式，默认为开发模式
func Get(s ...string) Mode {
	if len(s) == 0 {
		return Dev
	}
	
	switch s[0] {
	case string(Dev):
		return Dev
	case string(Prod):
		return Prod
	default:
		return Dev
	}
}
