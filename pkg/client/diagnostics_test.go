package client

import "testing"

// TestFailureDiagnosticsMessage covers the message a failed wait reports. The diagnostics are
// best-effort extra output, so the assertion still has to say what it was waiting for, and it
// has to say it even when there is no cluster left to gather anything from.
func TestFailureDiagnosticsMessage(t *testing.T) {
	message := FailureDiagnostics("HelmRelease '%s/%s' was not deployed at version '%s'", "org-giantswarm", "test-app", "1.2.3")()

	expected := "HelmRelease 'org-giantswarm/test-app' was not deployed at version '1.2.3'"
	if message != expected {
		t.Errorf("message = %q, expected %q", message, expected)
	}
}

// TestFailureDiagnosticsInMessage covers the namespace-scoped variant: the namespace only
// decides what is gathered, so the message is the caller's either way.
func TestFailureDiagnosticsInMessage(t *testing.T) {
	message := FailureDiagnosticsIn("kube-system", "HelmRelease '%s/%s' did not become ready", "kube-system", "test-app")()

	expected := "HelmRelease 'kube-system/test-app' did not become ready"
	if message != expected {
		t.Errorf("message = %q, expected %q", message, expected)
	}
}
