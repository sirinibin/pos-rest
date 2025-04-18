import base64
import hashlib
import json
import os
import re
import tempfile
from lxml import etree
from datetime import datetime,timezone
from cryptography import x509
from cryptography.hazmat.primitives import serialization, hashes
from cryptography.hazmat.primitives.asymmetric import padding
from cryptography.hazmat.backends import default_backend
from OpenSSL import crypto
from utilities.qr_code_generator import qr_code_generator
from cryptography.hazmat.primitives.asymmetric import ec
import pytz

class einvoice_signer:
    @staticmethod
    def pretty_print_xml(xml):
        """Pretty print the XML."""
        # Convert the XML to a string and then parse it again to pretty print
        xml_string = etree.tostring(xml, pretty_print=True, encoding='UTF-8').decode('UTF-8')
        return etree.fromstring(xml_string)

    # @staticmethod
    # def get_request_api_from_file(xml_file_path, x509_certificate_content, private_key_content):
    #     # Open XML document with preserveWhiteSpace = true
    #     parser = etree.XMLParser(remove_blank_text=False)
    #     xml = etree.parse(xml_file_path, parser)
    #     return einvoice_signer.get_request_api(xml, x509_certificate_content, private_key_content)
    @staticmethod
    def get_request_api_from_file(xml_file_path, x509_certificate_content, private_key_content):
        # Open XML document with preserveWhiteSpace = true
        parser = etree.XMLParser(remove_blank_text=False)
        xml = etree.parse(xml_file_path, parser)
        
        # Pretty print the XML before transformation
        xml = einvoice_signer.pretty_print_xml(xml)
        
        return einvoice_signer.get_request_api(xml, x509_certificate_content, private_key_content)

    # @staticmethod
    # def get_request_api(xml, x509_certificate_content, private_key_content):
    #     """Main function to process the invoice request."""
    #     # Define resource file paths
    #     resource_paths = {
    #         "xsl_file": 'resources/xslfile.xsl',
    #         "ubl_template": 'resources/zatca_ubl.xml',
    #         "signature": 'resources/zatca_signature.xml'
    #     }

    #     xml_declaration = '<?xml version="1.0" encoding="UTF-8"?>'
        
    #     # Extract UUID from XML
    #     uuid = einvoice_signer.extract_uuid(xml)

    #     # Determine if the invoice is simplified
    #     is_simplified_invoice = einvoice_signer.is_simplified_invoice(xml)

    #     # Transform the XML using XSLT
    #     transformed_xml = einvoice_signer.transform_xml(xml, resource_paths["xsl_file"])

    #     # Canonicalize the transformed XML
    #     canonical_xml = einvoice_signer.canonicalize_xml(transformed_xml)

    #     # Generate the hash and encode the invoice
    #     base64_hash = einvoice_signer.generate_base64_hash(canonical_xml)
    #     base64_invoice = einvoice_signer.encode_invoice(xml_declaration, canonical_xml)

    #     # Prepare the result for non-simplified invoices
    #     if not is_simplified_invoice:
    #         return einvoice_signer.create_result(uuid, base64_hash, base64_invoice)

    #     # Sign the simplified invoice
    #     return einvoice_signer.sign_simplified_invoice(canonical_xml, base64_hash, x509_certificate_content, private_key_content, resource_paths["ubl_template"], resource_paths["signature"], uuid)

    @staticmethod
    def get_request_api(xml, x509_certificate_content, private_key_content,basePath):
        """Main function to process the invoice request."""
        # Define resource file paths
        resource_paths = {
            "xsl_file": basePath+'resources/xslfile.xsl',
            "ubl_template": basePath+'resources/zatca_ubl.xml',
            "signature": basePath+'resources/zatca_signature.xml'
        }

        xml_declaration = '<?xml version="1.0" encoding="UTF-8"?>'
        
        
        # Extract UUID from XML
        uuid = einvoice_signer.extract_uuid(xml)

        # Determine if the invoice is simplified
        is_simplified_invoice = einvoice_signer.is_simplified_invoice(xml)

        # Transform the XML using XSLT
        transformed_xml = einvoice_signer.transform_xml(xml, resource_paths["xsl_file"])

        # Canonicalize the transformed XML
        canonical_xml = einvoice_signer.canonicalize_xml(transformed_xml)
        #print (canonical_xml)

        # Generate the hash and encode the invoice
        base64_hash = einvoice_signer.generate_base64_hash(canonical_xml)
        base64_invoice = einvoice_signer.encode_invoice(xml_declaration, canonical_xml)

        # Prepare the result for non-simplified invoices
        if not is_simplified_invoice:
            return einvoice_signer.create_result(uuid, base64_hash, base64_invoice)

        # Sign the simplified invoice
        return einvoice_signer.sign_simplified_invoice(canonical_xml, base64_hash, x509_certificate_content, private_key_content, resource_paths["ubl_template"], resource_paths["signature"], uuid)

    @staticmethod
    def extract_uuid(xml):
        """Extract UUID from the XML document."""
        uuid_nodes = xml.xpath('//cbc:UUID', namespaces={'cbc': 'urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2'})
        if not uuid_nodes:
            raise Exception("UUID not found in the XML document.")
        return uuid_nodes[0].text

    @staticmethod
    def is_simplified_invoice(xml):
        """Check if the invoice is a simplified invoice."""
        invoice_type_code_nodes = xml.xpath('//cbc:InvoiceTypeCode', namespaces={'cbc': 'urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2'})
        if invoice_type_code_nodes:
            name_attribute = invoice_type_code_nodes[0].get('name')
            return name_attribute.startswith("02")
        return False

    @staticmethod
    def transform_xml(xml, xsl_file_path):
        """Apply XSL transformation to the XML."""
        xsl = etree.parse(xsl_file_path)
        transform = etree.XSLT(xsl)
        transformed_xml = transform(xml)
        if transformed_xml is None:
            raise Exception("XSL Transformation failed.")
        return transformed_xml

    @staticmethod
    def canonicalize_xml(transformed_xml):
        """Canonicalize the transformed XML."""
        return etree.tostring(transformed_xml, method='c14n').decode('utf-8')

    @staticmethod
    def generate_base64_hash(canonical_xml):
        """Generate a Base64-encoded hash of the canonical XML."""
        hash_bytes = hashlib.sha256(canonical_xml.encode('utf-8')).digest()
        return base64.b64encode(hash_bytes).decode()

    @staticmethod
    def encode_invoice(xml_declaration, canonical_xml):
        """Encode the invoice as Base64."""
        updated_xml = f"{xml_declaration}\n{canonical_xml}"
        return base64.b64encode(updated_xml.encode('utf-8')).decode('utf-8')

    @staticmethod
    def create_result(uuid, base64_hash, base64_invoice):
        """Create the result dictionary."""
        return json.dumps({
            "invoiceHash": base64_hash,
            "uuid": uuid,
            "invoice": base64_invoice
        })

    @staticmethod
    def sign_simplified_invoice(canonical_xml, base64_hash, x509_certificate_content, private_key_content, ubl_template_path, signature_path, uuid):
        """Sign the simplified invoice and return the signed invoice."""
        # Get current time in UTC (timezone-aware)
      
        utc_now = datetime.now(timezone.utc)

        # Convert to Saudi Arabia timezone (UTC+3)
        saudi_tz = pytz.timezone("Asia/Riyadh")
        saudi_time = utc_now.astimezone(saudi_tz)

        # Format the timestamp
        signature_timestamp = saudi_time.strftime("%Y-%m-%dT%H:%M:%S")

        
        #signature_timestamp = datetime.now().strftime("%Y-%m-%dT%H:%M:%S")

        # Generate public key hashing
        public_key_hashing = einvoice_signer.generate_public_key_hashing(x509_certificate_content)
        #print("public_key_hashing:"+public_key_hashing)
        

        # Parse the X.509 certificate
        pem_certificate = einvoice_signer.wrap_certificate(x509_certificate_content)
        certificate = x509.load_pem_x509_certificate(pem_certificate.encode(), default_backend())

        # Extract certificate information
        issuer_name = einvoice_signer.get_issuer_name(certificate)
        serial_number = certificate.serial_number 
        signed_properties_hash = einvoice_signer.get_signed_properties_hash(signature_timestamp, public_key_hashing, issuer_name, serial_number)
        signature_value = einvoice_signer.get_digital_signature(base64_hash, private_key_content)

        # Generate the ECDSA result
        ecdsa_result = einvoice_signer.get_public_key_and_signature(x509_certificate_content)

        # Populate UBL Template
        ubl_content = einvoice_signer.populate_ubl_template(ubl_template_path, base64_hash, signed_properties_hash, signature_value, x509_certificate_content, signature_timestamp, public_key_hashing, issuer_name, serial_number)

        # Insert UBL into XML
        updated_xml_string = einvoice_signer.insert_ubl_into_xml(canonical_xml, ubl_content)

        # Generate QR Code, now including ecdsa_result
        qr_code = qr_code_generator.generate_qr_code(canonical_xml, base64_hash, signature_value, ecdsa_result)

        # Load and insert signature content
        updated_xml_string = einvoice_signer.insert_signature_into_xml(updated_xml_string, signature_path, qr_code)

           # Encode the final invoice
        base64_invoice = einvoice_signer.encode_invoice('<?xml version="1.0" encoding="UTF-8"?>\n', updated_xml_string)

        # Create the result dictionary
        return json.dumps({
            "invoiceHash": base64_hash,
            "uuid": uuid,
            "invoice": base64_invoice,
        })

    @staticmethod
    def wrap_certificate(x509_certificate_content):
        """Wrap the certificate content with PEM headers and footers."""
        return "-----BEGIN CERTIFICATE-----\n" + \
               "\n".join([x509_certificate_content[i:i + 64] for i in range(0, len(x509_certificate_content), 64)]) + \
               "\n-----END CERTIFICATE-----"

    @staticmethod
    def generate_public_key_hashing(x509_certificate_content):
        """Generate public key hashing from the X.509 certificate content."""
        # Generate the SHA256 hash of the x509_certificate_content string in binary format
        hash_bytes = hashlib.sha256(x509_certificate_content.encode('utf-8')).digest()
        # Convert the hash to hex and then base64 encode the result
        hash_hex = hash_bytes.hex()
        return base64.b64encode(hash_hex.encode('utf-8')).decode('utf-8')

    @staticmethod
    def populate_ubl_template(ubl_template_path, base64_hash, signed_properties_hash, signature_value, x509_certificate_content, signature_timestamp, public_key_hashing, issuer_name, serial_number):
        """Populate the UBL template with necessary values."""
        with open(ubl_template_path, 'r') as ubl_file:
            ubl_content = ubl_file.read()
            ubl_content = ubl_content.replace("INVOICE_HASH", base64_hash)
            ubl_content = ubl_content.replace("SIGNED_PROPERTIES", signed_properties_hash)
            ubl_content = ubl_content.replace("SIGNATURE_VALUE", signature_value)
            ubl_content = ubl_content.replace("CERTIFICATE_CONTENT", x509_certificate_content)
            ubl_content = ubl_content.replace("SIGNATURE_TIMESTAMP", signature_timestamp)
            ubl_content = ubl_content.replace("PUBLICKEY_HASHING", public_key_hashing)
            ubl_content = ubl_content.replace("ISSUER_NAME", issuer_name)
            ubl_content = ubl_content.replace("SERIAL_NUMBER", str(serial_number))

        return ubl_content

    @staticmethod
    def insert_ubl_into_xml(canonical_xml, ubl_content):
        """Insert UBL content into the canonical XML."""
        insert_position = canonical_xml.find('>') + 1  # Find position after the first '>'
        return canonical_xml[:insert_position] + ubl_content + canonical_xml[insert_position:]

    @staticmethod
    def insert_signature_into_xml(updated_xml_string, signature_path, qr_code):
        """Insert the signature content into the XML."""
        with open(signature_path, 'r') as signature_file:
            signature_content = signature_file.read()
            signature_content = signature_content.replace("BASE64_QRCODE", qr_code)

        # Insert signature string before <cac:AccountingSupplierParty>
        insert_position_signature = updated_xml_string.find('<cac:AccountingSupplierParty>')
        if insert_position_signature != -1:
            return updated_xml_string[:insert_position_signature] + signature_content + updated_xml_string[insert_position_signature:]
        else:
            raise Exception("The <cac:AccountingSupplierParty> tag was not found in the XML.")
        
    @staticmethod
    def get_issuer_name(certificate):
        issuer = certificate.issuer
        issuer_name_parts = []

        # Convert issuer to a dictionary-like structure
        issuer_dict = {}
        for attr in issuer:
            key = attr.oid._name
            if key in issuer_dict:
                if isinstance(issuer_dict[key], list):
                    issuer_dict[key].append(attr.value)
                else:
                    issuer_dict[key] = [issuer_dict[key], attr.value]
            else:
                issuer_dict[key] = attr.value

        # Check for 'CN' and add to the issuer name parts
        if 'commonName' in issuer_dict:
            issuer_name_parts.append(f"CN={issuer_dict['commonName']}")

        # Check for 'DC' (Domain Component) if it exists
        if 'domainComponent' in issuer_dict:
            dc_list = issuer_dict['domainComponent']
            if isinstance(dc_list, list):
                # Reverse the DC list to get them in the required order
                dc_list.reverse()
                for dc in dc_list:
                    if dc:  # Check if the DC is not empty
                        issuer_name_parts.append(f"DC={dc}")
            #else:
                #issuer_name_parts.append(f"DC={dc_list}")

        # Join the parts with a comma and return
        return ", ".join(issuer_name_parts)

    @staticmethod
    def get_signed_properties_hash(signing_time, digest_value, x509_issuer_name, x509_serial_number):
    # Construct the XML string with exactly 36 spaces in front of <xades:SignedSignatureProperties>
        xml_string = (
            '<xades:SignedProperties xmlns:xades="http://uri.etsi.org/01903/v1.3.2#" Id="xadesSignedProperties">\n'
            '                                    <xades:SignedSignatureProperties>\n'
            '                                        <xades:SigningTime>{}</xades:SigningTime>\n'.format(signing_time) +
            '                                        <xades:SigningCertificate>\n'
            '                                            <xades:Cert>\n'
            '                                                <xades:CertDigest>\n'
            '                                                    <ds:DigestMethod xmlns:ds="http://www.w3.org/2000/09/xmldsig#" Algorithm="http://www.w3.org/2001/04/xmlenc#sha256"/>\n'
            '                                                    <ds:DigestValue xmlns:ds="http://www.w3.org/2000/09/xmldsig#">{}</ds:DigestValue>\n'.format(digest_value) +
            '                                                </xades:CertDigest>\n'
            '                                                <xades:IssuerSerial>\n'
            '                                                    <ds:X509IssuerName xmlns:ds="http://www.w3.org/2000/09/xmldsig#">{}</ds:X509IssuerName>\n'.format(x509_issuer_name) +
            '                                                    <ds:X509SerialNumber xmlns:ds="http://www.w3.org/2000/09/xmldsig#">{}</ds:X509SerialNumber>\n'.format(x509_serial_number) +
            '                                                </xades:IssuerSerial>\n'
            '                                            </xades:Cert>\n'
            '                                        </xades:SigningCertificate>\n'
            '                                    </xades:SignedSignatureProperties>\n'
            '                                </xades:SignedProperties>'
        )

        # Clean up the XML string (normalize newlines and trim extra spaces)
        xml_string = xml_string.replace("\r\n", "\n").strip()

        # Generate the SHA256 hash of the XML string in binary format
        hash_bytes = hashlib.sha256(xml_string.encode('utf-8')).digest()

        # Convert the hash to hex and then base64 encode the result
        hash_hex = hash_bytes.hex()
        return base64.b64encode(hash_hex.encode('utf-8')).decode('utf-8')


    @staticmethod
    def get_digital_signature(xml_hashing, private_key_content):
        try:
            hash_bytes = base64.b64decode(xml_hashing)
            if hash_bytes is None:
                raise Exception("Failed to decode the base64-encoded XML hashing.")
            
            private_key_content = private_key_content.replace("\n", "").replace("\t", "")
            if "-----BEGIN EC PRIVATE KEY-----" not in private_key_content and "-----END EC PRIVATE KEY-----" not in private_key_content:
                private_key_content = f"-----BEGIN EC PRIVATE KEY-----\n{private_key_content}\n-----END EC PRIVATE KEY-----"

            #private_key = crypto.load_privatekey(crypto.FILETYPE_PEM, private_key_content)
            #if private_key is None:
            #    raise Exception("Failed to read private key.")
            private_key = serialization.load_pem_private_key(private_key_content.encode(), password=None, backend=default_backend())
            # Sign the hash using ECDSA and SHA-256
            signature = private_key.sign(
            hash_bytes,  # The hash that needs to be signed
            ec.ECDSA(hashes.SHA256())  # ECDSA with SHA-256 hashing algorithm
            )

            #signature = crypto.sign(private_key, hash_bytes, 'sha256')
            return base64.b64encode(signature).decode()
        except Exception as e:
            raise Exception(f"Failed to process signature: {e}")
        
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
    

