"""
Tests for the ZATCA HTTP helper: which answers are accepted, which are
retried, and that a request is never sent again once ZATCA may have
handled it. Run from the repository root:

    python -m unittest discover -s ZatcaPython/tests -v
"""
import os
import sys
import unittest
from unittest import mock

sys.path.insert(0, os.path.abspath("ZatcaPython"))

import requests

from utilities.api_helper import api_helper


class FakeResponse:
    def __init__(self, status_code, text="{}", headers=None):
        self.status_code = status_code
        self.text = text
        self.headers = headers or {}


class PostWithRetriesTest(unittest.TestCase):
    def post(self, *answers, retries=4):
        """Calls post_request_with_retries with requests.post answering
        `answers` in turn (a FakeResponse, or an exception to raise)."""
        self.delays = []
        with mock.patch.object(requests, "post", side_effect=list(answers)) as post:
            try:
                return api_helper.post_request_with_retries(
                    "https://gw-fatoora.zatca.gov.sa/x", {}, "{}", retries=retries, sleep=self.delays.append)
            finally:
                self.calls = post.call_count
                self.kwargs = post.call_args.kwargs if post.call_args else {}

    def test_200_is_returned(self):
        self.assertEqual(self.post(FakeResponse(200, '{"reportingStatus":"REPORTED"}')), '{"reportingStatus":"REPORTED"}')
        self.assertEqual(self.calls, 1)

    def test_202_accepted_with_warnings_is_returned(self):
        self.assertEqual(self.post(FakeResponse(202, '{"reportingStatus":"REPORTED"}')), '{"reportingStatus":"REPORTED"}')
        self.assertEqual(self.calls, 1)

    def test_every_request_has_a_timeout(self):
        self.post(FakeResponse(200))
        self.assertEqual(self.kwargs.get("timeout"), api_helper.TIMEOUT)

    def test_cloudflare_522_is_retried_until_it_passes(self):
        self.assertEqual(self.post(FakeResponse(522), FakeResponse(522), FakeResponse(200, "ok")), "ok")
        self.assertEqual(self.calls, 3)
        self.assertEqual(self.delays, [2, 4])

    def test_each_transient_status_is_retried(self):
        for status in api_helper.RETRY_STATUSES:
            with self.subTest(status=status):
                self.assertEqual(self.post(FakeResponse(status), FakeResponse(200, "ok")), "ok")
                self.assertEqual(self.calls, 2)

    def test_retry_after_is_honoured_but_capped(self):
        self.post(FakeResponse(429, headers={"Retry-After": "7"}), FakeResponse(200))
        self.assertEqual(self.delays, [7])
        self.post(FakeResponse(503, headers={"Retry-After": "120"}), FakeResponse(200))
        self.assertEqual(self.delays, [api_helper.MAX_RETRY_AFTER])

    def test_gives_up_after_the_last_attempt_with_the_status(self):
        with self.assertRaisesRegex(Exception, "HTTP error: 522"):
            self.post(*[FakeResponse(522)] * 4)
        self.assertEqual(self.calls, 4)

    def test_client_errors_are_not_retried(self):
        for status in (400, 401, 403, 404, 409):
            with self.subTest(status=status):
                with self.assertRaisesRegex(Exception, f"HTTP error: {status}"):
                    self.post(FakeResponse(status, '{"message":"Invalid-OTP"}'))
                self.assertEqual(self.calls, 1)

    def test_504_is_not_retried(self):
        # ZATCA may have handled the request; sending it again could
        # report an invoice twice.
        with self.assertRaisesRegex(Exception, "HTTP error: 504"):
            self.post(FakeResponse(504))
        self.assertEqual(self.calls, 1)

    def test_connection_errors_are_retried(self):
        self.assertEqual(self.post(requests.exceptions.ConnectionError("down"), FakeResponse(200, "ok")), "ok")
        self.assertEqual(self.calls, 2)
        with self.assertRaises(requests.exceptions.ConnectionError):
            self.post(*[requests.exceptions.ConnectionError("down")] * 4)

    def test_read_timeout_is_not_retried(self):
        with self.assertRaises(requests.exceptions.ReadTimeout):
            self.post(requests.exceptions.ReadTimeout("slow"), FakeResponse(200))
        self.assertEqual(self.calls, 1)


if __name__ == "__main__":
    unittest.main()
