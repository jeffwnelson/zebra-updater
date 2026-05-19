package printer

import "testing"

func TestBuildGetVar(t *testing.T) {
	got := BuildGetVar("device.product.name")
	want := "! U1 getvar \"device.product.name\"\r\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseResponse(t *testing.T) {
	cases := []struct {
		name, in, want string
		wantErr        bool
	}{
		{"simple", "\"ZD621-203dpi\"\r\n", "ZD621-203dpi", false},
		{"empty", "\"\"\r\n", "", false},
		{"escaped quote", "\"a\\\"b\"\r\n", "a\"b", false},
		{"no quotes", "ZD621\r\n", "", true},
		{"unterminated", "\"ZD621", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseResponse(c.in)
			if (err != nil) != c.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, c.wantErr)
			}
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
