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
import subprocess
import uuid as uuid_lib
import stat


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
    def get_request_api(xml, x509_certificate_content, private_key_content,environment_type):
        """Main function to process the invoice request."""
        # Define resource file paths
        resource_paths = {
            "xsl_file": os.path.join("",'ZatcaPython/resources/xslfile.xsl'),
            "ubl_template": os.path.join("",'ZatcaPython/resources/zatca_ubl.xml'),
            "signature": os.path.join("",'ZatcaPython/resources/zatca_signature.xml'),
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
        return einvoice_signer.sign_simplified_invoice(canonical_xml,environment_type,private_key_content, x509_certificate_content)

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
    def sign_simplified_invoice(canonical_xml, environment_type, private_key, certificate_content):
        """
        Sign the simplified invoice using Fatoora CLI based on the environment.
        Arguments:
        - canonical_xml: The canonicalized XML content to be signed.
        - environment_type: The environment type ("Simulation", "NonProduction", or "Production").
        - private_key: The private key content to be used for signing.
        - generated_csr: The CSR content to be used for signing.
        
        Returns:
        - JSON string containing the invoiceHash, uuid, and invoice.
        """

       

      
        jar_file_root_path = os.path.abspath("ZatcaPython/utilities/fatoora-cli")
        jar_file = os.path.join(jar_file_root_path,"Apps/zatca-einvoicing-sdk-238-R3.4.4.jar")


        if environment_type not in ["Production"]:
             jar_file_root_path = os.path.abspath("ZatcaPython/utilities/fatoora-cli-simulation")
             jar_file = os.path.join(jar_file_root_path,"Apps/zatca-einvoicing-sdk-238-R3.4.4.jar")

        env = os.environ.copy()
        env["SDK_CONFIG"] = os.path.join(jar_file_root_path,"Configuration/config.json")
        env["FATOORA_HOME"] = jar_file_root_path

        env_flag = ""

        # Add sandbox flag for NonProduction or Simulation
        if environment_type in ["Simulation"]:
            env_flag = "-sim"

        if environment_type in ["NonProduction"]:
            env_flag = "-nonprod"
        
        file_name = f"{uuid_lib.uuid4().hex}"
        private_key_file_path = os.path.join(jar_file_root_path,"Data/Certificates", f"ec-secp256k1-priv-key.pem")
        certificate_file_path = os.path.join(jar_file_root_path,"Data/Certificates", f"cert.pem")

        xml_file_path = os.path.join("ZatcaPython", f"{file_name}.xml")
        signed_file_path = os.path.join("ZatcaPython", f"{file_name}_signed.xml")
        request_file_path = os.path.join("ZatcaPython", f"{file_name}_request.json")

        try:
            # Save the canonical XML to the file
            with open(xml_file_path, "w") as xml_file:
                xml_file.write(canonical_xml)

            # Save the private key to a file
            with open(private_key_file_path, "w") as key_file:
                key_file.write(private_key)
            # Set 644 permissions
            #os.chmod(private_key_file_path, stat.S_IRUSR | stat.S_IWUSR | stat.S_IRGRP | stat.S_IROTH)

            # Save the CSR to a file
            with open(certificate_file_path, "w") as certificate_file:
                certificate_file.write(certificate_content)

              # Save the CSR to a file
            
            '''
            with open(signed_file_path, "w") as signed_file:
                signed_file.write("")
            '''    
            

            if not os.path.exists(xml_file_path):
                error_data = {
                    "error": f"canonical_xml file not found: {xml_file_path}",
                }
                print(json.dumps(error_data))
                exit(1);

            if not os.path.exists(private_key_file_path):
                error_data = {
                    "error": f"private_key_file_path file not found: {private_key_file_path}",
                }
                print(json.dumps(error_data))
                exit(1);

            if not os.path.exists(certificate_file_path):
                error_data = {
                    "error": f"certificate file not found: {certificate_file_path}",
                }
                print(json.dumps(error_data))
                exit(1);


            cmd = [
                "java", 
                "-jar", jar_file,
                "-sign",
                "-invoice", os.path.abspath(xml_file_path),
                "-signedInvoice", os.path.abspath(signed_file_path),
                env_flag,
            ]
            
            try:
                result = subprocess.run(cmd,env=env, check=True, capture_output=True, text=True)
                '''
                error_data = {
                        "error":f"Command error: {result.stderr}, out: {result.stdout}, pk:{private_key_file_path}",
                        #"error":basePath,
                }
                print(json.dumps(error_data))  # Log the error as JSON
                exit(1);
                '''
            except subprocess.CalledProcessError as e:
                error_data = {
                    "error": f"error running sign command:{str(e)},out:{e.stdout}, err:{e.stderr}",
                }
                print(json.dumps(error_data))  # Log the error as JSON
                exit(1);
               
    
            # Verify signed invoice file
            if not os.path.exists(signed_file_path):
                error_data = {
                    "error": f"Signed invoice file not found: {signed_file_path}. Executing Fatoora sign command:"+ " ".join(sign_command),
                }
                print(json.dumps(error_data))
                exit(1);
            

            # Step 3: Run the Fatoora invoice request command
            invoice_request_command = [
                "java", 
                "-jar", jar_file,
                "-invoice", signed_file_path,
                "-invoiceRequest",
                "-apiRequest", request_file_path,
                env_flag
            ]
            #print("Executing Fatoora invoice request command:", " ".join(invoice_request_command))
            subprocess.run(invoice_request_command,env=env, check=True,capture_output=True, text=True)

            if not os.path.exists(request_file_path):
                error_data = {
                    "error": f"Request file not found: {request_file_path}. Executing Fatoora sign command:"+ " ".join(invoice_request_command),
                }
                print(json.dumps(error_data))
                exit(1);

            # Step 4: Extract content from the generated request JSON file
            with open(request_file_path, "r") as request_file:
                request_data = json.load(request_file)


            
            for file_path in [xml_file_path, private_key_file_path, certificate_file_path, signed_file_path, request_file_path]:
                if os.path.exists(file_path):
                    try:
                        os.remove(file_path)
                    except Exception as cleanup_error:
                        #print(f"Failed to remove file {file_path}: {cleanup_error}")
                        error_data = {
                        "error": str(cleanup_error),
                        }
                        print(json.dumps(error_data))
                        exit(1);       
                if os.path.exists(file_path):
                    error_data = {
                        "error": f"{file_path} Not Removed",
                    }
                    print(json.dumps(error_data))
                    exit(1);       
            # Return the extracted data
            return json.dumps({
                "invoiceHash": request_data["invoiceHash"],
                "uuid": request_data["uuid"],
                "invoice": request_data["invoice"]
            })

        except subprocess.CalledProcessError as e:
            #print("Error executing Fatoora CLI command:", e.stderr)
            for file_path in [xml_file_path, private_key_file_path, certificate_file_path, signed_file_path, request_file_path]:
                if os.path.exists(file_path):
                    os.remove(file_path) 
            error_data = {
            "error": str(e),
            }
            print(json.dumps(error_data))
            exit(1)
          
        except Exception as e:
            #print("An error occurred:", e)
            for file_path in [xml_file_path, private_key_file_path, certificate_file_path, signed_file_path, request_file_path]:
                if os.path.exists(file_path):
                    os.remove(file_path)
            error_data = {
            "error": str(e),
            "traceback": ''
            }
            print(json.dumps(error_data)) 
            exit(1)
        
             
    '''
    @staticmethod
    def sign_simplified_invoice(canonical_xml, base64_hash, x509_certificate_content, private_key_content, ubl_template_path, signature_path, uuid):
        """Sign the simplified invoice based on ZATCA's latest guidelines."""
        # Get current time in Saudi Arabia timezone (UTC+3)
        utc_now = datetime.now(timezone.utc)
        saudi_tz = pytz.timezone("Asia/Riyadh")
        saudi_time = utc_now.astimezone(saudi_tz)
        signature_timestamp = saudi_time.strftime("%Y-%m-%dT%H:%M:%S")

        # Wrap the certificate content
        pem_certificate = einvoice_signer.wrap_certificate(x509_certificate_content)
        certificate = x509.load_pem_x509_certificate(pem_certificate.encode(), default_backend())

        # Extract certificate details
        issuer_name = einvoice_signer.get_issuer_name(certificate)
        serial_number = certificate.serial_number

        # Generate public key hashing
        public_key_hashing = einvoice_signer.generate_public_key_hashing(x509_certificate_content)

        # Generate the signed properties hash
        signed_properties_hash = einvoice_signer.get_signed_properties_hash(
            signature_timestamp, public_key_hashing, issuer_name, serial_number
        )

        # Generate the digital signature
        signature_value = einvoice_signer.get_digital_signature(base64_hash, private_key_content)

        # Generate the ECDSA result
        ecdsa_result = einvoice_signer.get_public_key_and_signature(x509_certificate_content)

        # Populate the UBL template
        ubl_content = einvoice_signer.populate_ubl_template(
            ubl_template_path, base64_hash, signed_properties_hash, signature_value,
            x509_certificate_content, signature_timestamp, public_key_hashing, issuer_name, serial_number
        )

        # Insert the UBL content into the canonical XML
        updated_xml_string = einvoice_signer.insert_ubl_into_xml(canonical_xml, ubl_content)

        # Generate the QR code with the ECDSA result
        qr_code = qr_code_generator.generate_qr_code(
            canonical_xml, base64_hash, signature_value, ecdsa_result
        )

        # Insert the signature content into the XML
        updated_xml_string = einvoice_signer.insert_signature_into_xml(
            updated_xml_string, signature_path, qr_code
        )

        # Encode the final invoice
        base64_invoice = einvoice_signer.encode_invoice(
            '<?xml version="1.0" encoding="UTF-8"?>\n', updated_xml_string
        )

        # Return the result dictionary
        return json.dumps({
            "invoiceHash": base64_hash,
            "uuid": uuid,
            "invoice": base64_invoice,
        })
    '''    

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


