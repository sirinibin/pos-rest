package models

func (quotation *Quotation) getHTML() string {
	htmlStr := `<html><body><div
	className="container"
	id="printableArea"
	style="
		backgroundColor: "white",
		border: "solid 2px",
		borderColor: "silver",
		borderRadius: "2mm",
		padding: "10px"
	"

>
	<div className="row" style={{ fontSize: "3mm" }}>
		<div className="col">
			<ul className="list-unstyled text-left">
				<li><h4 style={{ fontSize: "4mm" }}>{props.model.store ? props.model.store.name : "<STORE_NAME>"}</h4></li>
				<li>{props.model.store ? props.model.store.title : "<STORE_TITLE>"}</li>
				{/*<!-- <li><hr /></li> --> */}
				<li>C.R. / {props.model.store ? props.model.store.registration_number : "<STORE_CR_NO>"}</li>
				<li>VAT / {props.model.store ? props.model.store.vat_no : "<STORE_VAT_NO>"}</li>
			</ul>
		</div>
		<div className="col">
			<div className="invoice-logo text-center">
				{props.model.store && props.model.store.logo ? <img width="100" height="100" src={process.env.REACT_APP_API_URL + props.model.store.logo + "?" + (Date.now())} alt="Invoice logo" /> : null}
			</div>
		</div>
		<div className="col">
			<ul className="list-unstyled text-end">
				<li>
					<h4 style={{ fontSize: "4mm" }}>
						<strong>
							{props.model.store ? props.model.store.name_in_arabic : "<STORE_NAME_ARABIC>"}
						</strong>
					</h4>
				</li>
				<li>
					{props.model.store ? props.model.store.title_in_arabic : "<STORE_TITLE_ARABIC>"}
				</li>
				{/* <!-- <li><hr /></li> --> */}
				<li>{props.model.store ? props.model.store.registration_number_in_arabic : "<STORE_CR_NO_ARABIC>"} / ‫ت‬.‫س‬</li>
				<li>{props.model.store ? props.model.store.vat_no_in_arabic : "<STORE_VAT_NO_ARABIC>"} / ‫الضريبي‬ ‫الرقم‬</li>
			</ul>
		</div>
	</div>
	<div className="row">
		<div className="col">
			<u
			><h1 className="text-center" style={{ fontSize: "4mm" }}>
					QUOTATION / اقتباس
				</h1>
			</u>
		</div>
	</div>
	<div className="row table-active" style={{ fontSize: "3mm" }}>
		<div className="col">
			<ul className="list-unstyled mb0 text-start">
				<li><strong>Quotation: </strong>#{props.model.code ? props.model.code : "<ID_NUMBER>"}</li>
				<li><strong>Quotation Date: </strong>{props.model.date_str ? props.model.date_str : "<DATE>"}</li>
				<li>
					<strong>Customer: </strong>{props.model.customer ? props.model.customer.name : "<CUSTOMER_NAME>"}
				</li>
				<li><strong>VAT Number: </strong>{props.model.customer ? props.model.customer.vat_no : "<CUSTOMER_VAT_NO>"}
				</li>
			</ul>
		</div>
		<div className="col">
			<ul className="list-unstyled mb0 text-end">
				<li><strong> اقتباس: </strong>#{props.model.code ? convertToPersianNumber(props.model.code) : "<ID_NUMBER_ARABIC>"}</li>
				<li><strong>تاريخ الاقتباس: </strong>{props.model.date_str ? getArabicDate(props.model.date_str) : "<DATE_ARABIC>"}</li>
				<li>
					<strong>عميل: </strong>{props.model.customer ? props.model.customer.name_in_arabic : "<CUSTOMER_NAME_ARABIC>"}
				</li>
				<li><strong>ظريبه الشراء: </strong>{props.model.customer ? props.model.customer.vat_no_in_arabic : "<CUSTOMER_VAT_NO_ARABIC>"}</li>
			</ul>
		</div>
	</div>
	<div className="row" style={{ fontSize: "3mm" }}>
		<div className="col text-start">Page 1 of 1</div>
		<div className="col text-end">صفحة ۱ من ۱</div>
	</div>
	<div className="row">
		<div className="col">
			<div
				className="table-responsive"
				style={{
					overflow: "hidden", outline: "none"
				}}
				tabIndex="0"
			>

				<table
					className="table table-bordered"
					style={{ fontSize: "3mm", borderRadius: "6px" }}
				>
					<thead>
						<tr>
							<th className="per1 text-center" style={{ padding: "0px" }}>
								<ul
									className="list-unstyled"
									style={{
										fontSize: "3mm", height: "35px", marginBottom: "0px"
									}}
								>
									<li>رقم سري</li>
									<li>SI No.</li>
								</ul>
							</th>
							<th className="per3 text-center" style={{ padding: "0px" }}>
								<ul
									className="list-unstyled"
									style={{
										fonSize: "3mm", height: "35px", marginBottom: "0px"
									}}
								>
									<li>رمز الصنف</li>
									<li>Item Code</li>
								</ul>
							</th>
							<th className="per68 text-center" style={{ padding: "0px" }}>
								<ul
									className="list-unstyled"
									style={{
										fontSize: "3mm", height: "35px", marginBottom: "0px"
									}}
								>
									<li>وصف</li>
									<li>Description</li>
								</ul>
							</th>
							<th className="per1 text-center" style={{ padding: "0px" }}>
								<ul
									className="list-unstyled"
									style={{ fontSize: "3mm", height: "35px", marginBottom: "0px" }}
								>
									<li>كمية</li>
									<li>Qty</li>
								</ul>
							</th>
							<th className="per10 text-center" style={{ padding: "0px" }}>
								<ul
									className="list-unstyled"
									style={{ fontSize: "3mm", height: "35px", marginBottom: "0px" }}
								>
									<li>سعر الوحدة</li>
									<li>Unit Price</li>
								</ul>
							</th>
							<th className="per20 text-center" style={{ padding: "0px" }}>
								<ul
									className="list-unstyled"
									style={{ fontSize: "3mm", height: "35px", marginBottom: "0px" }}
								>
									<li>المبلغ الإجمالي</li>
									<li>Total Amount</li>
								</ul>
							</th>
						</tr>
					</thead>
					<tbody>
						{props.model.products && props.model.products.map((product, index) => (
							<tr className="text-center">
								<td>{index + 1}</td>
								<td>{product.code ? product.code : product.item_code ? product.item_code : null}</td>
								<td>
									<ul
										className="list-unstyled"
										style={{ fontSize: "3mm", height: "35px", marginBottom: "0px" }}
									>
										<li>{product.name_in_arabic}</li>
										<li>{product.name}</li>
									</ul>
								</td>
								<td>{product.quantity}</td>
								<td>
									<NumberFormat
										value={product.unit_price}
										displayType={"text"}
										thousandSeparator={true}
										suffix={" SAR"}
										renderText={(value, props) => value}
									/>
								</td>
								<td>
									<NumberFormat
										value={(product.unit_price * product.quantity).toFixed(2)}
										displayType={"text"}
										thousandSeparator={true}
										suffix={" SAR"}
										renderText={(value, props) => value}
									/>
								</td>
							</tr>
						))}
					</tbody>

					<tfoot>
						<tr>
							<th colSpan="3" className="text-end"></th>
							<th className="text-center">{props.model.total_quantity}</th>
							<th className="text-end" style={{ padding: "0px" }}>
								<ul
									className="list-unstyled"
									style={{
										fontSize: "3mm", marginBottom: "0px"
									}}
								>
									<li>المجموع:</li>
									<li>Total:</li>
								</ul>
							</th>
							<th className="text-center" colSpan="2">

								<NumberFormat
									value={props.model.total}
									displayType={"text"}
									thousandSeparator={true}
									suffix={" SAR"}
									renderText={(value, props) => value}
								/>

							</th>
						</tr>
						<tr>
							<th className="text-end" colSpan="4" style={{ padding: "0px" }}>
								<ul
									className="list-unstyled"
									style={{
										fontSize: "3mm", marginBottom: "0px"
									}}
								>
									<li>ضريبة:</li>
									<li>VAT:</li>
								</ul>
							</th>
							<th className="text-center" colSpan="1">{props.model.vat_percent}%</th>
							<th className="text-center" colSpan="2">
								<NumberFormat
									value={props.model.vat_price}
									displayType={"text"}
									thousandSeparator={true}
									suffix={" SAR"}
									renderText={(value, props) => value}
								/>
							</th>
						</tr>
						<tr>
							<th className="text-end" colSpan="5" style={{ padding: "0px" }}>
								<ul
									className="list-unstyled"
									style={{
										fontSize: "3mm", marginBottom: "0px"
									}}
								>
									<li>خصم:</li>
									<li>Discount:</li>
								</ul>
							</th>
							<th className="text-center" colSpan="2">
								<NumberFormat
									value={props.model.discount}
									displayType={"text"}
									thousandSeparator={true}
									suffix={" SAR"}
									renderText={(value, props) => value}
								/>
							</th>
						</tr>
						<tr>
							<th className="text-end" colSpan="5" style={{ padding: "0px" }}>
								<ul
									className="list-unstyled"
									style={{
										fontSize: "3mm", marginBottom: "0px"
									}}
								>
									<li>الإجمالي الصافي:</li>
									<li>Net Total:</li>
								</ul>
							</th>
							<th className="text-center" colSpan="2">
								<NumberFormat
									value={props.model.net_total}
									displayType={"text"}
									thousandSeparator={true}
									suffix={" SAR"}
									renderText={(value, props) => value}
								/>
							</th>
						</tr>
						<tr>
							<th colSpan="1" className="text-end" style={{ padding: "0px" }}>
								<ul
									className="list-unstyled"
									style={{
										fontSize: "3mm", marginBottom: "0px"
									}}
								>
									<li>بكلمات:</li>
									<li>In Words:</li>
								</ul>
							</th>
							<th
								colSpan="5"
								style={{
									paddingLeft: "5px",
									paddingTop: "0px",
									paddingBottom: "0px"
								}}

							>
								<ul
									className="list-unstyled"
									style={{ fontSize: "3mm", marginBottom: "0px" }}
								>
									<li>{n2words(props.model.net_total, { lang: 'ar' })}</li>
									<li>{n2words(props.model.net_total, { lang: 'en' })}</li>
								</ul>
							</th>
						</tr>
					</tfoot>
				</table>

				<table className="table table-bordered" style={{ fontSize: "3mm" }}>
					<thead>
						<tr>
							<th className="text-end" style={{ width: "13%", padding: "0px" }}>
								<ul
									className="list-unstyled"
									style={{ fontSize: "3mm", marginBottom: "0px" }}
								>
									<li>سلمت بواسطة:</li>
									<li>Delivered By:</li>
								</ul>
							</th>
							<th style={{ width: "37%" }}> {props.model.delivered_by_user ? props.model.delivered_by_user.name : null}</th>
							<th className="text-end" style={{ width: "13%", padding: "0px" }}>
								<ul
									className="list-unstyled"
									style={{ fontSize: "3mm", marginBottom: "0px" }}
								>
									<li>استلمت من قبل:</li>
									<li>Received By:</li>
								</ul>
							</th>
							<th style={{ width: "37%" }}>

							</th>
						</tr>
						<tr>
							<th className="text-end" style={{ padding: "0px" }}>
								<ul className="list-unstyled" style={{ fontSize: "3mm" }}>
									<li>إمضاء:</li>
									<li>Signature:</li>
								</ul>
							</th>
							<th style={{ width: "37%", height: "80px" }}>
								{props.model.delivered_by_signature ?
									<img src={process.env.REACT_APP_API_URL + props.model.delivered_by_signature.signature + "?" + (Date.now())} key={props.model.delivered_by_signature.signature} style={{ width: 100, height: 80 }} ></img>
									: null}
							</th>
							<th className="text-end" style={{ padding: "0px" }}>
								<ul className="list-unstyled" style={{ fontSize: "3mm" }}>
									<li>إمضاء:</li>
									<li>Signature:</li>
								</ul>
							</th>
							<th></th>
						</tr>
						<tr>
							<th className="text-end" style={{ padding: "0px" }}>
								<ul
									className="list-unstyled"
									style={{
										fontSize: "3mm", marginBottom: "0px"
									}}
								>
									<li>تاريخ:</li>
									<li>Date:</li>
								</ul>
							</th>
							<th>
								<ul
									className="list-unstyled"
									style={{
										fontSize: "3mm", height: "35px", marginBottom: "0px"
									}}
								>
									<li>{props.model.signature_date_str ? getArabicDate(props.model.signature_date_str) : ""}</li>
									<li> {props.model.signature_date_str ? props.model.signature_date_str : ""}</li>
								</ul>
							</th>
							<th className="text-end" style={{ padding: "0px" }}>
								<ul
									className="list-unstyled"
									style={{
										fontSize: "3mm", marginBottom: "0px"
									}}
								>
									<li>تاريخ:</li>
									<li>Date:</li>
								</ul>
							</th>
							<th></th>
						</tr>
					</thead>
				</table>
			</div>
		</div>
	</div>
	<div className="row" style={{ fontSize: "3mm" }}>
		<div className="col-md-2 text-start">
			{props.model.store && props.model.customer ? <QRCode value={"Quotation #: " + props.model.code + "<br/> Store: " + props.model.store.name + "<br/> Net Total: " + props.model.net_total + "<br/> Customer: " + props.model.customer.name} size={128} /> : null}
		</div>
		<div className="col-md-8 text-center">
			<ul className="list-unstyled mb0 text-center">
				<li>
					<b
					> {props.model.store ? props.model.store.address_in_arabic : "<STORE_ADDRESS_ARABIC>"}
					</b>
				</li>
				<li>
					<strong
					>{props.model.store ? props.model.store.address : "<STORE_ADDRESS>"}
					</strong>
				</li>

				<li>
					هاتف:<b
					> {props.model.store ? props.model.store.phone_in_arabic : "<STORE_PHONE_ARABIC>"}
					</b>,
					Phone:<strong
					>{props.model.store ? props.model.store.phone : "<STORE_PHONE>"}
					</strong>
				</li>
				<li>
					<strong>الرمز البريدي:</strong>{props.model.store ? props.model.store.zipcode_in_arabic : "<STORE_ZIPCODE_ARABIC>"},
					<strong>Email:{props.model.store ? props.model.store.email : "<STORE_EMAIL>"} </strong>

				</li>
			</ul>
		</div>
	</div>
</div>
</body></html>`
	return htmlStr
}
