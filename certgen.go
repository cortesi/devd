package devd

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"time"
)

// GenerateCert generates a self-signed certificate bundle for devd
func GenerateCert(dst string) error {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	notBefore := time.Now()
	notAfter := notBefore.Add(365 * 24 * time.Hour * 3)

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Acme Co"},
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	template.DNSNames = append(template.DNSNames, "devd.io")
	template.DNSNames = append(template.DNSNames, "*.devd.io")

	derBytes, err := x509.CreateCertificate(
		rand.Reader,
		&template,
		&template,
		&priv.PublicKey,
		priv,
	)
	if err != nil {
		return fmt.Errorf("Could not create cert: %s", err)
	}

	certOut, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("Could not open %s for writing: %s", dst, err)
	}
	err = pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	if err != nil {
		return err
	}

	err = certOut.Close()
	if err != nil {
		return err
	}

	keyOut, err := os.OpenFile(dst, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("Could not open %s for writing: %s", dst, err)
	}
	err = pem.Encode(
		keyOut,
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(priv),
		},
	)
	if err != nil {
		return err
	}

	err = keyOut.Close()
	if err != nil {
		return err
	}
	return nil
}
