package netutil

import (
	"net"
	"strconv"
	"strings"
)

// IsLoopbackHost reports whether host is a loopback IP or localhost.
func IsLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}

	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

// IsLoopbackAddr reports whether addr has a loopback host and a valid host:port shape.
func IsLoopbackAddr(addr string) bool {
	host, port, err := net.SplitHostPort(addr)
	if err != nil || !IsLoopbackHost(host) {
		return false
	}

	portNumber, err := strconv.Atoi(port)
	return err == nil && portNumber >= 0 && portNumber <= 65535
}
