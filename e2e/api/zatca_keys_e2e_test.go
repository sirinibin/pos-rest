//go:build e2e && zatca

package apie2e

import (
	"bytes"
	"crypto/sha256"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"math/big"
)

// Reading the keys ZATCA documents are signed with, to check each store's
// documents carry its own signature. ZATCA uses secp256k1, which Go's crypto
// packages don't parse or verify, so this reads the DER by hand.

// derElems splits a DER SEQUENCE into its elements.
func derElems(der []byte) ([]asn1.RawValue, error) {
	var seq asn1.RawValue
	if _, err := asn1.Unmarshal(der, &seq); err != nil {
		return nil, err
	}
	var out []asn1.RawValue
	for rest := seq.Bytes; len(rest) > 0; {
		var e asn1.RawValue
		var err error
		if rest, err = asn1.Unmarshal(rest, &e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

type signerKey struct {
	spki    []byte // SubjectPublicKeyInfo DER
	subject string
	x, y    *big.Int
}

func keyFromSPKI(spki, subject []byte) (*signerKey, error) {
	k := &signerKey{spki: spki}
	var name pkix.RDNSequence
	if _, err := asn1.Unmarshal(subject, &name); err == nil {
		var n pkix.Name
		n.FillFromRDNSequence(&name)
		k.subject = n.String()
	}
	var info struct {
		Alg asn1.RawValue
		Key asn1.BitString
	}
	if _, err := asn1.Unmarshal(spki, &info); err != nil {
		return nil, err
	}
	if b := info.Key.Bytes; len(b) == 65 && b[0] == 4 {
		k.x, k.y = new(big.Int).SetBytes(b[1:33]), new(big.Int).SetBytes(b[33:])
	}
	return k, nil
}

// certKey reads the key and subject of a base64 DER certificate (the
// document's x509_digital_certificate).
func certKey(b64 string) (*signerKey, error) {
	der, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, err
	}
	cert, err := derElems(der)
	if err != nil || len(cert) == 0 {
		return nil, errors.New("not a certificate")
	}
	tbs, err := derElems(cert[0].FullBytes)
	if err != nil {
		return nil, err
	}
	if len(tbs) > 0 && tbs[0].Class == asn1.ClassContextSpecific {
		tbs = tbs[1:] // version
	}
	if len(tbs) < 6 {
		return nil, errors.New("short certificate")
	}
	return keyFromSPKI(tbs[5].FullBytes, tbs[4].FullBytes)
}

// csrKey reads the key and subject of a store's CSR (base64 of the PEM).
func csrKey(b64pem string) (*signerKey, error) {
	raw, err := base64.StdEncoding.DecodeString(b64pem)
	if err != nil {
		return nil, err
	}
	der := raw
	if blk, _ := pem.Decode(raw); blk != nil {
		der = blk.Bytes
	}
	csr, err := derElems(der)
	if err != nil || len(csr) == 0 {
		return nil, errors.New("not a CSR")
	}
	info, err := derElems(csr[0].FullBytes)
	if err != nil || len(info) < 3 {
		return nil, errors.New("short CSR")
	}
	return keyFromSPKI(info[2].FullBytes, info[1].FullBytes)
}

func (k *signerKey) same(o *signerKey) bool {
	return k != nil && o != nil && bytes.Equal(k.spki, o.spki)
}

var (
	k1P, _  = new(big.Int).SetString("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEFFFFFC2F", 16)
	k1N, _  = new(big.Int).SetString("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEBAAEDCE6AF48A03BBFD25E8CD0364141", 16)
	k1Gx, _ = new(big.Int).SetString("79BE667EF9DCBBAC55A06295CE870B07029BFCDB2DCE28D959F2815B16F81798", 16)
	k1Gy, _ = new(big.Int).SetString("483ADA7726A3C4655DA4FBFC0E1108A8FD17B448A68554199C47D08FFB10D4B8", 16)
)

// k1Add adds two secp256k1 points (nil is the point at infinity).
func k1Add(x1, y1, x2, y2 *big.Int) (*big.Int, *big.Int) {
	if x1 == nil {
		return x2, y2
	}
	if x2 == nil {
		return x1, y1
	}
	var l *big.Int
	if x1.Cmp(x2) == 0 {
		if new(big.Int).Mod(new(big.Int).Add(y1, y2), k1P).Sign() == 0 {
			return nil, nil
		}
		num := new(big.Int).Mul(big.NewInt(3), new(big.Int).Mul(x1, x1))
		den := new(big.Int).ModInverse(new(big.Int).Lsh(y1, 1), k1P)
		l = num.Mul(num, den)
	} else {
		num := new(big.Int).Sub(y2, y1)
		den := new(big.Int).ModInverse(new(big.Int).Mod(new(big.Int).Sub(x2, x1), k1P), k1P)
		l = num.Mul(num, den)
	}
	l.Mod(l, k1P)
	x3 := new(big.Int).Sub(new(big.Int).Mul(l, l), new(big.Int).Add(x1, x2))
	x3.Mod(x3, k1P)
	y3 := new(big.Int).Sub(new(big.Int).Mul(l, new(big.Int).Sub(x1, x3)), y1)
	y3.Mod(y3, k1P)
	return x3, y3
}

func k1Mul(x, y, k *big.Int) (*big.Int, *big.Int) {
	var rx, ry *big.Int
	for i := k.BitLen() - 1; i >= 0; i-- {
		rx, ry = k1Add(rx, ry, rx, ry)
		if k.Bit(i) == 1 {
			rx, ry = k1Add(rx, ry, x, y)
		}
	}
	return rx, ry
}

// verifiesHash reports whether sigB64 (base64 DER ECDSA) is k's ECDSA-SHA256
// signature over the document's invoice hash (base64), as ZATCA signs it.
func (k *signerKey) verifiesHash(invoiceHashB64, sigB64 string) bool {
	if k == nil || k.x == nil {
		return false
	}
	msg, err1 := base64.StdEncoding.DecodeString(invoiceHashB64)
	sig, err2 := base64.StdEncoding.DecodeString(sigB64)
	var rs struct{ R, S *big.Int }
	if err1 != nil || err2 != nil {
		return false
	}
	if _, err := asn1.Unmarshal(sig, &rs); err != nil || rs.R.Sign() <= 0 || rs.S.Sign() <= 0 {
		return false
	}
	d := sha256.Sum256(msg)
	e := new(big.Int).SetBytes(d[:])
	w := new(big.Int).ModInverse(rs.S, k1N)
	if w == nil {
		return false
	}
	u1 := new(big.Int).Mod(new(big.Int).Mul(e, w), k1N)
	u2 := new(big.Int).Mod(new(big.Int).Mul(rs.R, w), k1N)
	x1, y1 := k1Mul(k1Gx, k1Gy, u1)
	x2, y2 := k1Mul(k.x, k.y, u2)
	X, _ := k1Add(x1, y1, x2, y2)
	return X != nil && new(big.Int).Mod(X, k1N).Cmp(rs.R) == 0
}
