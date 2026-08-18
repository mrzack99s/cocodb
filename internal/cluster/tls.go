package cluster

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"time"
)

// GenerateDevCert creates an in-memory self-signed TLS certificate for fast development & testing.
func GenerateDevCert(hosts ...string) (tls.Certificate, *x509.CertPool, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, nil, err
	}

	notBefore := time.Now().Add(-1 * time.Hour)
	notAfter := notBefore.Add(365 * 24 * time.Hour)

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return tls.Certificate{}, nil, err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"CoCoDB Cluster Security"},
			CommonName:   "cocodb.cluster.local",
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{"cocodb.cluster.local", "localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, h)
		}
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, nil, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	b, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: b})

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, nil, err
	}

	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(certPEM)

	return cert, pool, nil
}

// ServerTLSConfig creates a hardened TLS 1.3 server configuration.
func ServerTLSConfig(cert tls.Certificate, caPool *x509.CertPool, requireClientCert bool) *tls.Config {
	cfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
		CipherSuites: []uint16{
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_CHACHA20_POLY1305_SHA256,
		},
	}

	if requireClientCert && caPool != nil {
		cfg.ClientCAs = caPool
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
	}

	return cfg
}

// ClientTLSConfig creates a hardened TLS 1.3 client configuration.
func ClientTLSConfig(clientCert *tls.Certificate, caPool *x509.CertPool, serverName string) *tls.Config {
	cfg := &tls.Config{
		MinVersion: tls.VersionTLS13,
		ServerName: serverName,
	}

	if clientCert != nil {
		cfg.Certificates = []tls.Certificate{*clientCert}
	}
	if caPool != nil {
		cfg.RootCAs = caPool
	} else {
		cfg.InsecureSkipVerify = true // Fallback for dev mode when cert pool not provided
	}

	return cfg
}
