import json
import base64
import time
from utilities.api_helper import api_helper
from utilities.invoice_helper import invoice_helper
from utilities.einvoice_signer import einvoice_signer
from lxml import etree 
import sys
import traceback

def main():
    try:
        
        dataFromGo = sys.stdin.read().strip()
        if not dataFromGo:
            dataFromGo["error"]="No input received"
            print(json.dumps(data))
            return
        
        try:
            payloadFromGo = json.loads(dataFromGo)  # Parse JSON
 
        except json.JSONDecodeError:
            print("Invalid JSON received", file=sys.stderr)
            return    
      
        
        environment_type = payloadFromGo["env"]
        #environment_type = "NonProduction"

        api_path = 'developer-portal'  # Default value

        # Determine API path based on environment type
        if environment_type == 'NonProduction':
            api_path = 'developer-portal'
        elif environment_type == 'Simulation':
            api_path = 'simulation'
        elif environment_type == 'Production':
            api_path = 'core'

        #print("\n3: Clearance & Reporting Documents\n")

        #cert_info = api_helper.load_json_from_file("certificates/certificateInfo.json")
        #xml_template_path = "templates/invoice_S-INV-YUGU-000002.xml"
        xml_template_path = payloadFromGo["xml_file_path"]
        
        cert_info = {
            "error":"",
            "environmentType": environment_type,
            "csr": "",
            "privateKey": payloadFromGo["private_key"],
            #"privateKey": "",
            "OTP": "",
            "ccsid_requestID": "",
            "ccsid_binarySecurityToken": "",
            "ccsid_secret": "",
            #"ccsid_secret": "",
            "pcsid_requestID": "",
            "pcsid_binarySecurityToken": payloadFromGo["production_binary_security_token"],
            "pcsid_secret": payloadFromGo["production_secret"],
            "lastICV": "0",
            "lastInvoiceHash": "NWZlY2ViNjZmZmM4NmYzOGQ5NTI3ODZjNmQ2OTZjNzljMmRiYzIzOWRkNGU5MWI0NjcyOWQ3M2EyN2ZiNTdlOQ==",
            "complianceCsidUrl": f"https://gw-fatoora.zatca.gov.sa/e-invoicing/{api_path}/compliance",
            "complianceChecksUrl": f"https://gw-fatoora.zatca.gov.sa/e-invoicing/{api_path}/compliance/invoices",
            "productionCsidUrl": f"https://gw-fatoora.zatca.gov.sa/e-invoicing/{api_path}/production/csids",
            "reportingUrl": f"https://gw-fatoora.zatca.gov.sa/e-invoicing/{api_path}/invoices/reporting/single",
            "clearanceUrl": f"https://gw-fatoora.zatca.gov.sa/e-invoicing/{api_path}/invoices/clearance/single",
        }

        private_key = cert_info["privateKey"]
        x509_certificate_content = base64.b64decode(cert_info["pcsid_binarySecurityToken"]).decode('utf-8')
        #print(f"x509_certificate_content: {x509_certificate_content}\n")

        parser = etree.XMLParser(remove_blank_text=False)
        base_document = etree.parse(xml_template_path, parser)

        #document_types = [
        #    ["STDSI", "388", "Standard Invoice", ""],
        #   ["STDCN", "383", "Standard CreditNote", "InstructionNotes for Standard CreditNote"],
        #   ["STDDN", "381", "Standard DebitNote", "InstructionNotes for Standard DebitNote"],
        #    ["SIMSI", "388", "Simplified Invoice", ""],
        #   ["SIMCN", "383", "Simplified CreditNote", "InstructionNotes for Simplified CreditNote"],
        #   ["SIMDN", "381", "Simplified DebitNote", "InstructionNotes for Simplified DebitNote"]
        #]

        #icv = 0
        #pih = "NWZlY2ViNjZmZmM4NmYzOGQ5NTI3ODZjNmQ2OTZjNzljMmRiYzIzOWRkNGU5MWI0NjcyOWQ3M2EyN2ZiNTdlOQ=="
        is_simplified = payloadFromGo["is_simplified"]  
        #is_simplified = False  

        
        new_doc = base_document

        basPath = "ZatcaPython/"
        #basPath = ""
        
        json_payload = einvoice_signer.get_request_api(new_doc, x509_certificate_content, private_key,basPath)
        
        
        if einvoice_signer.is_simplified_invoice(new_doc):
            response = api_helper.invoice_reporting(cert_info, json_payload)
            request_type = "Reporting Api"
            api_url = cert_info["reportingUrl"]
        else:
            response = api_helper.invoice_clearance(cert_info, json_payload)
            request_type = "Clearance Api"
            api_url = cert_info["clearanceUrl"]

        #clean_response = api_helper.clean_up_json(response, request_type, api_url)

        json_decoded_response = json.loads(response)

        '''
        if json_decoded_response:
            print(f"Reporting api Server Response: \n{clean_response}")
        else:
            print(f"Invalid JSON Response: \n{response}")
            exit(1)

        if response is None:
            print(f"Failed to process {description}: serverResult is null.\n")
            exit(1)
        '''    

        status = json_decoded_response["reportingStatus"] if is_simplified else json_decoded_response["clearanceStatus"]
       
        clearedInvoice = ""
        if is_simplified == False: 
            clearedInvoice = json_decoded_response["clearedInvoice"]
        else:     
            simplifiedPayload = json.loads(json_payload)
            clearedInvoice = simplifiedPayload["invoice"]

        if "REPORTED" in status or "CLEARED" in status:
            json_payload = json.loads(json_payload)
            pih = json_payload["invoiceHash"]
            data = {
            "invoice_hash": json_payload["invoiceHash"],
            "reporting_passed": True,
            "cleared_invoice": clearedInvoice,
            "is_simplified": is_simplified,
            "error":   cert_info["error"],
            }
            
            print(json.dumps(data))  # Print JSON output
            #print(f"\n\npih:\n")
            #print(pih)
            #print(f"\n{description} processed successfully\n\n")
        else:
            #print(f"Failed to process {description}: status is {status}\n")
            data = {
            "invoice_hash": "",
            "reporting_passed": False,
            "error":   cert_info["error"],
            }
            print(json.dumps(data))  # Print JSON output
            #exit(1)

            #time.sleep(1) 
    except Exception as e:
        error_data = {
            "invoice_hash": "",
            "reporting_passed": False,
            "error": str(e),
            "traceback": traceback.format_exc()
        }
        print(json.dumps(error_data))  # Print error in JSON format
        #exit(1)  # Ensure Go detects failure        

if __name__ == "__main__":
    main()