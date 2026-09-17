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
