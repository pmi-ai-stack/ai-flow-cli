package jenkins

import (
	"path/filepath"
	"strings"
)

const (
	kindJenkinsfile = "jenkinsfile"
	kindJobDSL      = "job-dsl"
	kindJobConfig   = "job-config"
)

// targetKind classifies path (a "/"-separated path relative to a
// repository, or an OS path — filepath.Base tolerates either) by the
// spec's own three Detect signals: a bare "Jenkinsfile", a
// "jobs/*.groovy" (Job DSL — scoped to a jobs/ directory rather than
// every ".groovy" file in a checkout, the phase-27 plan's own design
// decision 6), or a "jobs/*.xml" (job-config XML).
func targetKind(path string) string {
	base := filepath.Base(path)
	switch {
	case base == "Jenkinsfile":
		return kindJenkinsfile
	case strings.HasSuffix(base, ".groovy"):
		return kindJobDSL
	default:
		return kindJobConfig
	}
}

// jobName infers the live Jenkins job name a target's own file
// corresponds to: its own base name with its extension stripped,
// except a bare "Jenkinsfile" (which names no job of its own) falls
// back to repoName — a repository-root Jenkinsfile conventionally
// names a multibranch-pipeline job after its own repository (the
// phase-27 plan's own design decision 7, the same disclosed-convention
// class as helm's chart-name-as-release-name default, phase 26 design
// decision 4).
func jobName(repoName, path string) string {
	base := filepath.Base(path)
	if base == "Jenkinsfile" {
		return repoName
	}
	return strings.TrimSuffix(base, filepath.Ext(base))
}
