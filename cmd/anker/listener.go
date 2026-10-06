package main

import (
	"crypto/tls"
	"net"
)

// Bind and load TLS synchronously before the privileged local API reports readiness.
func bindWeb(address, cert, key string) (net.Listener, *tls.Config, error) {
	var config *tls.Config
	if cert != "" {
		pair, err := tls.LoadX509KeyPair(cert, key)
		if err != nil {
			return nil, nil, err
		}
		config = &tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12}
	}
	listener, err := net.Listen("tcp", address)
	return listener, config, err
}
