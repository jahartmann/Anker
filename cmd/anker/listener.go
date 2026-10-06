package main

import (
	"crypto/tls"
	"net"
	"sync"
)

// Bind and load TLS synchronously before the privileged local API reports readiness.
func bindWeb(address, cert, key string) (net.Listener, *tls.Config, error) {
	var config *tls.Config
	if cert != "" {
		pair, err := tls.LoadX509KeyPair(cert, key)
		if err != nil {
			return nil, nil, err
		}
		var mu sync.Mutex
		current := &pair
		config = &tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			mu.Lock()
			defer mu.Unlock()
			if next, err := tls.LoadX509KeyPair(cert, key); err == nil {
				current = &next
			}
			return current, nil
		}}
	}
	listener, err := net.Listen("tcp", address)
	return listener, config, err
}
