package aws

import (
	"testing"
	"time"

	"github.com/ymedlop/kuberoutectl/internal/domain"
)

// The mode arrives on the existing describe-cluster response — no extra call.
// A cluster predating access entries has no accessConfig block at all, and that
// must decode to the empty mode rather than failing: an older cluster is a
// normal thing to find, not a parse error.
func TestParseEKSDescribe_AuthenticationMode(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "api",
			body: `{"cluster":{"name":"c","arn":"a","accessConfig":{"authenticationMode":"API"}}}`,
			want: "API",
		},
		{
			name: "api and config map",
			body: `{"cluster":{"name":"c","arn":"a","accessConfig":{"authenticationMode":"API_AND_CONFIG_MAP"}}}`,
			want: "API_AND_CONFIG_MAP",
		},
		{
			name: "config map",
			body: `{"cluster":{"name":"c","arn":"a","accessConfig":{"authenticationMode":"CONFIG_MAP"}}}`,
			want: "CONFIG_MAP",
		},
		{
			name: "no accessConfig block at all",
			body: `{"cluster":{"name":"c","arn":"a","status":"ACTIVE"}}`,
			want: "",
		},
		{
			name: "accessConfig present but empty",
			body: `{"cluster":{"name":"c","arn":"a","accessConfig":{}}}`,
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := parseEKSDescribe([]byte(tc.body))
			if err != nil {
				t.Fatalf("parseEKSDescribe: %v", err)
			}
			if c.AccessConfig.AuthenticationMode != tc.want {
				t.Errorf("AuthenticationMode = %q, want %q", c.AccessConfig.AuthenticationMode, tc.want)
			}
		})
	}
}

// The central function of this feature, and the one most likely to be written
// wrong in a way that still looks right.
//
// An access entry names an IAM principal; STS returns an assumed-role. They are
// never equal as strings, so both are reduced to account + final role/user
// name. The extraction direction is NOT the same for the two shapes:
//
//	assumed-role/NAME/session   → the FIRST segment is the name
//	role/…path…/NAME            → the LAST segment is the name
//
// An implementation that takes the last segment in both cases passes the plain
// role and the IAM user, and fails only the SSO path — which is the one case
// this entire feature exists for. Both shapes are in this one table so that
// implementation cannot go green.
func TestPrincipalKey(t *testing.T) {
	cases := []struct {
		name string
		arn  string
		want string
	}{
		{
			name: "edge case 4a: STS assumed-role, session name discarded",
			arn:  "arn:aws:sts::111122223333:assumed-role/AWSReservedSSO_Platform_ab12/yeray",
			want: "111122223333/AWSReservedSSO_Platform_ab12",
		},
		{
			name: "edge case 4b: the SSO entry for that same identity, IAM path discarded",
			arn:  "arn:aws:iam::111122223333:role/aws-reserved/sso.amazonaws.com/eu-central-1/AWSReservedSSO_Platform_ab12",
			want: "111122223333/AWSReservedSSO_Platform_ab12",
		},
		{
			name: "edge case 5a: plain role entry, no path",
			arn:  "arn:aws:iam::111122223333:role/PlatformAdmin",
			want: "111122223333/PlatformAdmin",
		},
		{
			name: "edge case 5b: assuming that plain role",
			arn:  "arn:aws:sts::111122223333:assumed-role/PlatformAdmin/session-1",
			want: "111122223333/PlatformAdmin",
		},
		{
			name: "edge case 6: IAM user, no session component at all",
			arn:  "arn:aws:iam::111122223333:user/ci-bot",
			want: "111122223333/ci-bot",
		},
		{
			name: "IAM user under a path",
			arn:  "arn:aws:iam::111122223333:user/automation/ci-bot",
			want: "111122223333/ci-bot",
		},
		{
			name: "a different account is a different principal",
			arn:  "arn:aws:iam::999988887777:role/PlatformAdmin",
			want: "999988887777/PlatformAdmin",
		},
		{
			name: "govcloud partition — the partition is not part of the key",
			arn:  "arn:aws-us-gov:iam::111122223333:role/PlatformAdmin",
			want: "111122223333/PlatformAdmin",
		},

		// Everything below must yield "", which callers treat as "no key" and
		// therefore "unknown". A wrong key here would match nothing, which under
		// api mode reads as a confirmed refusal — so failing closed means
		// returning nothing, not guessing.
		{name: "empty string", arn: "", want: ""},
		{name: "not an ARN at all", arn: "yeray", want: ""},
		{name: "too few colons", arn: "arn:aws:iam::111122223333", want: ""},
		{name: "no resource", arn: "arn:aws:iam::111122223333:", want: ""},
		{name: "resource type with no name", arn: "arn:aws:iam::111122223333:role/", want: ""},
		{name: "root user has no name segment", arn: "arn:aws:iam::111122223333:root", want: ""},
		{name: "empty account", arn: "arn:aws:iam:::role/PlatformAdmin", want: ""},
		{name: "a resource type we do not match on", arn: "arn:aws:sts::111122223333:federated-user/dev", want: ""},
		{name: "assumed-role with no session", arn: "arn:aws:sts::111122223333:assumed-role/PlatformAdmin", want: "111122223333/PlatformAdmin"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := principalKey(tc.arn); got != tc.want {
				t.Errorf("principalKey(%q) = %q, want %q", tc.arn, got, tc.want)
			}
		})
	}
}

