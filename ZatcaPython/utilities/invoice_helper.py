import base64
from lxml import etree 
import uuid

class invoice_helper:

    @staticmethod
    def is_simplified_invoice(xml):
        namespace = {'cbc': 'urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2'}
        invoice_type_code_node = xml.find('.//cbc:InvoiceTypeCode', namespaces=namespace)
        if invoice_type_code_node is not None:
            name_attribute = invoice_type_code_node.get('name')
            return name_attribute.startswith('02')
        return False

    @staticmethod
    def modify_xml(base_document, id, invoice_type_codename, invoice_type_code_value, icv, pih, instruction_note):
        # Clone the document to keep the original intact
        new_doc = etree.ElementTree(etree.fromstring(etree.tostring(base_document.getroot(), pretty_print=True)))

        # Generate a new GUID for the UUID
        #guid_string = str(uuid.uuid4()).upper()
   
        guid_string = '4bd41220-f619-47bc-830b-7fedd3b33032'

        # Define namespaces
        namespaces = {
            'cbc': 'urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2',
            'cac': 'urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2'
        }

        # Print the XML for debugging
        #print("Current XML structure:")
        #print(etree.tostring(new_doc, pretty_print=True).decode())

        # Modify the ID node
        id_node = new_doc.find('.//cbc:ID', namespaces=namespaces)
        if id_node is not None:
            id_node.text = id
        
        # Modify the UUID node
        uuid_node = new_doc.find('.//cbc:UUID', namespaces=namespaces)
        if uuid_node is not None:
            uuid_node.text = guid_string
        
        # Modify InvoiceTypeCode node and set 'name' attribute
        invoice_type_code_node = new_doc.find('.//cbc:InvoiceTypeCode', namespaces=namespaces)
        if invoice_type_code_node is not None:
            invoice_type_code_node.text = invoice_type_code_value
            invoice_type_code_node.set('name', invoice_type_codename)

        # Update AdditionalDocumentReference for ICV
        additional_reference_node = new_doc.find(".//cac:AdditionalDocumentReference[cbc:ID='ICV']/cbc:UUID", namespaces=namespaces)
        if additional_reference_node is not None:
            additional_reference_node.text = str(icv)
        else:
            print("UUID node not found for ICV.")

        # Update AdditionalDocumentReference for PIH
        pih_node = new_doc.find(".//cac:AdditionalDocumentReference[cbc:ID='PIH']/cac:Attachment/cbc:EmbeddedDocumentBinaryObject", namespaces=namespaces)
        if pih_node is not None:
            pih_node.text = pih
        else:
            print("EmbeddedDocumentBinaryObject node not found for PIH.")

        # Conditionally add InstructionNote or remove BillingReference elements
        if instruction_note:
            payment_means_node = new_doc.find('.//cac:PaymentMeans', namespaces=namespaces)
            if payment_means_node is not None:
                instruction_note_element = etree.Element('{urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2}InstructionNote')
                instruction_note_element.text = instruction_note
                payment_means_node.append(instruction_note_element)
        else:
            billing_reference_nodes = new_doc.findall('.//cac:BillingReference', namespaces=namespaces)
            for billing_reference_node in billing_reference_nodes:
                parent_node = new_doc.find('.//cac:BillingReference/..', namespaces=namespaces)
                if parent_node is not None:
                    parent_node.remove(billing_reference_node)    

        return new_doc
    
    @staticmethod
    def extract_invoice_hash_and_base64_qr_code(xml_input):
        if isinstance(xml_input, str):
            decoded_xml = base64.b64decode(xml_input)
            if decoded_xml is None:
                raise ValueError("Invalid Base64 string provided.")
            xml_input = decoded_xml
        elif not isinstance(xml_input, (bytes, etree._Element)):
            raise ValueError("Input must be a string or lxml.etree._Element.")

        # Load XML into an lxml Element
        if isinstance(xml_input, bytes):
            doc = etree.fromstring(xml_input)
        else:
            doc = xml_input  # Assume it's already an lxml Element

        # Initialize XPath with namespaces
        namespaces = {
            'ext': "urn:oasis:names:specification:ubl:schema:xsd:CommonExtensionComponents-2",
            'ds': "http://www.w3.org/2000/09/xmldsig#",
            'cbc': "urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2",
            'cac': "urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"
        }

        # Extract invoiceHash
        invoice_hash_node = doc.xpath("//ds:Reference[@Id='invoiceSignedData']/ds:DigestValue", namespaces=namespaces)
        invoice_hash = invoice_hash_node[0].text if invoice_hash_node else None

        # Extract base64QRCode
        base64_qr_code_node = doc.xpath("//cac:AdditionalDocumentReference[cbc:ID='QR']/cac:Attachment/cbc:EmbeddedDocumentBinaryObject", namespaces=namespaces)
        base64_qr_code = base64_qr_code_node[0].text if base64_qr_code_node else None

        return invoice_hash, base64_qr_code
