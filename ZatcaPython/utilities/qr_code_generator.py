import base64
import os
import re
import tempfile
import xml.etree.ElementTree as ET
from OpenSSL import crypto

class qr_code_generator:

    @staticmethod
    def generate_qr_code(canonical_xml, invoice_hash, signature_value, ecdsa_result):
        invoice_details = qr_code_generator.get_invoice_details(canonical_xml) #, invoice_hash, signature_value)

        # Retrieve the InvoiceTypeCode name (from position 8 in array)
        #invoice_type_code_name = invoice_details[8]
        invoice_details.append(invoice_hash)
        invoice_details.append(signature_value)
        #result = qr_code_generator.get_public_key_and_signature(x509_certificate_content)
        invoice_details.append(ecdsa_result['public_key'])
        # Only add certificateSignature if InvoiceTypeCode name starts with "02"
        #if invoice_type_code_name.startswith("02"):
        invoice_details.append(ecdsa_result['signature'])
        
        base64_qr_code = qr_code_generator.generate_qr_code_from_values(invoice_details)

        return base64_qr_code

    @staticmethod
    def get_invoice_details(xml): #, invoice_hash, signature_value)
        xml_object = ET.fromstring(xml)

        invoice_type_code = xml_object.find('.//{urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2}InvoiceTypeCode').text
        invoice_type_code_name = xml_object.find('.//{urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2}InvoiceTypeCode').get('name')
        supplier_name = xml_object.find('.//{urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2}AccountingSupplierParty/{urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2}Party/{urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2}PartyLegalEntity/{urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2}RegistrationName').text
        company_id = xml_object.find('.//{urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2}AccountingSupplierParty/{urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2}Party/{urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2}PartyTaxScheme/{urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2}CompanyID').text
        issue_date_time = xml_object.find('.//{urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2}IssueDate').text + 'T' + xml_object.find('.//{urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2}IssueTime').text
        payable_amount = xml_object.find('.//{urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2}LegalMonetaryTotal/{urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2}PayableAmount').text
        tax_amount = xml_object.find('.//{urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2}TaxTotal/{urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2}TaxAmount').text

        return [
            None,  # Index 0 is unused
            supplier_name,
            company_id,
            issue_date_time,
            payable_amount,
            tax_amount
            #invoice_hash,
            #signature_value,
            #invoice_type_code_name
        ]

    @staticmethod
    def generate_qr_code_from_values(invoice_details):
        data = b''  
        for key, value in enumerate(invoice_details[1:], start=1):
            if isinstance(value, str):
                value = value.encode('utf-8')
            
            tlv_data = qr_code_generator.write_tlv(key, value)
            data += tlv_data
            
         # Ensure to check if data is empty
        if not data:
            print("No data generated for QR code!")
        
        return base64.b64encode(data).decode()

    @staticmethod
    def write_length(length):
        if length <= 0x7F:
            return bytes([length])
        length_bytes = []
        while length > 0:
            length_bytes.insert(0, length & 0xFF)
            length >>= 8
        return bytes([0x80 | len(length_bytes)]) + bytes(length_bytes)

    @staticmethod
    def write_tag(tag):
        result = bytes()
        flag = True
        for i in range(3, -1, -1):
            num = (tag >> (8 * i)) & 0xFF
            if num != 0 or not flag or i == 0:
                if flag and i != 0 and (num & 0x1F) != 0x1F:
                    raise ValueError(f"Invalid tag value: {tag}")
                result += bytes([num])
                flag = False
        return result

    @staticmethod
    def write_tlv(tag, value):
        if value is None:
            raise ValueError("Please provide a value!")
        tlv = qr_code_generator.write_tag(tag)
        length = len(value)
        tlv += qr_code_generator.write_length(length)
        tlv += value
        return tlv

    @staticmethod
    def get_public_key_and_signature(certificate_base64):
        try:
            with tempfile.NamedTemporaryFile(delete=False, mode='w', suffix='.pem') as temp_file:
                cert_content = "-----BEGIN CERTIFICATE-----\n"
                cert_content += "\n".join([certificate_base64[i:i+64] for i in range(0, len(certificate_base64), 64)])
                cert_content += "\n-----END CERTIFICATE-----\n"
                temp_file.write(cert_content)
                temp_file_path = temp_file.name

            with open(temp_file_path, 'r') as f:
                cert = crypto.load_certificate(crypto.FILETYPE_PEM, f.read())

            pub_key = crypto.dump_publickey(crypto.FILETYPE_ASN1, cert.get_pubkey())
            pub_key_details = crypto.load_publickey(crypto.FILETYPE_ASN1, pub_key).to_cryptography_key().public_numbers()

            x = pub_key_details.x.to_bytes(32, byteorder='big').rjust(32, b'\0')
            y = pub_key_details.y.to_bytes(32, byteorder='big').rjust(32, b'\0')

            public_key_der = b'\x30\x56\x30\x10\x06\x07\x2A\x86\x48\xCE\x3D\x02\x01\x06\x05\x2B\x81\x04\x00\x0A\x03\x42\x00\x04' + x + y

            cert_pem = open(temp_file_path, 'r').read()
            matches = re.search(r'-----BEGIN CERTIFICATE-----(.+)-----END CERTIFICATE-----', cert_pem, re.DOTALL)
            if not matches:
                raise Exception("Error extracting DER data from certificate.")
            
            der_data = base64.b64decode(matches.group(1).replace('\n', ''))
            sequence_pos = der_data.rfind(b'\x30', -72)
            signature = der_data[sequence_pos:]

            return {
                'public_key': public_key_der,
                'signature': signature
            }
        except Exception as e:
            raise Exception("[Error] Failed to process certificate: " + str(e))
        finally:
            if os.path.exists(temp_file_path):
                os.unlink(temp_file_path)
