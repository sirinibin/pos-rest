"""
A document ZATCA rejects stays in the store's ICV/PIH chain: the reporting
script hands Go the hash of what it sent and marks it rejected, and only a
ZATCA answer (HTTP 400, or a status other than REPORTED/CLEARED) counts as a
rejection. Run from the repository root:

    python -m unittest discover -s ZatcaPython/tests -v
"""
import base64
import contextlib
import io
import json
import os
import sys
import tempfile
import unittest
from unittest import mock

sys.path.insert(0, os.path.abspath("ZatcaPython"))

import reporting_and_clearance as rc
from utilities.api_helper import ZatcaRejected, api_helper

SIGNED = json.dumps({"invoiceHash": "SIGNED-HASH", "uuid": "u-1", "invoice": "PGludm9pY2Uv"})


class Response:
    def __init__(self, status_code, text):
        self.status_code, self.text = status_code, text


class PostRequestTest(unittest.TestCase):
    def post(self, status, body="{}"):
        with mock.patch("utilities.api_helper.requests.post", return_value=Response(status, body)) as post:
            try:
                return api_helper.post_request_with_retries("https://zatca", {}, "{}"), post
            finally:
                pass

    def test_accepted_and_accepted_with_warnings_pass(self):
        for status in (200, 202):
            text, _ = self.post(status, '{"reportingStatus": "REPORTED"}')
            self.assertIn("REPORTED", text)

    def test_400_is_a_rejection_carrying_zatcas_answer(self):
        with mock.patch("utilities.api_helper.requests.post",
                        return_value=Response(400, '{"validationResults": "BR-KSA-44"}')) as post:
            with self.assertRaises(ZatcaRejected) as ctx:
                api_helper.post_request_with_retries("https://zatca", {}, "{}")
        self.assertIn("BR-KSA-44", ctx.exception.body)
        self.assertEqual(post.call_count, 1, "a rejection is not sent again")

    def test_other_errors_are_not_rejections(self):
        for status in (401, 500, 503):
            with mock.patch("utilities.api_helper.requests.post", return_value=Response(status, "x")):
                with self.assertRaises(Exception) as ctx:
                    api_helper.post_request_with_retries("https://zatca", {}, "{}")
                self.assertNotIsInstance(ctx.exception, ZatcaRejected, status)


class ReportingScriptTest(unittest.TestCase):
    def run_script(self, submit):
        with tempfile.NamedTemporaryFile("w", suffix=".xml", delete=False) as f:
            f.write("<Invoice/>")
        self.addCleanup(os.remove, f.name)
        payload = json.dumps({
            "env": "NonProduction", "private_key": "k", "store_id": "s1",
            "production_binary_security_token": base64.b64encode(b"cert").decode(),
            "production_secret": "secret", "xml_file_path": f.name, "is_simplified": True,
        })
        out = io.StringIO()
        with mock.patch.object(rc.einvoice_signer, "get_request_api", return_value=SIGNED), \
                mock.patch.object(rc.einvoice_signer, "is_simplified_invoice", return_value=True), \
                mock.patch.object(rc.api_helper, "invoice_reporting", side_effect=submit), \
                mock.patch("sys.stdin", io.StringIO(payload)), contextlib.redirect_stdout(out):
            rc.main()
        return json.loads(out.getvalue())

    def test_reported(self):
        got = self.run_script(lambda *a: json.dumps({"reportingStatus": "REPORTED"}))
        self.assertTrue(got["reporting_passed"])
        self.assertEqual(got["invoice_hash"], "SIGNED-HASH")
        self.assertFalse(got.get("rejected", False))

    def test_rejected_by_zatca_keeps_the_signed_hash(self):
        def reject(*a):
            raise ZatcaRejected("HTTP error: 400 - BR-KSA-44", '{"validationResults": {}}')
        got = self.run_script(reject)
        self.assertEqual(got, {"invoice_hash": "SIGNED-HASH", "reporting_passed": False,
                               "rejected": True, "error": "HTTP error: 400 - BR-KSA-44"})

    def test_not_reported_status_is_a_rejection(self):
        got = self.run_script(lambda *a: json.dumps({"reportingStatus": "NOT_REPORTED"}))
        self.assertTrue(got["rejected"])
        self.assertEqual(got["invoice_hash"], "SIGNED-HASH")
        self.assertIn("NOT_REPORTED", got["error"])

    def test_no_answer_from_zatca_is_not_a_rejection(self):
        def down(*a):
            raise Exception("HTTP error: 503")
        got = self.run_script(down)
        self.assertFalse(got["reporting_passed"])
        self.assertFalse(got.get("rejected", False))
        self.assertEqual(got["invoice_hash"], "")


if __name__ == "__main__":
    unittest.main()
