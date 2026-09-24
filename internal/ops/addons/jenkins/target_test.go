// Internal (package jenkins, not jenkins_test): targetKind/jobName are
// unexported and this is the only test that needs them directly.
package jenkins

import "testing"

func TestTargetKind(t *testing.T) {
	cases := map[string]string{
		"Jenkinsfile":            kindJenkinsfile,
		"repo/Jenkinsfile":       kindJenkinsfile,
		"jobs/build.groovy":      kindJobDSL,
		"jobs/build.xml":         kindJobConfig,
		"jobs/nested/deploy.xml": kindJobConfig,
	}
	for path, want := range cases {
		if got := targetKind(path); got != want {
			t.Errorf("targetKind(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestJobName(t *testing.T) {
	if got := jobName("acme.svc", "Jenkinsfile"); got != "acme.svc" {
		t.Errorf("jobName(Jenkinsfile) = %q, want the repo name", got)
	}
	if got := jobName("acme.svc", "jobs/build.xml"); got != "build" {
		t.Errorf("jobName(jobs/build.xml) = %q, want build", got)
	}
	if got := jobName("acme.svc", "jobs/deploy.groovy"); got != "deploy" {
		t.Errorf("jobName(jobs/deploy.groovy) = %q, want deploy", got)
	}
}
