package handler

import (
	"fmt"
	"net/http"
	"net/netip"
)

// RealIPHeader - заголовок запроса, в котором агент передаёт IP-адрес своего хоста.
const RealIPHeader = "X-Real-IP"

// TrustedSubnet пропускает запросы только от агентов из доверенной подсети.
// Нулевой *TrustedSubnet и подсеть, заданная пустой строкой, означают, что
// проверка выключена.
type TrustedSubnet struct {
	prefix  netip.Prefix
	enabled bool
}

// NewTrustedSubnet создаёт фильтр по подсети cidr в бесклассовой нотации
// (например, "192.168.1.0/24"). Пустой cidr выключает проверку. Некорректная
// запись подсети возвращает ошибку.
func NewTrustedSubnet(cidr string) (*TrustedSubnet, error) {
	if cidr == "" {
		return &TrustedSubnet{}, nil
	}
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return nil, fmt.Errorf("некорректная доверенная подсеть %q: %w", cidr, err)
	}
	return &TrustedSubnet{prefix: prefix.Masked(), enabled: true}, nil
}

// Contains сообщает, входит ли адрес ip в доверенную подсеть. Если проверка
// выключена, любой адрес считается доверенным.
func (ts *TrustedSubnet) Contains(ip string) bool {
	if ts == nil || !ts.enabled {
		return true
	}
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	return ts.prefix.Contains(addr.Unmap())
}

// Handle возвращает middleware, отвечающее 403 Forbidden на запросы, у которых
// IP-адрес из заголовка X-Real-IP отсутствует, некорректен или не входит в
// доверенную подсеть.
func (ts *TrustedSubnet) Handle(next http.Handler) http.Handler {
	if ts == nil || !ts.enabled {
		return next
	}
	return http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		if !ts.Contains(req.Header.Get(RealIPHeader)) {
			http.Error(resp, "Forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(resp, req)
	})
}
