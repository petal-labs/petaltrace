package netutil

import "testing"

func TestIsLoopbackHost(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{host: "localhost", want: true},
		{host: "LOCALHOST", want: true},
		{host: "127.0.0.1", want: true},
		{host: "::1", want: true},
		{host: "0.0.0.0", want: false},
		{host: "::", want: false},
		{host: "example.com", want: false},
		{host: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			if got := IsLoopbackHost(tt.host); got != tt.want {
				t.Errorf("IsLoopbackHost(%q) = %t, want %t", tt.host, got, tt.want)
			}
		})
	}
}

func TestIsLoopbackAddr(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		{addr: "127.0.0.1:8090", want: true},
		{addr: "[::1]:8090", want: true},
		{addr: "localhost:8090", want: true},
		{addr: "0.0.0.0:8090", want: false},
		{addr: "[::]:8090", want: false},
		{addr: ":8090", want: false},
		{addr: "127.0.0.1:bad", want: false},
		{addr: "127.0.0.1:65536", want: false},
		{addr: "not-an-address", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			if got := IsLoopbackAddr(tt.addr); got != tt.want {
				t.Errorf("IsLoopbackAddr(%q) = %t, want %t", tt.addr, got, tt.want)
			}
		})
	}
}
