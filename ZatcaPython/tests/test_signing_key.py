"""
Signing-key tests for the ZATCA signer. Run from the repository root:

    python -m unittest discover -s ZatcaPython/tests -v

The round trip through the Fatoora SDK runs when the SDK is unpacked at
ZatcaPython/utilities/fatoora-cli-simulation (CI takes it from the public
sirinibin/zatca-sdk repo) and java is on PATH; otherwise it is skipped.
"""
import base64
import datetime
import json
import os
import shutil
import sys
import unittest

sys.path.insert(0, os.path.abspath("ZatcaPython"))

from cryptography import x509
from cryptography.exceptions import InvalidSignature
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.x509.oid import NameOID

from utilities.einvoice_signer import einvoice_signer, sdk_private_key, signature_value

SDK_HOME = "ZatcaPython/utilities/fatoora-cli-simulation"
SAMPLE = os.path.join(SDK_HOME, "Data/Samples/Simplified/Invoice/Simplified_Invoice.xml")


def pem_body(pem):
    return "".join(pem.strip().splitlines()[1:-1])


def new_key():
    return ec.generate_private_key(ec.SECP256K1())


def key_text(key, fmt):
    return key.private_bytes(serialization.Encoding.PEM, fmt, serialization.NoEncryption()).decode()


def same_key(a, b):
    spki = lambda k: k.public_key().public_bytes(
        serialization.Encoding.DER, serialization.PublicFormat.SubjectPublicKeyInfo)
    return spki(a) == spki(b)


def load_sec1(b64):
    return serialization.load_der_private_key(base64.b64decode(b64), password=None)


class SdkPrivateKeyTest(unittest.TestCase):
    def test_pkcs8_becomes_sec1_of_the_same_key(self):
        key = new_key()
        out = sdk_private_key(pem_body(key_text(key, serialization.PrivateFormat.PKCS8)))
        self.assertTrue(base64.b64decode(out).startswith(b"\x30\x74\x02\x01\x01"), "not SEC1 DER")
        self.assertTrue(same_key(load_sec1(out), key))

    def test_sec1_stays_the_same(self):
        key = new_key()
        body = pem_body(key_text(key, serialization.PrivateFormat.TraditionalOpenSSL))
        self.assertEqual(sdk_private_key(body), body)

    def test_pem_with_header_lines(self):
        key = new_key()
        out = sdk_private_key(key_text(key, serialization.PrivateFormat.PKCS8))
        self.assertTrue(same_key(load_sec1(out), key))

    def test_not_a_key(self):
        with self.assertRaises(Exception):
            sdk_private_key("bm90IGEga2V5")

    def test_signature_value(self):
        self.assertEqual(signature_value("<ds:SignatureValue></ds:SignatureValue>"), "")
        self.assertEqual(signature_value("<x/>"), "")
        self.assertEqual(signature_value("<ds:SignatureValue>\n  MEUC==\n</ds:SignatureValue>"), "MEUC==")


@unittest.skipUnless(os.path.exists(SAMPLE) and shutil.which("java"), "Fatoora SDK or java not available")
class SdkSigningTest(unittest.TestCase):
    """The key the SDK's own -csr command writes (PKCS#8) must sign invoices."""

    def sign(self, key_body, key):
        name = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, "signing-key-test")])
        now = datetime.datetime.now(datetime.timezone.utc)
        cert = (x509.CertificateBuilder().subject_name(name).issuer_name(name)
                .public_key(key.public_key()).serial_number(1)
                .not_valid_before(now).not_valid_after(now + datetime.timedelta(days=1))
                .sign(key, hashes.SHA256()))
        cert_body = pem_body(cert.public_bytes(serialization.Encoding.PEM).decode())
        with open(SAMPLE) as f:
            xml = f.read()
        out = json.loads(einvoice_signer.sign_simplified_invoice(
            xml, "NonProduction", key_body, cert_body, "signing-key-test"))
        return out, signature_value(base64.b64decode(out["invoice"]).decode())

    def tearDown(self):
        shutil.rmtree("ZatcaPython/store-data/signing-key-test", ignore_errors=True)

    def check(self, fmt):
        key = new_key()
        out, sig = self.sign(pem_body(key_text(key, fmt)), key)
        self.assertNotEqual(sig, "", "SDK left SignatureValue empty")
        try:
            key.public_key().verify(base64.b64decode(sig), base64.b64decode(out["invoiceHash"]),
                                    ec.ECDSA(hashes.SHA256()))
        except InvalidSignature:
            self.fail("signature does not verify with the store's key")

    def test_pkcs8_key_signs(self):
        self.check(serialization.PrivateFormat.PKCS8)

    def test_sec1_key_signs(self):
        self.check(serialization.PrivateFormat.TraditionalOpenSSL)


if __name__ == "__main__":
    unittest.main()
