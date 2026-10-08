package hub

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"

	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// TokenVerifier validates a bearer token and returns its subject, e.g. "ns:name".
type TokenVerifier interface {
	Verify(ctx context.Context, token string) (subject string, err error)
}

type k8sTokenVerifier struct {
	cs       kubernetes.Interface
	audience string
}

// NewK8sTokenVerifier verifies tokens via the Kubernetes TokenReview API for
// the given audience.
func NewK8sTokenVerifier(cs kubernetes.Interface, audience string) TokenVerifier {
	return &k8sTokenVerifier{cs: cs, audience: audience}
}

func (v *k8sTokenVerifier) Verify(ctx context.Context, token string) (string, error) {
	review, err := v.cs.AuthenticationV1().TokenReviews().Create(ctx, &authenticationv1.TokenReview{
		Spec: authenticationv1.TokenReviewSpec{
			Token:     token,
			Audiences: []string{v.audience},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		return "", err
	}
	if !review.Status.Authenticated {
		return "", errors.New("token not authenticated")
	}
	// TokenReview clients that set spec.audiences must check the echoed status.audiences:
	// authenticated with an empty list only proves the API-server audience, not ours.
	if !slices.Contains(review.Status.Audiences, v.audience) {
		return "", errors.New("token not authenticated for audience")
	}
	return review.Status.User.Username, nil
}

// licenseHeader is checked after the token headers (see extractCredential).
const licenseHeader = "License-Key"

// extractCredential returns the first credential present on the request, in
// the order the Hub accepts them, flagging one that came from the license header.
func extractCredential(h http.Header) (value string, isLicense bool) {
	if v := h.Get("X-Kubeshark-Authorization"); v != "" {
		return v, false
	}
	if v := h.Get("X-Authorization"); v != "" {
		return v, false
	}
	if v := h.Get("Authorization"); v != "" {
		// ponytail: "Bearer " matched case-sensitively; a client sending "bearer " gets a 401.
		return strings.TrimPrefix(v, "Bearer "), false
	}
	if v := h.Get(licenseHeader); v != "" {
		return v, true
	}
	return "", false
}

// requireAuth wraps next with the auth middleware: pass-through when auth is
// off, otherwise a license match or an allowlisted TokenReview subject.
// Every rejection is a plain 401, never a redirect.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.cfg.AuthEnabled || s.authorized(r) {
			next.ServeHTTP(w, r)
			return
		}
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
	})
}

func (s *Server) authorized(r *http.Request) bool {
	credential, isLicense := extractCredential(r.Header)
	switch {
	case isLicense:
		return s.cfg.License != "" && credential == s.cfg.License
	case credential != "" && s.verifier != nil:
		subject, err := s.verifier.Verify(r.Context(), credential)
		return err == nil && slices.Contains(s.cfg.ServiceAccounts, subject)
	default:
		return false
	}
}
