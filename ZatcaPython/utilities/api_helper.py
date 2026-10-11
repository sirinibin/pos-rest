import json
import time
import requests
from requests.auth import HTTPBasicAuth

class api_helper:
    
    # ZATCA answers 200 when a request passes and 202 when it passes with
    # warnings (an invoice REPORTED/CLEARED with warnings is still accepted).
    OK_STATUSES = (200, 202)
    # Answers that mean ZATCA (or Cloudflare in front of it) never handled the
    # request, so sending it again is safe: rate limiting, gateway errors, and
    # Cloudflare's 52x "origin unreachable" family. 504 is left out on purpose:
    # the request may have been processed.
    RETRY_STATUSES = (429, 500, 502, 503, 520, 521, 522, 523, 524)
    TIMEOUT = (10, 60)  # seconds: connect, read
    MAX_RETRY_AFTER = 30

    @staticmethod
    def retry_delay(response, attempt, backoff_factor):
        delay = backoff_factor * (2 ** attempt)
        retry_after = response.headers.get("Retry-After", "") if response is not None else ""
        if retry_after.strip().isdigit():
            delay = max(delay, int(retry_after))
        return min(delay, api_helper.MAX_RETRY_AFTER)

    @staticmethod
    def post_request_with_retries(url, headers, json_payload, auth=None, retries=4, backoff_factor=2, sleep=time.sleep):
        for attempt in range(retries):
            last = attempt == retries - 1
            try:
                response = requests.post(url, headers=headers, data=json_payload, auth=auth, timeout=api_helper.TIMEOUT)
            except (requests.exceptions.ConnectionError, requests.exceptions.ConnectTimeout):
                # The request never reached ZATCA.
                if last:
                    raise
                sleep(api_helper.retry_delay(None, attempt, backoff_factor))
                continue

            if response.status_code in api_helper.OK_STATUSES:
                return response.text
            if response.status_code in api_helper.RETRY_STATUSES and not last:
                sleep(api_helper.retry_delay(response, attempt, backoff_factor))
                continue
            raise Exception(f"HTTP error: {response.status_code} - url={url} headers={dict(response.headers)} body={response.text}")

    @staticmethod
    def compliance_csid(cert_info, retries=4, backoff_factor=2):
        csr = cert_info['csr']
        OTP = cert_info['OTP']
        url = cert_info['complianceCsidUrl']

        json_payload = json.dumps({'csr': csr})
        
        headers = {
            'accept': 'application/json',
            'accept-language': 'en',
            'OTP': OTP,
            'Accept-Version': 'V2',
            'Content-Type': 'application/json',
        }

        return api_helper.post_request_with_retries(url, headers, json_payload, retries=retries, backoff_factor=backoff_factor)

    @staticmethod
    def production_csid(cert_info, retries=4, backoff_factor=2):
        request_id = cert_info['ccsid_requestID']
        id_token = cert_info['ccsid_binarySecurityToken']
        secret = cert_info['ccsid_secret']
        url = cert_info['productionCsidUrl']

        json_payload = json.dumps({'compliance_request_id': request_id})

        headers = {
            'accept': 'application/json',
            'accept-language': 'en',
            'Accept-Version': 'V2',
            'Content-Type': 'application/json',
        }

        auth = HTTPBasicAuth(id_token, secret)
        return api_helper.post_request_with_retries(url, headers, json_payload, auth=auth, retries=retries, backoff_factor=backoff_factor)

    @staticmethod
    def compliance_checks(cert_info, json_payload, retries=4, backoff_factor=2):
        id_token = cert_info['ccsid_binarySecurityToken']
        secret = cert_info['ccsid_secret']
        url = cert_info["complianceChecksUrl"]

        headers = {
            'accept': 'application/json',
            'accept-language': 'en',
            'Accept-Version': 'V2',
            'Content-Type': 'application/json',
        }

        auth = HTTPBasicAuth(id_token, secret)
        return api_helper.post_request_with_retries(url, headers, json_payload, auth=auth, retries=retries, backoff_factor=backoff_factor)

    @staticmethod
    def invoice_reporting(cert_info, json_payload, retries=4, backoff_factor=2):
        id_token = cert_info['pcsid_binarySecurityToken']
        secret = cert_info['pcsid_secret']
        url = cert_info["reportingUrl"]

        headers = {
            'accept': 'application/json',
            'accept-language': 'en',
            'Accept-Version': 'V2',
            'Content-Type': 'application/json',
        }

        auth = HTTPBasicAuth(id_token, secret)
        return api_helper.post_request_with_retries(url, headers, json_payload, auth=auth, retries=retries, backoff_factor=backoff_factor)

    @staticmethod
    def invoice_clearance(cert_info, json_payload, retries=4, backoff_factor=2):
        id_token = cert_info['pcsid_binarySecurityToken']
        secret = cert_info['pcsid_secret']
        url = cert_info["clearanceUrl"]

        headers = {
            'accept': 'application/json',
            'accept-language': 'en',
            'Clearance-Status': '1',
            'Accept-Version': 'V2',
            'Content-Type': 'application/json',
        }

        auth = HTTPBasicAuth(id_token, secret)
        return api_helper.post_request_with_retries(url, headers, json_payload, auth=auth, retries=retries, backoff_factor=backoff_factor)

    @staticmethod
    def load_json_from_file(file_path):
        try:
            with open(file_path, 'r') as file:
                return json.load(file)
        except FileNotFoundError:
            raise Exception(f"File not found: {file_path}")
        except json.JSONDecodeError as e:
            raise Exception(f"Error parsing JSON: {str(e)}")

    @staticmethod
    def save_json_to_file(file_path, data):
        try:
            with open(file_path, 'w') as file:
                json.dump(data, file, indent=4, ensure_ascii=False, separators=(',', ': '))
        except Exception as e:
            raise Exception(f"Error saving JSON: {str(e)}")

    @staticmethod
    def clean_up_json(api_response, request_type, api_url):
        array_response = json.loads(api_response)
        array_response['requestType'] = request_type
        array_response['apiUrl'] = api_url

        array_response = {k: v for k, v in array_response.items() if v is not None}

        reordered_response = {
            'requestType': array_response.pop('requestType'),
            'apiUrl': array_response.pop('apiUrl'),
            **array_response
        }

        return json.dumps(reordered_response, indent=4, ensure_ascii=False, separators=(',', ': '))
