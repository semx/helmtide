package prometheus

import "testing"

func TestNewRoundTripperInsecure(t *testing.T) {
	rt := newRoundTripper(true)

	if rt.TLSClientConfig == nil {
		t.Fatal("expected TLSClientConfig to be set")
	}

	if !rt.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("expected InsecureSkipVerify to be true when insecure is true")
	}
}

func TestNewRoundTripperSecureByDefault(t *testing.T) {
	rt := newRoundTripper(false)

	if rt.TLSClientConfig == nil {
		t.Fatal("expected TLSClientConfig to be set")
	}

	if rt.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("expected InsecureSkipVerify to be false by default")
	}
}

func TestInitWiresInsecureClient(t *testing.T) {
	c := NewConfig()
	c.URL = "http://localhost:9090"
	c.Expr = "up"
	c.Insecure = true

	if err := c.Init(t.Context(), nil); err != nil {
		t.Fatalf("unexpected error from Init: %v", err)
	}

	if c.client == nil {
		t.Fatal("expected prometheus client to be constructed")
	}
}
