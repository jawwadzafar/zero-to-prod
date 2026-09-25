// Package mtls shows service-to-service identity with mutual TLS (chapter
// 7.11): a tiny certificate authority issues short-lived certificates that
// name each workload with a SPIFFE-style ID (spiffe://snip.local/worker), and
// a server that accepts only callers whose certificate the CA signed and whose
// identity is on its allow list. Standard library only. In production, a
// system such as SPIRE or a service mesh runs the CA and rotates certificates
// automatically; the checks are the same.
package mtls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"slices"
	"time"
)

// CA is a certificate authority: a key pair whose certificate everyone trusts.
type CA struct {
	Cert *x509.Certificate
	key  *ecdsa.PrivateKey
	Pool *x509.CertPool // what verifiers load: "trust certificates signed by this CA"
}

func serial() *big.Int {
	n, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	return n
}

// NewCA creates a root certificate authority.
func NewCA(name string) (*CA, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial(),
		Subject:               pkix.Name{CommonName: name},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	cert, _ := x509.ParseCertificate(der)
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return &CA{Cert: cert, key: key, Pool: pool}, nil
}

// Issue creates a certificate for one workload. Short lifetimes (an hour
// here) mean a stolen certificate is soon useless, which is why automated
// rotation matters more than revocation lists.
func (ca *CA) Issue(spiffeID string, dnsNames []string, lifetime time.Duration) (tls.Certificate, error) {
	id, err := url.Parse(spiffeID)
	if err != nil {
		return tls.Certificate{}, err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial(),
		Subject:      pkix.Name{CommonName: spiffeID},
		URIs:         []*url.URL{id}, // the identity lives in a URI subject alternative name
		DNSNames:     dnsNames,
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(lifetime),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.Cert, &key.PublicKey, ca.key)
	if err != nil {
		return tls.Certificate{}, err
	}
	leaf, _ := x509.ParseCertificate(der)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, nil
}

// ServerTLS makes a server that demands a client certificate signed by ca.
func ServerTLS(ca *CA, cert tls.Certificate) *tls.Config {
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    ca.Pool,
		ClientAuth:   tls.RequireAndVerifyClientCert, // no valid certificate, no connection
		MinVersion:   tls.VersionTLS13,
	}
}

// ClientTLS makes a client that presents cert and trusts only servers signed by ca.
func ClientTLS(ca *CA, cert tls.Certificate) *tls.Config {
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      ca.Pool,
		MinVersion:   tls.VersionTLS13,
	}
}

// PeerID returns the caller's SPIFFE ID from its verified certificate.
func PeerID(r *http.Request) (string, error) {
	if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 {
		return "", errors.New("no verified client certificate")
	}
	leaf := r.TLS.VerifiedChains[0][0]
	for _, u := range leaf.URIs {
		if u.Scheme == "spiffe" {
			return u.String(), nil
		}
	}
	return "", errors.New("certificate has no SPIFFE ID")
}

// Allow wraps a handler: authentication came from TLS; this is authorization,
// by workload identity rather than by network address.
func Allow(allowed []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := PeerID(r)
		if err != nil || !slices.Contains(allowed, id) {
			http.Error(w, "forbidden for "+id, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
