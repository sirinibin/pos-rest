import json
import base64
import time
from utilities.api_helper import api_helper
from utilities.invoice_helper import invoice_helper
from utilities.einvoice_signer import einvoice_signer
from lxml import etree 

def main():
    print("\n3: Clearance & Reporting Documents\n")

    cert_info = api_helper.load_json_from_file("certificates/certificateInfo.json")
    xml_template_path = "templates/invoice.xml"

    private_key = cert_info["privateKey"]
    x509_certificate_content = base64.b64decode(cert_info["pcsid_binarySecurityToken"]).decode('utf-8')
    #print(f"x509_certificate_content: {x509_certificate_content}\n")

    parser = etree.XMLParser(remove_blank_text=False)
    base_document = etree.parse(xml_template_path, parser)
    document_types = [
         ["STDSI", "388", "Standard Invoice", ""],
     #   ["STDCN", "383", "Standard CreditNote", "InstructionNotes for Standard CreditNote"],
     #   ["STDDN", "381", "Standard DebitNote", "InstructionNotes for Standard DebitNote"],
     #    ["SIMSI", "388", "Simplified Invoice", ""],
     #   ["SIMCN", "383", "Simplified CreditNote", "InstructionNotes for Simplified CreditNote"],
     #   ["SIMDN", "381", "Simplified DebitNote", "InstructionNotes for Simplified DebitNote"]
    ]

    icv = 0
    pih = "NWZlY2ViNjZmZmM4NmYzOGQ5NTI3ODZjNmQ2OTZjNzljMmRiYzIzOWRkNGU5MWI0NjcyOWQ3M2EyN2ZiNTdlOQ=="

    for doc_type in document_types:
        prefix, type_code, description, instruction_note = doc_type
        icv += 1
        is_simplified = prefix.startswith("SIM")

        print(f"Processing {description}...\n")

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
        new_doc = base_document
        
        json_payload = einvoice_signer.get_request_api(new_doc, x509_certificate_content, private_key)
        
        #print(f"\n\njson_payload\n")
        #print(json_payload)

        
        if einvoice_signer.is_simplified_invoice(new_doc):
            response = api_helper.invoice_reporting(cert_info, json_payload)
            request_type = "Reporting Api"
            api_url = cert_info["reportingUrl"]
        else:
            response = api_helper.invoice_clearance(cert_info, json_payload)
            request_type = "Clearance Api"
            api_url = cert_info["clearanceUrl"]

        clean_response = api_helper.clean_up_json(response, request_type, api_url)

        json_decoded_response = json.loads(response)

        if json_decoded_response:
            print(f"Reporting api Server Response: \n{clean_response}")
        else:
            print(f"Invalid JSON Response: \n{response}")
            exit(1)

        if response is None:
            print(f"Failed to process {description}: serverResult is null.\n")
            exit(1)

        status = json_decoded_response["reportingStatus"] if is_simplified else json_decoded_response["clearanceStatus"]

        if "REPORTED" in status or "CLEARED" in status:
            json_payload = json.loads(json_payload)
            pih = json_payload["invoiceHash"]
            print(f"\n\npih:\n")
            print(pih)
            print(f"\n{description} processed successfully\n\n")
        else:
            print(f"Failed to process {description}: status is {status}\n")
            exit(1)

        time.sleep(1) 

if __name__ == "__main__":
    main()