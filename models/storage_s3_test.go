package models

import "testing"

func TestS3URIEncodeKey_Table(t *testing.T) {
	cases := []struct{ in, want string }{
		{"images/store1/expenses/receipt.jpg", "images/store1/expenses/receipt.jpg"},
		{"a/Cable Comnnection..pdf", "a/Cable%20Comnnection..pdf"},
		{"a/a+b (copy) #2.png", "a/a%2Bb%20%28copy%29%20%232.png"},
		{"a/x~y_z-1.TXT", "a/x~y_z-1.TXT"},
		{"a/ف.pdf", "a/%D9%81.pdf"},
		{"a/q?x=1&y=2;z,@!$'*:", "a/q%3Fx%3D1%26y%3D2%3Bz%2C%40%21%24%27%2A%3A"},
		{"a/100%.pdf", "a/100%25.pdf"},
		{"", ""},
	}
	for _, c := range cases {
		if got := S3URIEncodeKey(c.in); got != c.want {
			t.Errorf("S3URIEncodeKey(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestS3SigningHost_Table(t *testing.T) {
	cases := []struct {
		name string
		s    AdminSettings
		want string
	}{
		{"aws virtual-hosted", AdminSettings{S3BucketName: "b1", S3Region: "me-south-1"}, "b1.s3.me-south-1.amazonaws.com"},
		{"endpoint https", AdminSettings{S3BucketName: "b1", S3Endpoint: "https://sgp1.digitaloceanspaces.com"}, "sgp1.digitaloceanspaces.com"},
		{"endpoint with port and slash", AdminSettings{S3BucketName: "b1", S3Endpoint: "http://127.0.0.1:9000/"}, "127.0.0.1:9000"},
		{"endpoint with path", AdminSettings{S3BucketName: "b1", S3Endpoint: "https://minio.example.com/s3"}, "minio.example.com"},
	}
	for _, c := range cases {
		if got := S3SigningHost(c.s); got != c.want {
			t.Errorf("%s: S3SigningHost = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestS3StoragePutURL_EncodesKey(t *testing.T) {
	s := AdminSettings{S3BucketName: "b1", S3Region: "us-east-1"}
	if got, want := s3StoragePutURL(s, "images/x/logo name.png"), "https://b1.s3.us-east-1.amazonaws.com/images/x/logo%20name.png"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	s.S3Endpoint = "http://127.0.0.1:9000"
	if got, want := s3StoragePutURL(s, "images/x/a+b.png"), "http://127.0.0.1:9000/b1/images/x/a%2Bb.png"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
