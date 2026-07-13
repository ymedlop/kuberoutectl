package domain

import "testing"

func TestValidateUserLabel_Valid(t *testing.T) {
	cases := []struct{ k, v string }{
		{"env", "prod"},
		{"team", "platform"},
		{"owner", "yeray"},
		{"example.com/tier", "gold"},
		{"empty-value", ""},
	}
	for _, c := range cases {
		if err := ValidateUserLabel(c.k, c.v); err != nil {
			t.Errorf("ValidateUserLabel(%q,%q) unexpected error: %v", c.k, c.v, err)
		}
	}
}

func TestValidateUserLabel_RejectsReservedNamespace(t *testing.T) {
	err := ValidateUserLabelKey(LabelProvider) // kuberoutectl.io/provider
	if err == nil {
		t.Fatal("expected reserved-namespace key to be rejected")
	}
}

func TestValidateUserLabel_RejectsBadFormat(t *testing.T) {
	bad := []struct{ k, v string }{
		{"", "x"},        // empty key
		{"Env!", "x"},    // illegal char in key
		{"env", "pr od"}, // space in value
		{"env", "-bad"},  // value cannot start with '-'
		{"/name", "x"},   // empty prefix
	}
	for _, c := range bad {
		if err := ValidateUserLabel(c.k, c.v); err == nil {
			t.Errorf("expected error for key=%q value=%q", c.k, c.v)
		}
	}
}

func TestValidateLabelValue_TooLong(t *testing.T) {
	long := make([]byte, 64)
	for i := range long {
		long[i] = 'a'
	}
	if err := ValidateLabelValue(string(long)); err == nil {
		t.Error("expected error for 64-char value (max 63)")
	}
}
