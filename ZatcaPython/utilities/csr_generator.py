# csr_generator.py
from cryptography import x509
from cryptography.x509.oid import NameOID, ObjectIdentifier
from cryptography.hazmat.backends import default_backend
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec
import base64
import os
import re
from asn1crypto.core import UTF8String
import tempfile
import subprocess

class CsrGenerator:
    def __init__(self, config, environment_type,dirPrefix):
        self.config = config
        self.environment_type = environment_type
        self.asn_template = self.get_asn_template()
        self.environment_type = environment_type
        self.fatoora_cli_simulation = dirPrefix+"utilities/fatoora-cli-simulation/Apps/zatca-einvoicing-sdk-238-R3.4.4.jar"
        self.fatoora_cli = dirPrefix+"utilities/fatoora-cli/Apps/zatca-einvoicing-sdk-238-R3.4.4.jar"

    def get_asn_template(self):
        if self.environment_type == 'NonProduction':
            return 'TSTZATCA-Code-Signing'
        elif self.environment_type == 'Simulation':
            return 'PREZATCA-Code-Signing'
        elif self.environment_type == 'Production':
            return 'ZATCA-Code-Signing'
        else:
            raise ValueError("Invalid environment type specified.")

    def generate_private_key(self):
        return ec.generate_private_key(ec.SECP256K1(), default_backend())

    def generate_csr(self):
        """
        Generate both private key and CSR using Fatoora CLI.
        Returns:
            - private_key_content: Base64 string of private key (no headers)
            - csr_base64: Base64-encoded CSR
        """
        with tempfile.TemporaryDirectory() as tmpdir:
            csr_config_file = os.path.join(tmpdir, "csr.properties")
            private_key_file = os.path.join(tmpdir, "private.key")
            csr_file = os.path.join(tmpdir, "csr.pem")

            # Write csr.properties
            with open(csr_config_file, "w") as f:
                f.write(f"csr.common.name={self.config.get('csr.common.name','')}\n")
                f.write(f"csr.serial.number={self.config.get('csr.serial.number','')}\n")
                f.write(f"csr.organization.identifier={self.config.get('csr.organization.identifier','')}\n")
                f.write(f"csr.organization.unit.name={self.config.get('csr.organization.unit.name','')}\n")
                f.write(f"csr.organization.name={self.config.get('csr.organization.name','')}\n")
                f.write(f"csr.country.name={self.config.get('csr.country.name','SA')}\n")
                f.write(f"csr.invoice.type={self.config.get('csr.invoice.type','')}\n")
                f.write(f"csr.location.address={self.config.get('csr.location.address','')}\n")
                f.write(f"csr.industry.business.category={self.config.get('csr.industry.business.category','')}\n")

            # Build command
            if self.environment_type in ["NonProduction", "Simulation"]:
                cmd = [
                    "java", "-jar", self.fatoora_cli_simulation,
                    "-csr",
                    "-csrConfig", csr_config_file,
                    "-privateKey", private_key_file,
                    "-generatedCsr", csr_file,
                    "-pem"
                ]
            else:
                cmd = [
                    "java", "-jar", self.fatoora_cli,
                    "-csr",
                    "-csrConfig", csr_config_file,
                    "-privateKey", private_key_file,
                    "-generatedCsr", csr_file,
                    "-pem"
                ]

            # Add sandbox flag for NonProduction or Simulation
            if self.environment_type in ["Simulation"]:
                cmd.insert(3, "-sim")

            if self.environment_type in ["NonProduction"]:
                cmd.insert(3, "-nonprod")

            # Run Fatoora CLI
            try:
                subprocess.run(cmd, check=True, capture_output=True, text=True)
            except subprocess.CalledProcessError as e:
                print("Error generating CSR with Fatoora CLI:", e.stderr)
                raise

            # Read private key
            with open(private_key_file, "r") as f:
                private_key_pem = f.read()
            private_key_content = "".join(private_key_pem.strip().splitlines()[1:-1])

            # Read CSR
            with open(csr_file, "rb") as f:
                csr_pem = f.read()
            csr_base64 = base64.b64encode(csr_pem).decode("utf-8")

            return private_key_content, csr_base64

    '''
    def generate_csr(self):
        private_key = self.generate_private_key()
        
        # Build the CSR
        csr_builder = x509.CertificateSigningRequestBuilder()
        csr_builder = csr_builder.subject_name(x509.Name([
            x509.NameAttribute(NameOID.COMMON_NAME, self.config.get('csr.common.name', '')),
            x509.NameAttribute(NameOID.ORGANIZATION_NAME, self.config.get('csr.organization.name', '')),
            x509.NameAttribute(NameOID.ORGANIZATIONAL_UNIT_NAME, self.config.get('csr.organization.unit.name', '')),
            x509.NameAttribute(NameOID.COUNTRY_NAME, self.config.get('csr.country.name', 'SA')),
        ]))

        # Add ASN.1 extension
        csr_builder = csr_builder.add_extension(
            x509.UnrecognizedExtension(
                ObjectIdentifier("1.3.6.1.4.1.311.20.2"), 
                UTF8String(self.asn_template).dump()
            ),
            critical=False
        )
        #print("\nVAT:\n"+ self.config.get('csr.organization.identifier', ''))
        
        # Add SAN extension
        csr_builder = csr_builder.add_extension(
            x509.SubjectAlternativeName([
                x509.DirectoryName(x509.Name([
                    x509.NameAttribute(ObjectIdentifier("2.5.4.4"), self.config.get('csr.serial.number', '')),
                    x509.NameAttribute(ObjectIdentifier("0.9.2342.19200300.100.1.1"), self.config.get('csr.organization.identifier', '')),
                    x509.NameAttribute(ObjectIdentifier("2.5.4.12"), self.config.get('csr.invoice.type', '')),
                    x509.NameAttribute(ObjectIdentifier("2.5.4.26"), self.config.get('csr.location.address', '')),
                    x509.NameAttribute(ObjectIdentifier("2.5.4.15"), self.config.get('csr.industry.business.category', ''))
                ]))
            ]),
            critical=False
        )

        # Sign the CSR with the private key
        csr = csr_builder.sign(private_key, hashes.SHA256(), default_backend())

        # Serialize private key and CSR
        private_key_pem = private_key.private_bytes(
            encoding=serialization.Encoding.PEM,
            format=serialization.PrivateFormat.TraditionalOpenSSL,
            encryption_algorithm=serialization.NoEncryption()
        )
        csr_pem = csr.public_bytes(serialization.Encoding.PEM)

        # Strip header/footer from private key
        private_key_content = re.sub(
            r'-----BEGIN .* PRIVATE KEY-----|-----END .* PRIVATE KEY-----|\n', '', 
            private_key_pem.decode('utf-8')
        )

        

        # Encode CSR in Base64
        csr_base64 = base64.b64encode(csr_pem).decode('utf-8')

        return private_key_content, csr_base64
    '''    

    def save_to_files(self, private_key_pem, csr_pem):
        os.makedirs("certificates", exist_ok=True)
        private_key_file = 'certificates/PrivateKey.pem'
        csr_file = 'certificates/taxpayer.csr'
        
        with open(private_key_file, "wb") as key_file:
            key_file.write(private_key_pem)
        
        with open(csr_file, "wb") as csr_file:
            csr_file.write(csr_pem)
        
        print(f"\nPrivate key and CSR have been saved to {private_key_file} and {csr_file}, respectively.")
