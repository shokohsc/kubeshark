package hub

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	authenticationv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

type stubVerifier struct {
	subject string
	err     error
	tokens  []string
}

func (s *stubVerifier) Verify(_ context.Context, token string) (string, error) {
	s.tokens = append(s.tokens, token)
	return s.subject, s.err
}

// doAuthRequest issues GET /echo with the given header name/value pairs.
func doAuthRequest(t *testing.T, h http.Handler, headers ...string) *httptest.ResponseRecorder {
	t.Helper()
	if len(headers)%2 != 0 {
		t.Fatalf("headers must be name/value pairs, got %d entries", len(headers))
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/echo", nil)
	for i := 0; i < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	h.ServeHTTP(rec, req)
	return rec
}

func TestAuthDisabledPasses(t *testing.T) {
	rec := doAuthRequest(t, NewServer(Config{}, NewStore(), nil).Handler())

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /echo status = %d, want 200", rec.Code)
	}
}

func TestAuthHeadersAccepted_AllVariants(t *testing.T) {
	cfg := Config{
		AuthEnabled:     true,
		ServiceAccounts: []string{"ns:kubeshark-cli"},
		License:         "lic-1234",
	}
	cases := []struct {
		name   string
		header string
		value  string
	}{
		{"X-Kubeshark-Authorization", "X-Kubeshark-Authorization", "tok"},
		{"X-Authorization", "X-Authorization", "tok"},
		{"Authorization raw", "Authorization", "tok"},
		{"Authorization bearer", "Authorization", "Bearer tok"},
		{"License-Key", "License-Key", "lic-1234"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := &stubVerifier{subject: "ns:kubeshark-cli"}

			rec := doAuthRequest(t, NewServer(cfg, NewStore(), v).Handler(), tc.header, tc.value)

			if rec.Code != http.StatusOK {
				t.Fatalf("%s: %s status = %d, want 200", tc.name, tc.header, rec.Code)
			}
		})
	}
}

func TestAuthRejected(t *testing.T) {
	cfg := Config{AuthEnabled: true, ServiceAccounts: []string{"ns:kubeshark-cli"}}
	cases := []struct {
		name     string
		verifier *stubVerifier
		headers  []string
	}{
		{"missing header", &stubVerifier{subject: "ns:kubeshark-cli"}, nil},
		{"wrong token", &stubVerifier{err: errors.New("invalid token")}, []string{"X-Kubeshark-Authorization", "tok"}},
		{"subject not allowlisted", &stubVerifier{subject: "other:sa"}, []string{"X-Kubeshark-Authorization", "tok"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doAuthRequest(t, NewServer(cfg, NewStore(), tc.verifier).Handler(), tc.headers...)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want exactly 401", rec.Code)
			}
			if loc := rec.Header().Get("Location"); loc != "" {
				t.Errorf("Location header = %q, want none (401, never 3xx)", loc)
			}
		})
	}
}

func TestLicenseKeyAccepted(t *testing.T) {
	cfg := Config{
		AuthEnabled:     true,
		ServiceAccounts: []string{"ns:kubeshark-cli"},
		License:         "lic-1234",
	}

	vErr := &stubVerifier{err: errors.New("verifier must not be consulted for a license")}
	rec := doAuthRequest(t, NewServer(cfg, NewStore(), vErr).Handler(), "License-Key", "lic-1234")
	if rec.Code != http.StatusOK {
		t.Fatalf("valid License-Key status = %d, want 200", rec.Code)
	}
	if len(vErr.tokens) != 0 {
		t.Errorf("verifier called with %v on license match, want no calls", vErr.tokens)
	}

	vOK := &stubVerifier{subject: "ns:kubeshark-cli"}
	rec = doAuthRequest(t, NewServer(cfg, NewStore(), vOK).Handler(), "License-Key", "wrong-license")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong License-Key status = %d, want exactly 401", rec.Code)
	}
	if len(vOK.tokens) != 0 {
		t.Errorf("verifier called with %v on license mismatch, want no calls", vOK.tokens)
	}
}

