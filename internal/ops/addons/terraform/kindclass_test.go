// Internal (package terraform, not terraform_test): classifyKind is
// unexported and this is the only test that needs it directly.
package terraform

import (
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
)

func TestClassifyKind(t *testing.T) {
	cases := []struct {
		resourceType string
		want         ops.KindClass
	}{
		{"aws_db_instance", ops.KindData},
		{"aws_rds_cluster", ops.KindData},
		{"aws_dynamodb_table", ops.KindData},
		{"aws_s3_bucket", ops.KindData},
		{"aws_s3_bucket_policy", ops.KindData},
		{"aws_iam_role", ops.KindIAM},
		{"aws_iam_role_policy_attachment", ops.KindIAM},
		{"aws_secretsmanager_secret", ops.KindSecret},
		{"aws_kms_key", ops.KindSecret},
		{"aws_vpc", ops.KindNetwork},
		{"aws_security_group", ops.KindNetwork},
		{"aws_codepipeline", ops.KindPipeline},
		{"aws_instance", ops.KindWorkload},
		{"aws_lambda_function", ops.KindWorkload},
		{"google_compute_instance", ops.KindWorkload},
	}
	for _, c := range cases {
		if got := classifyKind(c.resourceType); got != c.want {
			t.Errorf("classifyKind(%q) = %q, want %q", c.resourceType, got, c.want)
		}
	}
}
