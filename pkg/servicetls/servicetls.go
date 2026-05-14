package servicetls

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"sort"
	"strings"
	"time"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	ModeMTLS     = "mtls"
	ModeTLS      = "tls"
	ModeDisabled = "disabled"

	EnvMode       = "DISPATCHER_TLS_MODE"
	EnvSecret     = "DISPATCHER_TLS_SECRET"
	EnvServerName = "DISPATCHER_TLS_SERVER_NAME"

	defaultServerName = "dispatcher"
)

var (
	certNotBefore = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	certNotAfter  = time.Date(2125, 1, 1, 0, 0, 0, 0, time.UTC)
)

type Config struct {
	Mode        string
	Secret      string
	Role        string
	ServiceID   string
	ServerName  string
	ServerNames []string
}

func NormalizeMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "auto", "mtls", "m-tls", "mutual", "mutual-tls":
		return ModeMTLS
	case "tls", "server", "server-tls":
		return ModeTLS
	case "off", "false", "no", "none", "disabled", "disable", "insecure", "plaintext":
		return ModeDisabled
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

func Enabled(mode string) bool {
	return NormalizeMode(mode) != ModeDisabled
}

func ServerCredentials(cfg Config) (credentials.TransportCredentials, error) {
	mode := NormalizeMode(cfg.Mode)
	if mode == ModeDisabled {
		return nil, nil
	}
	if mode != ModeTLS && mode != ModeMTLS {
		return nil, fmt.Errorf("unsupported dispatcher TLS mode %q", cfg.Mode)
	}

	caCert, caKey, caDER, pool, err := certificateAuthority(cfg.Secret)
	if err != nil {
		return nil, err
	}
	cert, err := signedServerCertificate(cfg, caCert, caKey, caDER)
	if err != nil {
		return nil, err
	}

	tlsConfig := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert},
		ClientCAs:    pool,
	}
	if mode == ModeMTLS {
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	}
	return credentials.NewTLS(tlsConfig), nil
}

func ClientCredentials(cfg Config) (credentials.TransportCredentials, error) {
	mode := NormalizeMode(cfg.Mode)
	if mode == ModeDisabled {
		return insecure.NewCredentials(), nil
	}
	if mode != ModeTLS && mode != ModeMTLS {
		return nil, fmt.Errorf("unsupported dispatcher TLS mode %q", cfg.Mode)
	}

	caCert, caKey, caDER, pool, err := certificateAuthority(cfg.Secret)
	if err != nil {
		return nil, err
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    pool,
		ServerName: normalizeServerName(cfg.ServerName),
	}
	if mode == ModeMTLS {
		cert, err := signedClientCertificate(cfg, caCert, caKey, caDER)
		if err != nil {
			return nil, err
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}
	return credentials.NewTLS(tlsConfig), nil
}

func certificateAuthority(secret string) (*x509.Certificate, ed25519.PrivateKey, []byte, *x509.CertPool, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil, nil, nil, nil, fmt.Errorf("dispatcher TLS secret is not configured")
	}

	caKey := ed25519.NewKeyFromSeed(deriveSeed(secret, "ca-key"))
	tmpl := &x509.Certificate{
		SerialNumber:          deterministicSerial(secret, "ca-serial"),
		Subject:               pkix.Name{CommonName: "nopsai dispatcher automatic CA"},
		NotBefore:             certNotBefore,
		NotAfter:              certNotAfter,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, caKey.Public(), caKey)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("create dispatcher TLS CA: %w", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("parse dispatcher TLS CA: %w", err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return cert, caKey, der, pool, nil
}

func signedServerCertificate(cfg Config, caCert *x509.Certificate, caKey ed25519.PrivateKey, caDER []byte) (tls.Certificate, error) {
	dnsNames, ipAddresses := serverSubjectAltNames(cfg)
	_, leafKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generate dispatcher server key: %w", err)
	}
	serial, err := randomSerial()
	if err != nil {
		return tls.Certificate{}, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: defaultServerName},
		NotBefore:    certNotBefore,
		NotAfter:     certNotAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     dnsNames,
		IPAddresses:  ipAddresses,
	}
	return signLeaf(tmpl, caCert, caKey, caDER, leafKey, "dispatcher server certificate")
}