func TestBearerPrefixStripped(t *testing.T) {
	cfg := Config{AuthEnabled: true, ServiceAccounts: []string{"ns:kubeshark-cli"}}
	v := &stubVerifier{subject: "ns:kubeshark-cli"}

	rec := doAuthRequest(t, NewServer(cfg, NewStore(), v).Handler(), "Authorization", "Bearer tok")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(v.tokens) != 1 || v.tokens[0] != "tok" {
		t.Fatalf("verifier tokens = %v, want [tok]", v.tokens)
	}
}

func TestServiceAccountSubjectForms(t *testing.T) {
	cfg := Config{AuthEnabled: true, ServiceAccounts: []string{"default:kubeshark-cli"}}
	// Kubernetes returns the full prefix for SA token reviews; the allowlist
	// stores the bare ns:name form, so both must authenticate.
	for _, subject := range []string{
		"default:kubeshark-cli",
		"system:serviceaccount:default:kubeshark-cli",
	} {
		v := &stubVerifier{subject: subject}
		rec := doAuthRequest(t, NewServer(cfg, NewStore(), v).Handler(), "X-Kubeshark-Authorization", "tok")
		if rec.Code != http.StatusOK {
			t.Errorf("subject %q status = %d, want 200", subject, rec.Code)
		}
	}

	v := &stubVerifier{subject: "system:serviceaccount:other:sa"}
	rec := doAuthRequest(t, NewServer(cfg, NewStore(), v).Handler(), "X-Kubeshark-Authorization", "tok")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("non-allowlisted prefixed subject status = %d, want 401", rec.Code)
	}
}

func TestK8sTokenVerifier_TokenReview(t *testing.T) {
	const audience = "kubeshark-hub"

	t.Run("authenticated returns subject", func(t *testing.T) {
		cs := fake.NewSimpleClientset()
		cs.PrependReactor("create", "tokenreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
			review := action.(k8stesting.CreateAction).GetObject().(*authenticationv1.TokenReview)
			if review.Spec.Token != "tok" {
				t.Errorf("TokenReview token = %q, want %q", review.Spec.Token, "tok")
			}
			if len(review.Spec.Audiences) != 1 || review.Spec.Audiences[0] != audience {
				t.Errorf("TokenReview audiences = %v, want [%s]", review.Spec.Audiences, audience)
			}
			return true, &authenticationv1.TokenReview{
				Status: authenticationv1.TokenReviewStatus{
					Authenticated: true,
					Audiences:     []string{audience},
					User:          authenticationv1.UserInfo{Username: "ns:kubeshark-cli"},
				},
			}, nil
		})

		subject, err := NewK8sTokenVerifier(cs, audience).Verify(context.Background(), "tok")

		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if subject != "ns:kubeshark-cli" {
			t.Errorf("subject = %q, want %q", subject, "ns:kubeshark-cli")
		}
	})

	t.Run("not authenticated returns error", func(t *testing.T) {
		cs := fake.NewSimpleClientset()
		cs.PrependReactor("create", "tokenreviews", func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, &authenticationv1.TokenReview{
				Status: authenticationv1.TokenReviewStatus{Authenticated: false},
			}, nil
		})

		if _, err := NewK8sTokenVerifier(cs, audience).Verify(context.Background(), "tok"); err == nil {
			t.Fatal("Verify error = nil, want error for unauthenticated token")
		}
	})

	t.Run("audience mismatch returns error", func(t *testing.T) {
		cs := fake.NewSimpleClientset()
		cs.PrependReactor("create", "tokenreviews", func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, &authenticationv1.TokenReview{
				Status: authenticationv1.TokenReviewStatus{
					Authenticated: true,
					Audiences:     []string{"other-audience"},
					User:          authenticationv1.UserInfo{Username: "ns:kubeshark-cli"},
				},
			}, nil
		})

		if _, err := NewK8sTokenVerifier(cs, audience).Verify(context.Background(), "tok"); err == nil {
			t.Fatal("Verify error = nil, want error for audience mismatch")
		}
	})
}
