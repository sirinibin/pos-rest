package models

import "testing"

// The store's ZATCA signing key and API credentials never leave the server in
// a store API response; everything else stays as it is.
func TestHideZatcaSecrets(t *testing.T) {
	s := Store{Name: "Shop"}
	s.Zatca.PrivateKey, s.Zatca.Secret, s.Zatca.BinarySecurityToken = "PK", "S", "BST"
	s.Zatca.ProductionSecret, s.Zatca.ProductionBinarySecurityToken = "PS", "PBST"
	s.Zatca.Connected, s.Zatca.Phase = true, "2"
	s.HideZatcaSecrets()
	for name, v := range map[string]string{"private_key": s.Zatca.PrivateKey, "secret": s.Zatca.Secret,
		"binary_security_token": s.Zatca.BinarySecurityToken, "production_secret": s.Zatca.ProductionSecret,
		"production_binary_security_token": s.Zatca.ProductionBinarySecurityToken} {
		if v != "" {
			t.Errorf("%s still in the response: %q", name, v)
		}
	}
	if !s.Zatca.Connected || s.Zatca.Phase != "2" || s.Name != "Shop" {
		t.Errorf("other fields changed: %+v", s.Zatca)
	}
}
