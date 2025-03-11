package ipdns

// Provider 定义了获取 IP 地址的接口
type Provider interface {
	// GetIP 返回当前服务器的 IP 地址
	GetIP() (string, error)
}