func signedClientCertificate(cfg Config, caCert *x509.Certificate, caKey ed25519.PrivateKey, caDER []byte) (tls.Certificate, error) {
	_, leafKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generate dispatcher client key: %w", err)
	}
	serial, err := randomSerial()
	if err != nil {
		return tls.Certificate{}, err
	}

	role := strings.TrimSpace(cfg.Role)
	serviceID := strings.TrimSpace(cfg.ServiceID)
	commonName := strings.Trim(strings.Join([]string{role, serviceID}, ":"), ":")
	if commonName == "" {
		commonName = "nopsai-service"
	}

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    certNotBefore,
		NotAfter:     certNotAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	if serviceURI := clientServiceURI(role, serviceID); serviceURI != nil {
		tmpl.URIs = []*url.URL{serviceURI}
	}
	return signLeaf(tmpl, caCert, caKey, caDER, leafKey, "dispatcher client certificate")
}

func signLeaf(tmpl *x509.Certificate, caCert *x509.Certificate, caKey ed25519.PrivateKey, caDER []byte, leafKey ed25519.PrivateKey, label string) (tls.Certificate, error) {
	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, leafKey.Public(), caKey)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("create %s: %w", label, err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("parse %s: %w", label, err)
	}
	return tls.Certificate{
		Certificate: [][]byte{der, caDER},
		PrivateKey:  leafKey,
		Leaf:        leaf,
	}, nil
}

func deriveSeed(secret, purpose string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("nopsai dispatcher tls v1\n"))
	_, _ = mac.Write([]byte(purpose))
	return mac.Sum(nil)
}

func deterministicSerial(secret, purpose string) *big.Int {
	sum := deriveSeed(secret, purpose)
	serial := new(big.Int).SetBytes(sum[:16])
	if serial.Sign() == 0 {
		serial.SetInt64(1)
	}
	return serial
}

func randomSerial() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return nil, fmt.Errorf("generate certificate serial: %w", err)
	}
	if serial.Sign() == 0 {
		serial.SetInt64(1)
	}
	return serial, nil
}

func serverSubjectAltNames(cfg Config) ([]string, []net.IP) {
	dnsSet := map[string]struct{}{}
	ipSet := map[string]net.IP{}

	addHost(dnsSet, ipSet, defaultServerName)
	addHost(dnsSet, ipSet, "localhost")
	addHost(dnsSet, ipSet, "127.0.0.1")
	addHost(dnsSet, ipSet, "::1")
	addHost(dnsSet, ipSet, cfg.ServerName)
	for _, name := range cfg.ServerNames {
		addHost(dnsSet, ipSet, name)
	}

	dnsNames := make([]string, 0, len(dnsSet))
	for name := range dnsSet {
		dnsNames = append(dnsNames, name)
	}
	sort.Strings(dnsNames)

	ipKeys := make([]string, 0, len(ipSet))
	for key := range ipSet {
		ipKeys = append(ipKeys, key)
	}
	sort.Strings(ipKeys)
	ipAddresses := make([]net.IP, 0, len(ipKeys))
	for _, key := range ipKeys {
		ipAddresses = append(ipAddresses, ipSet[key])
	}

	return dnsNames, ipAddresses
}

func addHost(dnsSet map[string]struct{}, ipSet map[string]net.IP, raw string) {
	host := hostFromAddress(raw)
	if host == "" {
		return
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsUnspecified() {
			return
		}
		ipSet[ip.String()] = ip
		return
	}
	if strings.ContainsAny(host, "/*") {
		return
	}
	dnsSet[strings.ToLower(host)] = struct{}{}
}

func normalizeServerName(raw string) string {
	host := hostFromAddress(raw)
	if host == "" {
		return defaultServerName
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
		return defaultServerName
	}
	return host
}

func hostFromAddress(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "://") {
		if parsed, err := url.Parse(raw); err == nil {
			return strings.Trim(parsed.Hostname(), "[]")
		}
	}
	if host, _, err := net.SplitHostPort(raw); err == nil {
		return strings.Trim(host, "[]")
	}
	return strings.Trim(raw, "[]")
}

func clientServiceURI(role, serviceID string) *url.URL {
	role = strings.Trim(strings.ToLower(strings.TrimSpace(role)), "/")
	serviceID = strings.Trim(strings.TrimSpace(serviceID), "/")
	if role == "" && serviceID == "" {
		return nil
	}
	if role == "" {
		role = "service"
	}
	if serviceID == "" {
		serviceID = role
	}
	return &url.URL{
		Scheme: "spiffe",
		Host:   "nopsai.local",
		Path:   "/" + url.PathEscape(role) + "/" + url.PathEscape(serviceID),
	}
}
