package receiver

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"time"
)

const (
	certLifetime = 24 * time.Hour
	serialBits   = 62
)

func ServiceHosts() []string {
	return []string{"hooks.slack.com", "discord.com", "discordapp.com", "api.telegram.org", "localhost"}
}

func NewTLS(now time.Time) (*tls.Config, []byte, error) {
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("make the CA key: %w", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber: serial(), Subject: pkix.Name{CommonName: "Dhow notify receiver CA"}, NotBefore: now.Add(-time.Minute),
		NotAfter: now.Add(certLifetime), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, fmt.Errorf("sign the CA certificate: %w", err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		return nil, nil, fmt.Errorf("read the CA certificate: %w", err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("make the server key: %w", err)
	}
	template := &x509.Certificate{
		SerialNumber: serial(), Subject: pkix.Name{CommonName: "notify-receiver"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(certLifetime),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames: ServiceHosts(), IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
	if err != nil {
		return nil, nil, fmt.Errorf("sign the server certificate: %w", err)
	}
	cfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
		Certificates: []tls.Certificate{{
			Certificate: [][]byte{der}, PrivateKey: key,
		}},
	}
	return cfg, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), nil
}

func serial() *big.Int {
	n, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), serialBits))
	if err != nil {
		return big.NewInt(time.Now().UnixNano())
	}
	return n.Add(n, big.NewInt(1))
}
