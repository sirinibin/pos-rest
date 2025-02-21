import json
import base64
from utilities.api_helper import api_helper
from utilities.csr_generator import CsrGenerator
from utilities.invoice_helper import invoice_helper
from utilities.einvoice_signer import einvoice_signer
from lxml import etree 
import sys
import traceback

def main():
    try:
        dataFromGo = sys.stdin.read().strip()
        if not dataFromGo:
            #dataFromGo["error"]="No input received"
            #print(json.dumps(data))
            error_data = {
            "invoice_hash": "",
            "compliance_passed": False,
            "error": "No input received",
            "traceback": traceback.format_exc()
            }
            print(json.dumps(error_data)) 
            return
        
        try:
            payloadFromGo = json.loads(dataFromGo)  # Parse JSON
 
        except json.JSONDecodeError:
            #print("Invalid JSON received", file=sys.stderr)
            error_data = {
            "invoice_hash": "",
            "compliance_passed": False,
            "error": "Invalid JSON received",
            "traceback": traceback.format_exc()
            }
            print(json.dumps(error_data)) 
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

        # Prepare certificate information
        cert_info = {
            "error":"",
            "environmentType": environment_type,
            "csr": "",
            "privateKey": payloadFromGo["private_key"],
            #"privateKey": "",
            "OTP": "",
            "ccsid_requestID": "",
            "ccsid_binarySecurityToken": payloadFromGo["binary_security_token"],
            #"ccsid_binarySecurityToken": "",
            "ccsid_secret": payloadFromGo["secret"],
            #"ccsid_secret": "",
            "pcsid_requestID": "",
            "pcsid_binarySecurityToken": "",
            "pcsid_secret": "",
            "lastICV": "0",
            "lastInvoiceHash": "NWZlY2ViNjZmZmM4NmYzOGQ5NTI3ODZjNmQ2OTZjNzljMmRiYzIzOWRkNGU5MWI0NjcyOWQ3M2EyN2ZiNTdlOQ==",
            "complianceCsidUrl": f"https://gw-fatoora.zatca.gov.sa/e-invoicing/{api_path}/compliance",
            "complianceChecksUrl": f"https://gw-fatoora.zatca.gov.sa/e-invoicing/{api_path}/compliance/invoices",
            "productionCsidUrl": f"https://gw-fatoora.zatca.gov.sa/e-invoicing/{api_path}/production/csids",
            "reportingUrl": f"https://gw-fatoora.zatca.gov.sa/e-invoicing/{api_path}/invoices/reporting/single",
            "clearanceUrl": f"https://gw-fatoora.zatca.gov.sa/e-invoicing/{api_path}/invoices/clearance/single",
        }
  

        #cert_info = api_helper.load_json_from_file("certificates/certificateInfo.json")
        #xml_template_path = "templates/invoice.xml"
        xml_template_path = payloadFromGo["xml_file_path"]

        #cert_info["ccsid_requestID"] = payloadFromGo["compliance_request_id"]
        #cert_info["ccsid_binarySecurityToken"] =  payloadFromGo["binary_security_token"]
        #cert_info["ccsid_secret"] = payloadFromGo["secret"]

 
        # 3: Sending Sample Documents
        #print("\n3: Sending Sample Documents\n")


        private_key = cert_info["privateKey"]
        x509_certificate_content = base64.b64decode(cert_info["ccsid_binarySecurityToken"]).decode('utf-8')

        parser = etree.XMLParser(remove_blank_text=False)
        base_document = etree.parse(xml_template_path, parser)
        # document_types = [
        #    ["STDSI", "388", "Standard Invoice", ""],
        # ["STDCN", "383", "Standard CreditNote", "InstructionNotes for Standard CreditNote"],
        # ["STDDN", "381", "Standard DebitNote", "InstructionNotes for Standard DebitNote"],
        # ["SIMSI", "388", "Simplified Invoice", ""],
        # ["SIMCN", "383", "Simplified CreditNote", "InstructionNotes for Simplified CreditNote"],
        # ["SIMDN", "381", "Simplified DebitNote", "InstructionNotes for Simplified DebitNote"]
        #]

        #icv = 0
        #pih = "NWZlY2ViNjZmZmM4NmYzOGQ5NTI3ODZjNmQ2OTZjNzljMmRiYzIzOWRkNGU5MWI0NjcyOWQ3M2EyN2ZiNTdlOQ=="

        '''
        for doc_type in document_types:
        prefix, type_code, description, instruction_note = doc_type
        icv += 1
        is_simplified = prefix.startswith("SIM")

        print(f"Processing {description}...\n")
        '''

        '''
            new_doc = invoice_helper.modify_xml(
                base_document,
                f"{prefix}-0001",
                "0200000" if is_simplified else "0100000",
                type_code,
                icv,
                pih,
                instruction_note
            )
        '''

        #is_simplified = False
        is_simplified = payloadFromGo["is_simplified"]    
            
        new_doc = base_document

        basPath = "ZatcaPython/"
        #basPath = ""
    
        json_payload = einvoice_signer.get_request_api(new_doc, x509_certificate_content, private_key,basPath)
    
        
        response = api_helper.compliance_checks(cert_info, json_payload)
        #request_type = "Compliance Checks"
        #api_url = cert_info["complianceChecksUrl"]

        #clean_response = api_helper.clean_up_json(response, request_type, api_url)

        json_decoded_response = json.loads(response)

        ''' 
        if json_decoded_response:
            print(f"complianceChecks Server Response: \n{clean_response}")
        else:
            print(f"Invalid JSON Response: \n{response}")
            exit(1)

        if response is None:
            print(f"Failed to process invoice: serverResult is null.\n")
            exit(1)
        '''    

    

        status = json_decoded_response["reportingStatus"] if is_simplified else json_decoded_response["clearanceStatus"]

        if "REPORTED" in status or "CLEARED" in status:
            json_payload = json.loads(json_payload)
            pih = json_payload["invoiceHash"]
            data = {
            "invoice_hash": json_payload["invoiceHash"],
            "compliance_passed": True,
            "error":   "",
            }
            print(json.dumps(data))  # Print JSON output
            #print(f"\ninvoice processed successfully\n\n")
        else:
            data = {
            "invoice_hash": "",
            "compliance_passed": False,
            "error":   "Compliance check failed: status is {status}\n",
            }
            print(json.dumps(data))  # Print JSON output
            #print(f"Failed to process invoice: status is {status}\n")
            #exit(1)  

            #time.sleep(1)  
    except Exception as e:
        error_data = {
            "invoice_hash": "",
            "compliance_passed": False,
            "error": "Compliance check failed: "+str(e),
            "traceback": traceback.format_exc()
        }
        print(json.dumps(error_data))  # Print error in JSON format
        #exit(1)  # Ensure Go detects failure
if __name__ == "__main__":
    main()
