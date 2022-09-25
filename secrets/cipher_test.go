package secrets

import (
	"strings"
	"testing"
)

func TestEncDec(t *testing.T) {
	tests := map[string]struct {
		pwd string
		in  string
	}{
		"test-01": {
			pwd: "klebezeug",
			in:  "something not really confidential",
		},
		"test-02": {
			pwd: "",
			in:  "something actually less confidential",
		},
		"test-03": {
			pwd: "dump",
			in:  "",
		},
		"test-04": {
			pwd: "",
			in:  "",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			enc, err := encrypt([]byte(test.in), test.pwd)
			if err != nil {
				t.Fatalf("encrypt failed: %v", err)
			}
			senc := string(enc)
			//check if enc contains somethign from input
			sl := strings.Fields(test.in)
			for _, s := range sl {
				if strings.Contains(senc, s) {
					t.Fatalf("encrypted contains a word of the original data %q", s)
				}
			}

			//decrypt
			dec, err := decrypt(enc, test.pwd)
			if err != nil {
				t.Fatalf("decrypt failed: %v", err)
			}

			//
			if string(dec) != test.in {
				t.Fatalf("have %q, want %q", string(dec), test.in)
			}
		})
	}
}