// Edge case 14: a profile whose STS identity is empty has no key to match, and
// an empty key must never match an entry — otherwise a broken identity would be
// reported as operable on every cluster.
func TestPrincipalKey_EmptyNeverMatchesAnEntry(t *testing.T) {
	entries := []string{
		"arn:aws:iam::111122223333:role/PlatformAdmin",
		"arn:aws:iam::111122223333:user/ci-bot",
	}
	for _, e := range entries {
		if principalKey("") == principalKey(e) {
			t.Errorf("an empty identity matched entry %q", e)
		}
	}
}

// Edge case 10: the command succeeded, so a body we cannot read is a format
// regression, not an empty cluster. Degrading it to "nobody has access" would
// turn a kuberoutectl bug into a confident claim about someone's permissions.
func TestParseAccessEntries(t *testing.T) {
	t.Run("first page carries a nextToken", func(t *testing.T) {
		got, err := parseAccessEntries(readFixture(t, "access-entries-page1.json"))
		if err != nil {
			t.Fatalf("parseAccessEntries: %v", err)
		}
		if got.NextToken == "" {
			t.Error("NextToken is empty; the pagination test below would prove nothing")
		}
		if len(got.AccessEntries) == 0 {
			t.Fatal("no entries decoded")
		}
	})

	t.Run("last page has no token", func(t *testing.T) {
		got, err := parseAccessEntries(readFixture(t, "access-entries-page2.json"))
		if err != nil {
			t.Fatalf("parseAccessEntries: %v", err)
		}
		if got.NextToken != "" {
			t.Errorf("NextToken = %q, want empty on the last page", got.NextToken)
		}
	})

	t.Run("a cluster with no entries is not an error", func(t *testing.T) {
		got, err := parseAccessEntries([]byte(`{"accessEntries":[]}`))
		if err != nil {
			t.Fatalf("an empty list is a legitimate answer, got: %v", err)
		}
		if len(got.AccessEntries) != 0 {
			t.Errorf("entries = %v, want none", got.AccessEntries)
		}
	})

	t.Run("malformed body is a wrapped hard error", func(t *testing.T) {
		if _, err := parseAccessEntries([]byte(`{"accessEntries": [`)); err == nil {
			t.Fatal("want an error for a malformed body, got nil")
		}
	})
}

// The mode is a property of the cluster, identical across every profile that
// sees it, so it can ride on provider-private metadata and survive the fold's
// winner-copy unchanged. It is deliberately NOT written to Target.AccessCheck
// here: an empty AccessCheck means "not attempted", and a single-profile target
// must stay that way even though its mode is perfectly well known.
func TestBuildTarget_CarriesAuthenticationModeInMetadata(t *testing.T) {
	tgt := buildTarget("ops", "eu-central-1",
		awsIdentity{Account: "111122223333"},
		awsCluster{
			Name:         "c",
			Arn:          "arn:aws:eks:eu-central-1:111122223333:cluster/c",
			AccessConfig: awsAccessConfig{AuthenticationMode: "API"},
		},
		domain.HealthValid, domain.ActionUse, time.Unix(0, 0).UTC(),
	)
	if got := tgt.Metadata["authentication_mode"]; got != "API" {
		t.Errorf("metadata authentication_mode = %q, want %q", got, "API")
	}
	if tgt.AccessCheck != "" {
		t.Errorf("AccessCheck = %q, want empty — buildTarget must not claim a check was attempted", tgt.AccessCheck)
	}
}
